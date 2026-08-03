// Package rudp implements FromSoftware's reliable-UDP-over-UDP transport used by
// the game server: a hand-rolled, TCP-like protocol with SYN/ACK handshaking,
// sequence numbers, and retransmission, layered above the encrypted datagrams.
// This file defines the wire packet; session.go holds the state machine.
package rudp

import "fmt"

// OpCode is the reliable-UDP packet operation.
type OpCode uint8

const (
	OpUnset       OpCode = 0x00
	OpSYN         OpCode = 0x02
	OpRACK        OpCode = 0x03
	OpDAT         OpCode = 0x04
	OpHBT         OpCode = 0x05
	OpFIN         OpCode = 0x06
	OpRST         OpCode = 0x07
	OpPTDATFRAG   OpCode = 0x08
	OpACK         OpCode = 0x31
	OpSYNACK      OpCode = 0x32
	OpDATACK      OpCode = 0x34
	OpFINACK      OpCode = 0x36
	OpPTDATFRGACK OpCode = 0x38
)

func (o OpCode) String() string {
	switch o {
	case OpUnset:
		return "UNSET"
	case OpSYN:
		return "SYN"
	case OpRACK:
		return "RACK"
	case OpDAT:
		return "DAT"
	case OpHBT:
		return "HBT"
	case OpFIN:
		return "FIN"
	case OpRST:
		return "RST"
	case OpACK:
		return "ACK"
	case OpSYNACK:
		return "SYN_ACK"
	case OpDATACK:
		return "DAT_ACK"
	case OpFINACK:
		return "FIN_ACK"
	default:
		return fmt.Sprintf("OpCode(%#x)", uint8(o))
	}
}

// isSequenced reports whether an opcode consumes a sequence number and goes
// through the retransmit channel.
func (o OpCode) isSequenced() bool {
	return o == OpDAT || o == OpDATACK || o == OpFINACK
}

const (
	headerSize = 7
	magic0     = 0xF5
	magic1     = 0x02
	// initialDataSize is the 35-byte connection-prefix block before the first
	// (SYN) packet.
	initialDataSize = 35
)

// packet is a decoded reliable-UDP packet.
type packet struct {
	opcode        OpCode
	local, remote uint32 // 12-bit ack counters
	payload       []byte
}

// synPayload and synAckPayload are the fixed 8-byte opcode payloads.
var (
	synPayload    = []byte{0x12, 0x10, 0x20, 0x20, 0x00, 0x00, 0xA0, 0x00}
	synAckPayload = []byte{0x12, 0x10, 0x20, 0x20, 0x00, 0x01, 0x00, 0x00}
)

// encode serializes the 7-byte header and payload.
func (p packet) encode() []byte {
	out := make([]byte, headerSize+len(p.payload))
	out[0] = magic0
	out[1] = magic1
	// Pack the two 12-bit counters: local low 8 in [2], local high nibble in the
	// high nibble of [3], remote high nibble in the low nibble of [3], remote low
	// 8 in [4].
	out[2] = byte(p.local & 0xFF)
	out[3] = byte(((p.local>>8)&0xF)<<4) | byte((p.remote>>8)&0xF)
	out[4] = byte(p.remote & 0xFF)
	out[5] = byte(p.opcode)
	out[6] = 0xFF
	copy(out[headerSize:], p.payload)
	return out
}

// decodePacket parses a reliable-UDP packet from decrypted bytes.
func decodePacket(b []byte) (packet, error) {
	if len(b) < headerSize {
		return packet{}, fmt.Errorf("rudp: packet %d shorter than header", len(b))
	}
	if b[0] != magic0 || b[1] != magic1 {
		return packet{}, fmt.Errorf("rudp: bad magic %#x %#x", b[0], b[1])
	}
	local := uint32(b[2]) | (uint32(b[3]&0xF0>>4) << 8)
	remote := uint32(b[4]) | (uint32(b[3]&0x0F) << 8)
	p := packet{
		opcode:  OpCode(b[5]),
		local:   local,
		remote:  remote,
		payload: append([]byte(nil), b[headerSize:]...),
	}
	return p, nil
}

// initialData builds the 35-byte connection prefix carrying the steam/player id
// twice (17 bytes each plus a separating zero byte).
func initialData(id string) []byte {
	b := make([]byte, initialDataSize)
	copy(b[0:17], id)
	// b[17] separator stays 0
	copy(b[18:35], id)
	return b
}

// looksLikeInitialData reports whether a decrypted payload is prefixed with the
// connection-prefix block (heuristic from the reference: long enough and not
// starting with a packet magic byte).
func looksLikeInitialData(b []byte) bool {
	return len(b) > initialDataSize && b[0] != 0xF5 && b[0] != 0x25
}
