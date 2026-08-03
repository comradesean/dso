package rudp

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

// Message-layer opcodes shared across games. Individual games extend msg_type
// with their own request opcodes.
const (
	MsgReply uint32 = 0x0000
	MsgPush  uint32 = 0x0320

	// PushIndex is the msg_index used for push messages.
	PushIndex uint32 = 0xFFFFFFFF
)

const (
	msgHeaderSize      = 12
	msgRespHeaderSize  = 16
	maxFragmentLength  = 900
	minSizeForCompress = 512
)

// Message is a decoded game message.
type Message struct {
	Type    uint32 // request opcode, MsgReply, or MsgPush
	Index   uint32
	IsReply bool
	Payload []byte // protobuf bytes

	ackSeq uint32 // sequence to acknowledge when replying to this message
}

// encodeMessage builds the message-layer bytes: 12-byte header, an optional
// 16-byte response header for replies, then the body.
func encodeMessage(m Message) []byte {
	size := msgHeaderSize + len(m.Payload)
	if m.IsReply {
		size += msgRespHeaderSize
	}
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], msgHeaderSize)
	binary.BigEndian.PutUint32(buf[4:8], m.Type)
	binary.LittleEndian.PutUint32(buf[8:12], m.Index)
	off := msgHeaderSize
	if m.IsReply {
		// Response header {0,1,0,0}, NATIVE little-endian (no byte swap), unlike
		// the TCP response header.
		binary.LittleEndian.PutUint32(buf[12:16], 0)
		binary.LittleEndian.PutUint32(buf[16:20], 1)
		binary.LittleEndian.PutUint32(buf[20:24], 0)
		binary.LittleEndian.PutUint32(buf[24:28], 0)
		off += msgRespHeaderSize
	}
	copy(buf[off:], m.Payload)
	return buf
}

// decodeMessage parses reassembled message-layer bytes.
func decodeMessage(b []byte, ackSeq uint32) (Message, error) {
	if len(b) < msgHeaderSize {
		return Message{}, fmt.Errorf("rudp: message %d shorter than header", len(b))
	}
	msgType := binary.BigEndian.Uint32(b[4:8])
	index := binary.LittleEndian.Uint32(b[8:12])
	off := msgHeaderSize
	isReply := msgType == MsgReply
	if isReply {
		if len(b) < msgHeaderSize+msgRespHeaderSize {
			return Message{}, fmt.Errorf("rudp: reply message %d shorter than headers", len(b))
		}
		off += msgRespHeaderSize
	}
	return Message{
		Type:    msgType,
		Index:   index,
		IsReply: isReply,
		Payload: append([]byte(nil), b[off:]...),
		ackSeq:  ackSeq,
	}, nil
}

// encodeFragments splits a message payload into one or more fragment packets.
// For Milestone 1 outbound compression is disabled; payloads are small.
func encodeFragments(payload []byte, counter uint16) [][]byte {
	var frags [][]byte
	count := (len(payload) + maxFragmentLength - 1) / maxFragmentLength
	if count == 0 {
		count = 1 // still emit a single (empty) fragment
	}
	for i := 0; i < count; i++ {
		start := i * maxFragmentLength
		end := start + maxFragmentLength
		if end > len(payload) {
			end = len(payload)
		}
		chunk := payload[start:end]

		hdr := make([]byte, 12)
		binary.LittleEndian.PutUint16(hdr[0:2], counter) // packet_counter is little-endian
		hdr[2] = 0                                       // compress_flag = 0 (M1)
		// hdr[3:6] unknown = 0
		binary.BigEndian.PutUint16(hdr[6:8], uint16(len(payload))) // total_payload_length
		// hdr[8] unknown = 0
		hdr[9] = byte(i)                                           // fragment_index
		binary.BigEndian.PutUint16(hdr[10:12], uint16(len(chunk))) // fragment_length

		frag := make([]byte, 0, 12+len(chunk))
		frag = append(frag, hdr...)
		frag = append(frag, chunk...)
		frags = append(frags, frag)
	}
	return frags
}

// reassembler accumulates fragment packets into complete message payloads.
type reassembler struct {
	buf       []byte
	received  int
	total     int
	compress  bool
	decompLen uint32
	active    bool
}

// push adds one fragment; when the message is complete it returns the full
// (decompressed) payload.
func (r *reassembler) push(frag []byte) ([]byte, bool, error) {
	if len(frag) < 12 {
		return nil, false, fmt.Errorf("rudp: fragment %d shorter than header", len(frag))
	}
	compressFlag := frag[2] != 0
	total := int(binary.BigEndian.Uint16(frag[6:8]))
	fragIndex := frag[9]
	fragLen := int(binary.BigEndian.Uint16(frag[10:12]))

	off := 12
	if compressFlag && fragIndex == 0 {
		if len(frag) < off+4 {
			return nil, false, fmt.Errorf("rudp: compressed fragment missing length")
		}
		r.decompLen = binary.BigEndian.Uint32(frag[off : off+4])
		off += 4
	}
	body := frag[off:]
	if len(body) < fragLen {
		return nil, false, fmt.Errorf("rudp: fragment body %d shorter than declared %d", len(body), fragLen)
	}
	body = body[:fragLen]

	if !r.active {
		r.active = true
		r.total = total
		r.compress = compressFlag
		r.buf = nil
		r.received = 0
	}
	r.buf = append(r.buf, body...)
	r.received += fragLen

	if r.received < r.total {
		return nil, false, nil
	}

	// Complete.
	payload := r.buf
	compressed := r.compress
	decompLen := r.decompLen
	*r = reassembler{}

	if compressed {
		out, err := zlibInflate(payload, int(decompLen))
		if err != nil {
			return nil, false, err
		}
		payload = out
	}
	return payload, true, nil
}

func zlibInflate(data []byte, expected int) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("rudp: zlib open: %w", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("rudp: zlib read: %w", err)
	}
	if expected > 0 && len(out) != expected {
		return nil, fmt.Errorf("rudp: decompressed %d bytes, expected %d", len(out), expected)
	}
	return out, nil
}

// MessageConn layers the fragment and message framing over a Session, exposing
// whole game messages.
type MessageConn struct {
	sess    *Session
	reasm   reassembler
	counter uint16
	nextIdx uint32
}

// NewMessageConn wraps a session.
func NewMessageConn(sess *Session) *MessageConn { return &MessageConn{sess: sess} }

// Session returns the underlying session.
func (mc *MessageConn) Session() *Session { return mc.sess }

// send fragments and transmits a message; when reply is true the first fragment
// is a DAT_ACK acknowledging ackSeq.
func (mc *MessageConn) send(m Message, reply bool, ackSeq uint32) {
	body := encodeMessage(m)
	frags := encodeFragments(body, mc.counter)
	mc.counter++
	for i, f := range frags {
		if reply && i == 0 {
			mc.sess.SendDataAck(f, ackSeq)
		} else {
			mc.sess.SendData(f)
		}
	}
}

// SendRequest sends a request with the given opcode.
func (mc *MessageConn) SendRequest(msgType uint32, payload []byte) {
	mc.nextIdx++
	mc.send(Message{Type: msgType, Index: mc.nextIdx, Payload: payload}, false, 0)
}

// SendReply sends a reply to a received request, copying its index.
func (mc *MessageConn) SendReply(inReplyTo Message, payload []byte) {
	mc.send(Message{Type: MsgReply, Index: inReplyTo.Index, IsReply: true, Payload: payload}, true, inReplyTo.ackSeq)
}

// SendPush sends a server push (type determined by the protobuf's first field).
func (mc *MessageConn) SendPush(payload []byte) {
	mc.send(Message{Type: MsgPush, Index: PushIndex, Payload: payload}, false, 0)
}

// Recv drains the session, reassembles fragments, and returns the next complete
// message if one is ready.
func (mc *MessageConn) Recv() (Message, bool, error) {
	for {
		data, ackSeq, ok := mc.sess.Receive()
		if !ok {
			return Message{}, false, nil
		}
		payload, complete, err := mc.reasm.push(data)
		if err != nil {
			return Message{}, false, err
		}
		if !complete {
			continue
		}
		msg, err := decodeMessage(payload, ackSeq)
		if err != nil {
			return Message{}, false, err
		}
		return msg, true, nil
	}
}
