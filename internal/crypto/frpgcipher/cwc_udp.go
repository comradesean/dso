package frpgcipher

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/sstreight/dso/internal/crypto/cwc"
)

// UDPCipher seals and opens whole game-server datagrams. connectionPrefix marks
// the datagram whose plaintext begins with the 35-byte connection-prefix block
// (the SYN); it is carried in the client->server framing and reported back by
// Open.
type UDPCipher interface {
	Seal(plaintext []byte, connectionPrefix bool) ([]byte, error)
	Open(datagram []byte) (plaintext []byte, connectionPrefix bool, err error)
}

const (
	udpIVLen    = 11
	udpTagLen   = 16
	udpTokenLen = 8
)

// ServerUDPCipher is used by the server: it seals server->client datagrams
// ([IV|tag|ct], AAD=IV) and opens client->server datagrams
// ([token|IV|tag|ptype|ct], AAD=IV|token|ptype).
type ServerUDPCipher struct {
	ctx *cwc.Context
}

// NewServerUDPCipher builds the server-side UDP cipher from the game CWC key.
func NewServerUDPCipher(key []byte) (*ServerUDPCipher, error) {
	ctx, err := cwc.NewContext(key)
	if err != nil {
		return nil, err
	}
	return &ServerUDPCipher{ctx: ctx}, nil
}

func (c *ServerUDPCipher) Seal(pt []byte, _ bool) ([]byte, error) {
	return sealServerToClient(c.ctx, pt)
}

func (c *ServerUDPCipher) Open(in []byte) ([]byte, bool, error) {
	return openClientToServer(c.ctx, in)
}

// ClientUDPCipher is used by the client/emulator: it seals client->server
// datagrams and opens server->client datagrams.
type ClientUDPCipher struct {
	ctx   *cwc.Context
	token [8]byte
}

// NewClientUDPCipher builds the client-side UDP cipher from the game CWC key and
// the 8-byte auth token.
func NewClientUDPCipher(key []byte, token [8]byte) (*ClientUDPCipher, error) {
	ctx, err := cwc.NewContext(key)
	if err != nil {
		return nil, err
	}
	return &ClientUDPCipher{ctx: ctx, token: token}, nil
}

func (c *ClientUDPCipher) Seal(pt []byte, connectionPrefix bool) ([]byte, error) {
	iv := make([]byte, udpIVLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	var ptype byte
	if connectionPrefix {
		ptype = 1
	}
	aad := make([]byte, 0, udpIVLen+udpTokenLen+1)
	aad = append(aad, iv...)
	aad = append(aad, c.token[:]...)
	aad = append(aad, ptype)

	ct, tag := c.ctx.EncryptMessage(iv, aad, pt, udpTagLen)
	out := make([]byte, 0, udpTokenLen+udpIVLen+udpTagLen+1+len(ct))
	out = append(out, c.token[:]...)
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ptype)
	out = append(out, ct...)
	return out, nil
}

func (c *ClientUDPCipher) Open(in []byte) ([]byte, bool, error) {
	return openServerToClient(c.ctx, in)
}

// sealServerToClient produces [IV(11)|tag(16)|ct], AAD = IV.
func sealServerToClient(ctx *cwc.Context, pt []byte) ([]byte, error) {
	iv := make([]byte, udpIVLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	ct, tag := ctx.EncryptMessage(iv, iv, pt, udpTagLen)
	out := make([]byte, 0, udpIVLen+udpTagLen+len(ct))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return out, nil
}

// openServerToClient parses [IV(11)|tag(16)|ct], AAD = IV.
func openServerToClient(ctx *cwc.Context, in []byte) ([]byte, bool, error) {
	if len(in) < udpIVLen+udpTagLen+1 {
		return nil, false, fmt.Errorf("frpgcipher: server->client datagram too short (%d)", len(in))
	}
	iv := in[:udpIVLen]
	tag := in[udpIVLen : udpIVLen+udpTagLen]
	ct := in[udpIVLen+udpTagLen:]
	pt, ok := ctx.DecryptMessage(iv, iv, ct, tag)
	if !ok {
		return nil, false, errors.New("frpgcipher: server->client tag verification failed")
	}
	return pt, false, nil
}

// openClientToServer parses [token(8)|IV(11)|tag(16)|ptype(1)|ct],
// AAD = IV|token|ptype.
func openClientToServer(ctx *cwc.Context, in []byte) ([]byte, bool, error) {
	const minLen = udpTokenLen + udpIVLen + udpTagLen + 1 + 1
	if len(in) < minLen {
		return nil, false, fmt.Errorf("frpgcipher: client->server datagram too short (%d)", len(in))
	}
	token := in[:udpTokenLen]
	iv := in[udpTokenLen : udpTokenLen+udpIVLen]
	tag := in[udpTokenLen+udpIVLen : udpTokenLen+udpIVLen+udpTagLen]
	ptype := in[udpTokenLen+udpIVLen+udpTagLen]
	ct := in[udpTokenLen+udpIVLen+udpTagLen+1:]

	aad := make([]byte, 0, udpIVLen+udpTokenLen+1)
	aad = append(aad, iv...)
	aad = append(aad, token...)
	aad = append(aad, ptype)

	pt, ok := ctx.DecryptMessage(iv, aad, ct, tag)
	if !ok {
		return nil, false, errors.New("frpgcipher: client->server tag verification failed")
	}
	return pt, ptype != 0, nil
}

// TokenFromDatagram extracts the 8-byte auth token that prefixes a
// client->server datagram, for demuxing before the key is known.
func TokenFromDatagram(in []byte) ([8]byte, bool) {
	var t [8]byte
	if len(in) < udpTokenLen {
		return t, false
	}
	copy(t[:], in[:udpTokenLen])
	return t, true
}
