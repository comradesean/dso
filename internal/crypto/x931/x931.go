// Package x931 implements the RSA X9.31 encryption padding used on the Frpg2
// login/auth TCP stream, which Go's crypto/rsa does not expose.
//
// On that stream the server encrypts its replies with a raw RSA private-key
// operation over an X9.31-padded block (OpenSSL's RSA_private_encrypt with
// RSA_X931_PADDING); the client recovers them with the matching public-key
// operation. Inbound traffic (client -> server) uses OAEP, which is stdlib.
//
// The padding and the "use the numerically smaller of s and n-s" rule match
// OpenSSL exactly so that a real game client (the interop reference) accepts
// our output.
package x931

import (
	"crypto/rsa"
	"errors"
	"math/big"
)

var (
	errTooLong  = errors.New("x931: message too long for modulus")
	errBadPad   = errors.New("x931: invalid padding")
	nibbleValid = big.NewInt(0xC)
)

// pad applies X9.31 padding to msg for a k-byte modulus, producing:
//
//	0x6B, 0xBB*(j-1), 0xBA, msg, 0xCC   (j > 0)
//	0x6A, msg, 0xCC                     (j == 0)
//
// where j = k - len(msg) - 2.
func pad(msg []byte, k int) ([]byte, error) {
	j := k - len(msg) - 2
	if j < 0 {
		return nil, errTooLong
	}
	out := make([]byte, 0, k)
	if j == 0 {
		out = append(out, 0x6A)
	} else {
		out = append(out, 0x6B)
		for i := 0; i < j-1; i++ {
			out = append(out, 0xBB)
		}
		out = append(out, 0xBA)
	}
	out = append(out, msg...)
	out = append(out, 0xCC)
	return out, nil
}

// unpad strips X9.31 padding, returning the embedded message.
func unpad(em []byte) ([]byte, error) {
	n := len(em)
	if n < 3 || em[n-1] != 0xCC {
		return nil, errBadPad
	}
	var i int
	switch em[0] {
	case 0x6A:
		i = 1
	case 0x6B:
		i = 1
		for i < n && em[i] == 0xBB {
			i++
		}
		if i >= n || em[i] != 0xBA {
			return nil, errBadPad
		}
		i++
	default:
		return nil, errBadPad
	}
	if i > n-1 {
		return nil, errBadPad
	}
	return em[i : n-1], nil
}

// PrivateEncrypt performs the server-side operation: X9.31-pad msg, raise to the
// private exponent mod n, and return the numerically smaller of s and n-s,
// left-padded to the modulus size.
func PrivateEncrypt(priv *rsa.PrivateKey, msg []byte) ([]byte, error) {
	k := priv.Size()
	em, err := pad(msg, k)
	if err != nil {
		return nil, err
	}
	m := new(big.Int).SetBytes(em)
	if m.Cmp(priv.N) >= 0 {
		return nil, errTooLong
	}
	s := new(big.Int).Exp(m, priv.D, priv.N)
	if diff := new(big.Int).Sub(priv.N, s); s.Cmp(diff) > 0 {
		s = diff
	}
	out := make([]byte, k)
	s.FillBytes(out)
	return out, nil
}

// PublicDecrypt performs the client-side operation (used by the emulator and
// tests): raise sig to the public exponent mod n, undo the smaller-of rule when
// the low nibble is not 0xC, then strip the X9.31 padding.
func PublicDecrypt(pub *rsa.PublicKey, sig []byte) ([]byte, error) {
	k := pub.Size()
	if len(sig) != k {
		return nil, errBadPad
	}
	s := new(big.Int).SetBytes(sig)
	if s.Cmp(pub.N) >= 0 {
		return nil, errBadPad
	}
	e := big.NewInt(int64(pub.E))
	m := new(big.Int).Exp(s, e, pub.N)
	if new(big.Int).And(m, big.NewInt(0xf)).Cmp(nibbleValid) != 0 {
		m.Sub(pub.N, m)
	}
	em := make([]byte, k)
	m.FillBytes(em)
	return unpad(em)
}
