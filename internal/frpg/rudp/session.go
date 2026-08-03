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
	maxAckTopQuart     = (maxAckValue / 4) * 3
	maxAckBottomQuart  = (maxAckValue / 4) * 1
	maxPacketsInFlight = 32

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
	steamID  string
	send     SendFunc
	now      func() time.Time

	state State

	sequenceIndex            uint32
	sequenceIndexAcked       uint32
	remoteSequenceIndex      uint32
	remoteSequenceIndexAcked uint32

	pendingReceive  []packet
	received        [][]byte
	sendQueue       []outPacket
	retransmitBuf   []outPacket
	isRetransmit    bool
	retransmitIndex uint32
	retransmitTimer time.Time
	retransmitTries int
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

func newSession(isClient bool, steamID string, send SendFunc, opts ...Option) *Session {
	s := &Session{
		isClient:      isClient,
		steamID:       steamID,
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
func NewClientSession(steamID string, send SendFunc, opts ...Option) *Session {
	return newSession(true, steamID, send, opts...)
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

// Receive returns the next in-order DAT payload, if any.
func (s *Session) Receive() ([]byte, bool) {
	if len(s.received) == 0 {
		return nil, false
	}
	p := s.received[0]
	s.received = s.received[1:]
	return p, true
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
	p := packet{opcode: OpUnset, payload: payload}
	s.enqueue(p)
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
		b = append(initialData(s.steamID), b...)
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
	case OpDAT:
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
	case OpDATACK:
		s.handleAckLike(p.remote)
		s.sendACK(p.local)
		s.received = append(s.received, p.payload)
	case OpRACK:
		// "Reject ACK" — ignored.
	default:
		s.errState = fmt.Errorf("rudp: unknown opcode %s", p.opcode)
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
	s.received = append(s.received, p.payload)
	s.sendACK(p.local)
}

// handleAckLike updates the highest acknowledged local sequence, with the
// reference's crude 12-bit overflow handling.
func (s *Session) handleAckLike(inRemoteAck uint32) {
	if s.sequenceIndexAcked > maxAckTopQuart && inRemoteAck < maxAckBottomQuart {
		s.sequenceIndexAcked = inRemoteAck
	} else if inRemoteAck > s.sequenceIndexAcked {
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
		local := op.pkt.local
		acked := (local > maxAckTopQuart && s.sequenceIndexAcked < maxAckBottomQuart) || local <= s.sequenceIndexAcked
		if !acked {
			kept = append(kept, op)
		}
	}
	s.retransmitBuf = kept

	now := s.now()
	if !s.isRetransmit {
		if len(s.retransmitBuf) > 0 {
			op := s.retransmitBuf[0]
			if now.Sub(op.sendTime) > retransmitInterval {
				s.sendRaw(op.pkt)
				s.isRetransmit = true
				s.retransmitIndex = op.pkt.local
				s.retransmitPkt = op.pkt
				s.retransmitTries = 0
				s.retransmitTimer = now
			}
		}
	} else {
		recovered := (s.retransmitIndex > maxAckTopQuart && s.sequenceIndexAcked < maxAckBottomQuart) || s.sequenceIndexAcked >= s.retransmitIndex
		if recovered {
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

	for !s.isRetransmit && len(s.sendQueue) > 0 && len(s.retransmitBuf) < maxPacketsInFlight {
		op := s.sendQueue[0]
		s.sendQueue = s.sendQueue[1:]
		op.rawSendTime = s.now()
		s.retransmitBuf = append(s.retransmitBuf, op)
		s.sendRaw(op.pkt)
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
