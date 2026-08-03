// Package packet implements the outer Frpg2 TCP packet framing used by the
// login and auth services. On the wire each packet is a big-endian uint16
// length prefix followed by a 12-byte header and the payload:
//
//	[u16 outerLen][Header(12)][payload]   outerLen = 12 + len(payload)
//
// The header carries a per-connection send counter and the payload length
// (twice, as a u32 and a u16). All multi-byte header fields are big-endian.
package packet

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

// MaxPacketLength bounds the outer length prefix (BuildConfig::MAX_PACKET_LENGTH).
const MaxPacketLength = 64 * 1024

// headerSize is the fixed Frpg2PacketHeader size.
const headerSize = 12

// Stream reads and writes Frpg2 packets over a net.Conn.
type Stream struct {
	conn net.Conn
	r    *bufio.Reader

	wmu  sync.Mutex
	sent uint16
}

// NewStream wraps a connection.
func NewStream(conn net.Conn) *Stream {
	return &Stream{conn: conn, r: bufio.NewReader(conn)}
}

// Conn returns the underlying connection (for deadlines, close, remote addr).
func (s *Stream) Conn() net.Conn { return s.conn }

// ReadPacket reads one packet and returns its payload (the bytes after the
// 12-byte header).
func (s *Stream) ReadPacket() ([]byte, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(s.r, lenBuf[:]); err != nil {
		return nil, err
	}
	outerLen := int(binary.BigEndian.Uint16(lenBuf[:]))
	if outerLen == 0 {
		return nil, fmt.Errorf("packet: zero-length packet")
	}
	if outerLen > MaxPacketLength {
		return nil, fmt.Errorf("packet: length %d exceeds max %d", outerLen, MaxPacketLength)
	}
	if outerLen < headerSize {
		return nil, fmt.Errorf("packet: length %d smaller than header", outerLen)
	}
	buf := make([]byte, outerLen)
	if _, err := io.ReadFull(s.r, buf); err != nil {
		return nil, err
	}
	// buf = [Header(12)][payload]. Validate the declared payload length.
	payloadLen := int(binary.BigEndian.Uint32(buf[4:8]))
	if payloadLen != outerLen-headerSize {
		return nil, fmt.Errorf("packet: declared payload length %d != actual %d", payloadLen, outerLen-headerSize)
	}
	return buf[headerSize:], nil
}

// WritePacket frames and writes a payload as one packet.
func (s *Stream) WritePacket(payload []byte) error {
	if len(payload) > MaxPacketLength-headerSize {
		return fmt.Errorf("packet: payload length %d too large", len(payload))
	}
	s.wmu.Lock()
	defer s.wmu.Unlock()

	s.sent++
	out := make([]byte, 2+headerSize+len(payload))
	binary.BigEndian.PutUint16(out[0:2], uint16(headerSize+len(payload)))
	// Header:
	binary.BigEndian.PutUint16(out[2:4], s.sent) // send_counter
	// out[4], out[5] unknown_1/2 = 0
	binary.BigEndian.PutUint32(out[6:10], uint32(len(payload))) // payload_length
	// out[10:12] unknown_3 = 0
	binary.BigEndian.PutUint16(out[12:14], uint16(len(payload))) // payload_length_short
	copy(out[14:], payload)

	_, err := s.conn.Write(out)
	return err
}
