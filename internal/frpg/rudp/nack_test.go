package rudp

import (
	"testing"
	"time"
)

// newTestSession builds a server session whose sends are captured rather than
// transmitted, so the retransmit choice can be inspected.
func newCapturingSession(t *testing.T) (*Session, *[]packet) {
	t.Helper()
	var sent []packet
	s := NewServerSession(func(b []byte, _ bool) error {
		p, err := decodePacket(b)
		if err == nil {
			sent = append(sent, p)
		}
		return nil
	})
	s.state = StateEstablished
	return s, &sent
}

// TestRACKRetransmitsTheMissingPacket is the regression for the bug that killed
// live sessions.
//
// While a gap is open the client is structurally unable to acknowledge anything,
// and it silently discards retransmissions of sequences it already holds. So the
// only packet that can restart the connection is the successor to its last ack.
// Sending anything else spends the whole 32-second retransmit budget on packets
// the peer throws away without a word.
func TestRACKRetransmitsTheMissingPacket(t *testing.T) {
	s, sent := newCapturingSession(t)

	// Three packets in flight: 10, 11, 12. The peer has acknowledged 10, so the
	// hole is 11 — but 12 is also outstanding.
	for _, seq := range []uint32{10, 11, 12} {
		s.retransmitBuf = append(s.retransmitBuf, outPacket{
			pkt:      packet{opcode: OpDAT, local: seq, payload: []byte{byte(seq)}},
			sendTime: time.Now(),
		})
	}
	s.sequenceIndexAcked = 10
	*sent = nil

	// The peer reports it rejected out-of-sequence packets, carrying its ack.
	s.handleRACK(packet{opcode: OpRACK, local: 0, remote: 10})

	if len(*sent) != 1 {
		t.Fatalf("RACK produced %d packets, want exactly 1 fast retransmit", len(*sent))
	}
	if got := (*sent)[0].local; got != 11 {
		t.Errorf("retransmitted seq %d, want 11 (the hole). Resending anything the "+
			"peer already holds is discarded in silence", got)
	}
	if s.rackCount != 1 {
		t.Errorf("rackCount = %d, want 1", s.rackCount)
	}
}

// TestRACKCarriesAValidAck — RACK and ACK are the only opcodes whose ack counters
// the client populates when it has nothing new to acknowledge. Dropping that ack
// throws away the one piece of information the packet exists to deliver.
func TestRACKCarriesAValidAck(t *testing.T) {
	s, _ := newCapturingSession(t)
	s.sequenceIndexAcked = 5

	s.handleRACK(packet{opcode: OpRACK, remote: 9})

	if s.sequenceIndexAcked != 9 {
		t.Errorf("sequenceIndexAcked = %d after a RACK acking 9, want 9", s.sequenceIndexAcked)
	}
}

// TestRetransmitTargetsTheHoleNotTheBufferHead — the timer-driven path must make
// the same choice as the RACK path.
func TestRetransmitTargetsTheHoleNotTheBufferHead(t *testing.T) {
	s, sent := newCapturingSession(t)

	// Buffer head is 20, but the peer has acknowledged 21 already — so the packet
	// it actually wants is 22.
	for _, seq := range []uint32{20, 21, 22, 23} {
		s.retransmitBuf = append(s.retransmitBuf, outPacket{
			pkt:      packet{opcode: OpDAT, local: seq},
			sendTime: time.Now().Add(-2 * retransmitInterval),
		})
	}
	s.sequenceIndexAcked = 21
	*sent = nil

	s.handleOutgoing()

	if len(*sent) == 0 {
		t.Fatal("no retransmit issued")
	}
	if got := (*sent)[0].local; got != 22 {
		t.Errorf("retransmitted seq %d, want 22 (acked+1), not the buffer head", got)
	}
}

// TestHoleWeDoNotHoldIsCounted — if the peer wants a sequence we never sent, no
// amount of retransmitting helps. It must be visible rather than silently
// consuming the budget.
func TestHoleWeDoNotHoldIsCounted(t *testing.T) {
	s, _ := newCapturingSession(t)
	s.retransmitBuf = append(s.retransmitBuf, outPacket{
		pkt: packet{opcode: OpDAT, local: 100}, sendTime: time.Now(),
	})
	s.sequenceIndexAcked = 50 // wants 51, which we do not hold

	if s.retransmitHole() {
		t.Error("claimed to retransmit a sequence not in the buffer")
	}
	if s.Diag().HoleMisses != 1 {
		t.Errorf("HoleMisses = %d, want 1 — an unfillable hole must be diagnosable",
			s.Diag().HoleMisses)
	}
}

// TestFragmentOpcodesAreDataNotErrors — PT_DAT_FRAG shares the client's DAT
// handler, differing only by a flag. Treating it as unknown killed the session.
func TestFragmentOpcodesAreDataNotErrors(t *testing.T) {
	for _, op := range []OpCode{OpPTDATFRAG, OpPTDATFRGACK} {
		s, _ := newCapturingSession(t)
		s.processPacket(packet{opcode: op, local: 1, remote: 0, payload: []byte{0xAA}})
		if s.errState != nil {
			t.Errorf("%s set errState (%v); it is ordinary data to the client", op, s.errState)
		}
	}
}

// TestUnknownOpcodeDoesNotKillTheSession — the client ignores base opcodes above
// 8, and dropping a live session over one unexpected byte is far worse than
// dropping the packet.
func TestUnknownOpcodeDoesNotKillTheSession(t *testing.T) {
	s, _ := newCapturingSession(t)
	s.processPacket(packet{opcode: OpCode(0x64), local: 1})

	if s.errState != nil {
		t.Errorf("an unknown opcode killed the session: %v", s.errState)
	}
	if s.Diag().UnknownOpcodes != 1 {
		t.Errorf("UnknownOpcodes = %d, want 1", s.Diag().UnknownOpcodes)
	}
}
