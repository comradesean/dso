// Package message implements the Frpg2 message layer that sits inside the TCP
// packet payload for the login and auth services. A message is a 12-byte header
// (header_size, msg_type, msg_index), an optional 16-byte response header when
// the message is a Reply, and an encrypted body.
//
// The cipher is swappable at runtime: the auth stream begins under RSA, drops to
// plaintext for the 27-byte key-exchange blob, then switches to AES-CWC.
package message

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/sstreight/dso/internal/frpg/packet"
)

// Type is the Frpg2 login/auth message type (Frpg2MessageType).
type Type uint32

const (
	Reply                       Type = 0x0
	KeyMaterial                 Type = 0x1
	GetServiceStatus            Type = 0x2
	PSNTicket                   Type = 0x3
	RequestQueryLoginServerInfo Type = 0x5
	RequestHandshake            Type = 0x6
)

func (t Type) String() string {
	switch t {
	case Reply:
		return "Reply"
	case KeyMaterial:
		return "KeyMaterial"
	case GetServiceStatus:
		return "GetServiceStatus"
	case PSNTicket:
		return "PSNTicket"
	case RequestQueryLoginServerInfo:
		return "RequestQueryLoginServerInfo"
	case RequestHandshake:
		return "RequestHandshake"
	default:
		return fmt.Sprintf("Type(%#x)", uint32(t))
	}
}

const (
	msgHeaderSize  = 12
	respHeaderSize = 16
)

// Cipher encrypts and decrypts message bodies. Implementations live in the
// crypto packages; a nil Cipher means the body is sent in the clear.
type Cipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// Message is a decoded Frpg2 message. Type == Reply indicates a response (which
// carries the extra response header on the wire); Index is the msg_index, which
// a reply copies from the request it answers.
type Message struct {
	Type    Type
	Index   uint32
	Payload []byte // decrypted body
}

// Stream is a bidirectional Frpg2 message stream over a connection.
type Stream struct {
	pkt      *packet.Stream
	enc, dec Cipher
}

// NewStream wraps a connection. It starts with no cipher (plaintext); callers
// install ciphers via SetCiphers.
func NewStream(conn net.Conn) *Stream {
	return &Stream{pkt: packet.NewStream(conn)}
}

// Conn returns the underlying connection.
func (s *Stream) Conn() net.Conn { return s.pkt.Conn() }

// SetCiphers installs the encrypt (outbound) and decrypt (inbound) ciphers.
// Either may be nil to send/receive that direction in the clear.
func (s *Stream) SetCiphers(enc, dec Cipher) {
	s.enc, s.dec = enc, dec
}

// Send encodes and writes a message. For a Reply, pass Type Reply and the
// request's Index; the response header is emitted automatically.
func (s *Stream) Send(msg Message) error {
	body := msg.Payload
	if s.enc != nil {
		enc, err := s.enc.Encrypt(body)
		if err != nil {
			return fmt.Errorf("message: encrypt: %w", err)
		}
		body = enc
	}

	size := msgHeaderSize + len(body)
	if msg.Type == Reply {
		size += respHeaderSize
	}
	buf := make([]byte, size)

	binary.BigEndian.PutUint32(buf[0:4], msgHeaderSize) // header_size = 12
	binary.BigEndian.PutUint32(buf[4:8], uint32(msg.Type))
	binary.LittleEndian.PutUint32(buf[8:12], msg.Index) // msg_index is little-endian

	off := msgHeaderSize
	if msg.Type == Reply {
		// Response header {0, 1, 0, 0} big-endian.
		binary.BigEndian.PutUint32(buf[12:16], 0)
		binary.BigEndian.PutUint32(buf[16:20], 1)
		binary.BigEndian.PutUint32(buf[20:24], 0)
		binary.BigEndian.PutUint32(buf[24:28], 0)
		off += respHeaderSize
	}
	copy(buf[off:], body)

	return s.pkt.WritePacket(buf)
}

// Recv reads and decodes the next message.
func (s *Stream) Recv() (Message, error) {
	payload, err := s.pkt.ReadPacket()
	if err != nil {
		return Message{}, err
	}
	if len(payload) < msgHeaderSize {
		return Message{}, fmt.Errorf("message: payload %d shorter than header", len(payload))
	}

	// header_size (buf[0:4]) is always 12; msg_type is big-endian, index LE.
	msgType := Type(binary.BigEndian.Uint32(payload[4:8]))
	index := binary.LittleEndian.Uint32(payload[8:12])

	off := msgHeaderSize
	if msgType == Reply {
		if len(payload) < msgHeaderSize+respHeaderSize {
			return Message{}, fmt.Errorf("message: reply payload %d shorter than headers", len(payload))
		}
		off += respHeaderSize
	}

	body := payload[off:]
	if s.dec != nil && len(body) > 0 {
		dec, err := s.dec.Decrypt(body)
		if err != nil {
			return Message{}, fmt.Errorf("message: decrypt: %w", err)
		}
		body = dec
	}

	return Message{Type: msgType, Index: index, Payload: body}, nil
}
