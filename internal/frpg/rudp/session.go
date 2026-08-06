package rudp

import (
	"crypto/rand"
	"fmt"
	"time"
)

// State is the reliable-UDP connection state.
type State int

const (
	StateListening State = iota
	StateSynReceived
	StateEstablished
	StateClosing
	StateClosed
	StateConnecting
)

func (s State) String() string {
	switch s {
	case StateListening:
		return "Listening"
	case StateSynReceived:
		return "SynReceived"
	case StateEstablished:
		return "Established"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	case StateConnecting:
		return "Connecting"
	default:
		return "Unknown"
	}
}

// Protocol constants (from the reference Frpg2ReliableUdpPacketStream).
const (
	maxAckValue        = 4096
	maxPacketsInFlight = 32

	// maxPacketsPerPump paces the send queue. The in-flight cap alone bounds how
	// many packets are UNACKNOWLEDGED at once, but says nothing about how fast
	// they leave — so a full window used to go out back-to-back in microseconds.
	//
	// That killed a real session on 2026-08-05. Four RequestGetGhostDataList
	// replies of ~10.7 KB arrived within 148 ms; at 900-byte fragments that is 13
	// packets each, 54 in total, and they went out as one burst. The client
	// NACKed 25 of them and never received 5. Its ack froze one packet short, we
	// retransmitted that packet for the full 32-second budget, it was never
	// accepted, and the session died.
	//
	// The other client survived the identical request pattern the same minute,
	// because its replies were ~6 KB — 7 fragments each, 28 packets, inside the
	// in-flight window. Burst size was the ONLY difference between the two.
	//
	// At the 100 ms pump interval this is 80 packets/sec, so a 13-fragment ghost
	// list takes ~200 ms to clear and the 54-packet case ~700 ms. Slower than a
	// burst, and slower is the entire point: the receiver is an emulated PS3
	// network stack and it demonstrably cannot absorb a LAN-speed window.
	maxPacketsPerPump = 8

	retransmitInterval      = 1000 * time.Millisecond
	retransmitCycleInterval = 200 * time.Millisecond
	retransmitMaxAttempts   = 160
	resendSynInterval       = 500 * time.Millisecond
	minTimeBetweenResendAck = 150 * time.Millisecond
	heartbeatInterval       = 5 * time.Second
	connectionCloseTimeout  = 5 * time.Second
)

// SendFunc transmits an encoded reliable packet; connectionPrefix marks the SYN
// datagram whose payload carries the 35-byte connection prefix.
type SendFunc func(reliableBytes []byte, connectionPrefix bool) error

type outPacket struct {
	pkt         packet
	sendTime    time.Time
	rawSendTime time.Time
}

// Session is one reliable-UDP connection (server or client role).
type Session struct {
	isClient bool
	psnID    string
	send     SendFunc
	now      func() time.Time

	state State

	sequenceIndex            uint32
	sequenceIndexAcked       uint32
	remoteSequenceIndex      uint32
	remoteSequenceIndexAcked uint32

	pendingReceive  []packet
	received        []recvItem
	sendQueue       []outPacket
	retransmitBuf   []outPacket
	isRetransmit    bool
	retransmitIndex uint32
	retransmitTimer time.Time
	retransmitTries int
	rackCount       int
	holeMisses      int
	unknownOpcodes  int
	retransmitPkt   packet

	lastPacketReceived time.Time
	lastAckSend        time.Time
	lastHeartbeat      time.Time
	resendSynTimer     time.Time
	closeTimer         time.Time

	errState error
}

// Option configures a Session.
type Option func(*Session)

// WithNow overrides the clock (for tests).
func WithNow(now func() time.Time) Option { return func(s *Session) { s.now = now } }

// WithInitialSequence fixes the initial local sequence (for deterministic tests).
func WithInitialSequence(v uint32) Option {
	return func(s *Session) { s.sequenceIndex = v % maxAckValue }
}

func newSession(isClient bool, psnID string, send SendFunc, opts ...Option) *Session {
	s := &Session{
		isClient:      isClient,
		psnID:         psnID,
		send:          send,
		now:           time.Now,
		state:         StateListening,
		sequenceIndex: randSequence(),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// NewServerSession creates a server-role session (waits for an incoming SYN).
func NewServerSession(send SendFunc, opts ...Option) *Session {
	return newSession(false, "", send, opts...)
}

// NewClientSession creates a client-role session; call Connect to initiate.
func NewClientSession(psnID string, send SendFunc, opts ...Option) *Session {
	return newSession(true, psnID, send, opts...)
}

func randSequence() uint32 {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return (uint32(b[0]) | uint32(b[1])<<8) % maxAckValue
}

// State returns the current connection state.
func (s *Session) State() State { return s.state }

// Err returns any fatal error the session has entered.
func (s *Session) Err() error { return s.errState }

// recvItem is a delivered DAT payload together with the local sequence number
// that a reply should acknowledge (DAT_ACK).
type recvItem struct {
	data   []byte
	ackSeq uint32
}

// Receive returns the next in-order DAT payload and the sequence a reply should
// acknowledge.
func (s *Session) Receive() (data []byte, ackSeq uint32, ok bool) {
	if len(s.received) == 0 {
		return nil, 0, false
	}
	p := s.received[0]
	s.received = s.received[1:]
	return p.data, p.ackSeq, true
}

func (s *Session) nextRemoteSequence() uint32 { return (s.remoteSequenceIndex + 1) % maxAckValue }

// Connect initiates the handshake (client role).
func (s *Session) Connect() {
	s.state = StateConnecting
	s.resendSynTimer = s.now()
	s.sendSYN()
}

// SendData queues an application payload for reliable delivery as a DAT packet.
func (s *Session) SendData(payload []byte) {
	s.enqueue(packet{opcode: OpUnset, payload: payload})
}

// SendDataAck queues an application payload as a DAT_ACK that also acknowledges
// the remote sequence ackRemote (used for replies).
func (s *Session) SendDataAck(payload []byte, ackRemote uint32) {
	s.enqueue(packet{opcode: OpUnset, remote: ackRemote, payload: payload})
}

// enqueue implements the reference Send(): sequenced/unset opcodes go through the
// retransmit channel; everything else is sent immediately.
func (s *Session) enqueue(p packet) {
	if s.state == StateClosing {
		return
	}
	if p.opcode.isSequenced() || p.opcode == OpUnset {
		if p.opcode == OpUnset {
			remote := p.remote
			p.local = s.sequenceIndex
			if remote > 0 {
				p.opcode = OpDATACK
				s.remoteSequenceIndexAcked = remote
			} else {
				p.opcode = OpDAT
			}
		}
		s.sequenceIndex = (s.sequenceIndex + 1) % maxAckValue
		s.sendQueue = append(s.sendQueue, outPacket{pkt: p, sendTime: s.now()})
	} else {
		s.sendRaw(p)
	}
}

func (s *Session) sendRaw(p packet) {
	if s.errState != nil {
		return
	}
	b := p.encode()
	connPrefix := false
	if p.opcode == OpSYN {
		// Client SYN carries the connection prefix ahead of the packet.
		b = append(initialData(s.psnID), b...)
		connPrefix = true
	}
	if err := s.send(b, connPrefix); err != nil {
		s.errState = err
	}
}

// Feed processes one decrypted incoming datagram payload (with the connection
// prefix already possibly present).
func (s *Session) Feed(payload []byte) {
	// Strip the 35-byte connection prefix from the client's SYN datagram.
	if looksLikeInitialData(payload) {
		payload = payload[initialDataSize:]
	}
	p, err := decodePacket(payload)
	if err != nil {
		s.errState = err
		return
	}
	s.handleIncomingPacket(p)
	s.consumeIncoming()
}

func (s *Session) handleIncomingPacket(p packet) {
	s.lastPacketReceived = s.now()

	if p.opcode.isSequenced() {
		// A sequenced packet while we are not established means the two sides
		// disagree about whether a connection exists: the client is sending game
		// data, we have no session for it. Tell it so.
		//
		// Without this the disagreement is unresolvable. We answer with an ACK
		// for sequence 0, which is meaningless to a client waiting on an ack for
		// 753, and it has no reason to re-SYN because as far as it knows it is
		// connected. Observed live on 2026-08-05: 52 seconds of the client
		// retransmitting the same four sequences into a server that had silently
		// replaced its session, until the client gave up and sent RST. A ~30
		// second stall became a 4 minute outage.
		//
		// RST is what the client itself sends in this situation, and it makes the
		// client tear down and reconnect promptly instead of waiting us out.
		// Rate-limited on the same timer as the redundant ACK it replaces, so a
		// retransmitting peer cannot make us flood.
		if s.state == StateListening {
			if s.now().Sub(s.lastAckSend) > minTimeBetweenResendAck {
				s.sendRaw(packet{opcode: OpRST, local: 0, remote: s.remoteSequenceIndexAcked})
				s.lastAckSend = s.now()
			}
			return
		}

		outOfSequence := false
		if s.state != StateEstablished {
			outOfSequence = true
		}
		if p.local != s.nextRemoteSequence() {
			outOfSequence = true
		}
		if indexByLocal(s.pendingReceive, p.local) >= 0 {
			outOfSequence = true
		}
		if outOfSequence {
			if s.now().Sub(s.lastAckSend) > minTimeBetweenResendAck {
				s.sendACK(s.remoteSequenceIndexAcked)
			}
			return
		}
		s.pendingReceive = append(s.pendingReceive, p)
		return
	}
	s.processPacket(p)
}

func (s *Session) consumeIncoming() {
	for len(s.pendingReceive) > 0 {
		next := s.pendingReceive[0]
		if next.local == s.nextRemoteSequence() {
			s.processPacket(next)
			s.pendingReceive = s.pendingReceive[1:]
			s.remoteSequenceIndex = (s.remoteSequenceIndex + 1) % maxAckValue
		} else {
			break
		}
	}
}

func (s *Session) processPacket(p packet) {
	switch p.opcode {
	case OpSYN:
		s.handleSYN(p)
	case OpSYNACK:
		s.handleSYNACK(p)
	case OpDAT, OpPTDATFRAG:
		// PT_DAT_FRAG is "DAT, marked partial": the client's receive dispatcher
		// indexes the same handler for base opcodes 4 and 8, differing only in a
		// flag passed to the message-store insert. Treating it as anything else —
		// or as an error — would kill a session over a packet the client considers
		// ordinary.
		s.handleDAT(p)
	case OpHBT:
		s.handleAckLike(p.remote)
		if !s.isClient {
			s.sendHBT()
		}
	case OpFIN:
		s.sendFINACK(p.local)
		s.state = StateClosing
	case OpFINACK:
		s.state = StateClosing
	case OpRST:
		s.state = StateListening
		s.reset()
	case OpACK:
		if s.state == StateSynReceived {
			s.state = StateEstablished
		}
		s.handleAckLike(p.remote)
	case OpDATACK, OpPTDATFRGACK:
		s.handleAckLike(p.remote)
		s.sendACK(p.local)
		s.received = append(s.received, recvItem{data: p.payload, ackSeq: p.local})
	case OpRACK:
		s.handleRACK(p)
	default:
		// Counted, never fatal. The client itself ignores base opcodes above 8,
		// and killing a live session over one unexpected byte is a far worse
		// failure than dropping the packet.
		s.unknownOpcodes++
	}
}

func (s *Session) handleSYN(p packet) {
	s.state = StateSynReceived
	s.sendSYNACK(p.local)
	s.sendACK(p.local)
}

func (s *Session) handleSYNACK(p packet) {
	s.state = StateSynReceived
	s.remoteSequenceIndex = p.local
	s.sendACK(s.remoteSequenceIndex)
	s.sequenceIndex = (s.sequenceIndex + 1) % maxAckValue
}

func (s *Session) handleDAT(p packet) {
	s.received = append(s.received, recvItem{data: p.payload, ackSeq: p.local})
	s.sendACK(p.local)
}

// handleRACK responds to the peer's rejected-packet report.
//
// RACK is NOT "reject ack" as the reference guesses. It is a periodic statistics
// report from the client's transmit pump — "I discarded N packets totalling M
// bytes because they were out of sequence" — carrying a u32 count and u32 byte
// total. Confirmed in the client at EBOOT 0xEA8684 (v1.10), where the pump emits
// it whenever its rejected-bytes counter is non-zero and then zeroes both.
//
// It is the only NACK the protocol has, and it tells us exactly one thing: the
// client is missing the packet after the one it last acknowledged. That matters
// enormously, because while a gap is open the client is STRUCTURALLY UNABLE to
// acknowledge anything further — its ack scheduler only fires when its last-sent
// ack differs from its receive index, and the receive index cannot advance past
// a hole. Worse, retransmitting a sequence it has already buffered is discarded
// in silence. So ignoring RACK and retransmitting blindly is exactly how a
// session spends its whole retransmit budget talking into a void.
//
// RACK also carries a valid ack. The client's encoder writes the ack counters for
// RACK and ACK even when it has nothing new to acknowledge, and leaves them zero
// for every other opcode — which is why a captured RACK reads their_ack=1381
// while the DATs around it read 0.
func (s *Session) handleRACK(p packet) {
	s.rackCount++
	s.handleAckLike(p.remote)
	// Fast retransmit: send precisely the packet the peer is missing, now, rather
	// than waiting out the retransmit timer on whatever happens to be at the head
	// of the buffer.
	s.retransmitHole()
}

// retransmitHole sends the one packet the peer is waiting for: the successor to
// its last acknowledged sequence. Reports whether it found it.
func (s *Session) retransmitHole() bool {
	want := (s.sequenceIndexAcked + 1) % maxAckValue
	for _, op := range s.retransmitBuf {
		if op.pkt.local != want {
			continue
		}
		s.sendRaw(op.pkt)
		s.isRetransmit = true
		s.retransmitIndex = op.pkt.local
		s.retransmitPkt = op.pkt
		s.retransmitTries = 0
		s.retransmitTimer = s.now()
		return true
	}
	// The peer wants a sequence we do not hold. That is a hole in our own
	// numbering rather than a lost packet, and no amount of retransmitting will
	// fix it — so it must be visible instead of silently consuming the budget.
	s.holeMisses++
	return false
}

// handleAckLike updates the highest acknowledged local sequence.
func (s *Session) handleAckLike(inRemoteAck uint32) {
	if seqNewer(inRemoteAck, s.sequenceIndexAcked) {
		s.sequenceIndexAcked = inRemoteAck
	}
}

func (s *Session) sendSYN() {
	s.enqueue(packet{opcode: OpSYN, local: s.sequenceIndex, remote: 0, payload: append([]byte(nil), synPayload...)})
}

func (s *Session) sendSYNACK(remote uint32) {
	s.enqueue(packet{opcode: OpSYNACK, local: s.sequenceIndex, remote: remote, payload: append([]byte(nil), synAckPayload...)})
	s.remoteSequenceIndex = remote
	s.sequenceIndex = (s.sequenceIndex + 1) % maxAckValue
}

func (s *Session) sendACK(remote uint32) {
	s.enqueue(packet{opcode: OpACK, local: 0, remote: remote})
	s.remoteSequenceIndexAcked = remote
	s.lastAckSend = s.now()
}

func (s *Session) sendFINACK(remote uint32) {
	s.enqueue(packet{opcode: OpFINACK, local: s.sequenceIndex, remote: remote})
}

func (s *Session) sendHBT() {
	s.enqueue(packet{opcode: OpHBT, local: 0, remote: s.remoteSequenceIndexAcked})
}

// SendReset tells the peer the connection is gone.
//
// Called when the server abandons a session — a dead transport or an idle
// timeout — so the client finds out immediately instead of discovering it by
// being ignored. It bypasses the send queue deliberately: the queue is exactly
// what has failed in the case this exists for, and the packet is unsequenced so
// it needs no acknowledgement.
//
// errState is cleared for the duration because sendRaw refuses to transmit on a
// failed session, and this is the one message that must still go out. Nothing
// else runs against the session afterwards — the caller is discarding it.
func (s *Session) SendReset() {
	saved := s.errState
	s.errState = nil
	s.sendRaw(packet{opcode: OpRST, local: 0, remote: s.remoteSequenceIndexAcked})
	s.errState = saved
}

func (s *Session) reset() {
	s.sequenceIndex = randSequence()
	s.sequenceIndexAcked = 0
	s.remoteSequenceIndex = 0
	s.remoteSequenceIndexAcked = 0
	s.pendingReceive = nil
	s.received = nil
	s.sendQueue = nil
	s.retransmitBuf = nil
}

// Pump advances timers, performs retransmission, and flushes the send queue.
// It returns a non-nil error if the connection has failed.
func (s *Session) Pump() error {
	if s.errState != nil {
		return s.errState
	}
	if s.state == StateClosing && len(s.sendQueue) == 0 {
		s.state = StateClosed
	}
	if s.state == StateClosed {
		s.reset()
		return nil
	}

	if s.state == StateEstablished && s.isClient {
		if s.now().Sub(s.lastHeartbeat) > heartbeatInterval {
			s.sendHBT()
			s.lastHeartbeat = s.now()
		}
	}
	if s.state == StateConnecting {
		if s.now().Sub(s.resendSynTimer) > resendSynInterval {
			s.sendSYN()
			s.resendSynTimer = s.now()
		}
	}
	if !s.closeTimer.IsZero() && s.state == StateClosing {
		if s.now().Sub(s.closeTimer) > connectionCloseTimeout {
			s.state = StateClosed
			return nil
		}
	}

	s.handleOutgoing()
	return s.errState
}

func (s *Session) handleOutgoing() {
	// Trim acknowledged packets from the retransmit buffer.
	kept := s.retransmitBuf[:0]
	for _, op := range s.retransmitBuf {
		if !seqAtOrBefore(op.pkt.local, s.sequenceIndexAcked) {
			kept = append(kept, op)
		}
	}
	s.retransmitBuf = kept

	now := s.now()
	if !s.isRetransmit {
		if len(s.retransmitBuf) > 0 {
			op := s.retransmitBuf[0]
			if now.Sub(op.sendTime) > retransmitInterval {
				// Always retransmit the sequence the peer is actually missing.
				// Resending anything it already holds is discarded in silence, so
				// picking the head of the buffer is only correct by coincidence.
				if !s.retransmitHole() {
					s.sendRaw(op.pkt)
					s.isRetransmit = true
					s.retransmitIndex = op.pkt.local
					s.retransmitPkt = op.pkt
					s.retransmitTries = 0
					s.retransmitTimer = now
				}
			}
		}
	} else {
		if seqAtOrBefore(s.retransmitIndex, s.sequenceIndexAcked) {
			s.isRetransmit = false
		} else if now.Sub(s.retransmitTimer) > retransmitCycleInterval {
			s.retransmitTimer = now
			s.retransmitTries++
			if s.retransmitTries > retransmitMaxAttempts {
				s.errState = fmt.Errorf("rudp: connection died (max retransmits)")
				return
			}
			s.sendRaw(s.retransmitPkt)
		}
	}

	sent := 0
	for !s.isRetransmit && len(s.sendQueue) > 0 &&
		len(s.retransmitBuf) < maxPacketsInFlight && sent < maxPacketsPerPump {
		op := s.sendQueue[0]
		s.sendQueue = s.sendQueue[1:]
		op.rawSendTime = s.now()
		s.retransmitBuf = append(s.retransmitBuf, op)
		s.sendRaw(op.pkt)
		sent++
	}
}

func indexByLocal(q []packet, local uint32) int {
	for i, p := range q {
		if p.local == local {
			return i
		}
	}
	return -1
}

// Diag is a snapshot of the transport state, for logging a session that looks
// stuck. Retransmit trouble is otherwise invisible until the connection dies.
type Diag struct {
	// Retransmitting is true while an unacknowledged packet is blocking the send
	// queue — nothing new goes out until it is acknowledged or the session dies.
	Retransmitting bool
	// StuckSeq is the sequence number being retransmitted.
	StuckSeq uint32
	// Tries is how many retransmit attempts have been spent of retransmitMaxAttempts.
	Tries, MaxTries int
	// Acked is the highest sequence the peer has acknowledged.
	Acked uint32
	// Pending is how many packets are awaiting acknowledgement.
	Pending int
	// RACKs is how many rejected-packet reports the peer has sent. Each one means
	// it discarded out-of-sequence packets and is waiting on a gap.
	RACKs int
	// HoleMisses counts times the peer asked for a sequence we do not hold — a
	// hole in our own numbering rather than a lost packet.
	HoleMisses int
	// UnknownOpcodes counts packets we could not classify. Never fatal.
	UnknownOpcodes int
}

// Diag returns the current transport state.
func (s *Session) Diag() Diag {
	return Diag{
		Retransmitting: s.isRetransmit,
		StuckSeq:       s.retransmitIndex,
		Tries:          s.retransmitTries,
		MaxTries:       retransmitMaxAttempts,
		Acked:          s.sequenceIndexAcked,
		Pending:        len(s.retransmitBuf),
		RACKs:          s.rackCount,
		HoleMisses:     s.holeMisses,
		UnknownOpcodes: s.unknownOpcodes,
	}
}
