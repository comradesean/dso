package frpgcipher

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/sstreight/dso/internal/crypto/cwc"
)

// tcpIVLen and tcpTagLen are the framing sizes for the authenticated TCP stream.
const (
	tcpIVLen  = 11
	tcpTagLen = 16
)

// cwcTCP is the AES-CWC cipher used on the auth TCP stream after the RSA
// handshake. Each message is framed as [IV(11) | tag(16) | ciphertext], and the
// IV doubles as the additional authenticated data.
type cwcTCP struct {
	ctx *cwc.Context
}

// NewCWCTCP builds the TCP CWC cipher for a 16-byte key. The returned value is
// used for both the encrypt and decrypt directions (state is per-message).
func NewCWCTCP(key []byte) (cipher, error) {
	ctx, err := cwc.NewContext(key)
	if err != nil {
		return nil, err
	}
	return cwcTCP{ctx: ctx}, nil
}

func (c cwcTCP) Encrypt(pt []byte) ([]byte, error) {
	iv := make([]byte, tcpIVLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	ct, tag := c.ctx.EncryptMessage(iv, iv, pt, tcpTagLen)
	out := make([]byte, 0, tcpIVLen+tcpTagLen+len(ct))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return out, nil
}

func (c cwcTCP) Decrypt(in []byte) ([]byte, error) {
	if len(in) < tcpIVLen+tcpTagLen+1 {
		return nil, fmt.Errorf("frpgcipher: cwc-tcp input too short (%d)", len(in))
	}
	iv := in[:tcpIVLen]
	tag := in[tcpIVLen : tcpIVLen+tcpTagLen]
	ct := in[tcpIVLen+tcpTagLen:]
	pt, ok := c.ctx.DecryptMessage(iv, iv, ct, tag)
	if !ok {
		return nil, errors.New("frpgcipher: cwc-tcp tag verification failed")
	}
	return pt, nil
}
