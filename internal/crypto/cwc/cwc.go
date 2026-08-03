// Package cwc implements the AES-CWC-128 authenticated encryption mode used by
// FromSoftware's "Frpg2" game servers (Dark Souls 2/3). It is a clean-room Go
// port of the integer path of Brian Gladman's CWC reference implementation,
// validated against that project's published test vectors.
//
// CWC combines AES in counter mode with a Carter-Wegman polynomial hash over
// GF(2^127-1). This package exposes whole-message encrypt/decrypt; the various
// wire framings (TCP vs UDP, client vs server) are layered on top elsewhere.
package cwc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/subtle"
	"fmt"
	"math/big"
)

// Context holds the key schedule and derived hash key for one CWC key. It is
// safe for concurrent use: all per-message state is kept in locals.
type Context struct {
	block cipher.Block
	z     *big.Int // hash key Z
}

// NewContext creates a CWC context from a 16, 24, or 32-byte AES key.
func NewContext(key []byte) (*Context, error) {
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("cwc: invalid key length %d (want 16, 24, or 32)", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c := &Context{block: block}

	// Derive the hash key Z = AES_K(0xC0 || 0x00*15), with the top bit of the
	// first byte cleared, read as a 128-bit big-endian integer.
	var zv [16]byte
	zv[0] = 0xC0
	c.block.Encrypt(zv[:], zv[:])
	zv[0] &= 0x7f
	c.z = new(big.Int).SetBytes(zv[:])
	return c, nil
}

// msgState is the mutable per-message hashing/counter state.
type msgState struct {
	hash polyHash
	ctr  [16]byte
}

// initMessage sets up the counter block and zeroes the hash. iv must be 11 bytes.
func (c *Context) initMessage(s *msgState, iv []byte) {
	s.hash = newPolyHash(c.z)
	s.ctr = [16]byte{}
	s.ctr[0] = 0x80
	copy(s.ctr[1:12], iv[:11])
	// counter field (bytes 12..15) already zero
}

// beIncCtr increments the 32-bit big-endian counter at offset 12.
func (s *msgState) beIncCtr() {
	for i := 15; i >= 12; i-- {
		s.ctr[i]++
		if s.ctr[i] != 0 {
			break
		}
	}
}

// absorb feeds a byte slice into the hash as a sequence of full 12-byte blocks,
// with any trailing partial block zero-padded to 12 bytes. This matches the
// reference behaviour where the header's and the ciphertext's trailing partial
// blocks are each finalized as their own padded block.
func (c *Context) absorb(s *msgState, data []byte) {
	n := len(data)
	off := 0
	for n-off >= ablkLen {
		s.hash.update(data[off : off+ablkLen])
		off += ablkLen
	}
	if rem := n - off; rem > 0 {
		var blk [ablkLen]byte
		copy(blk[:], data[off:])
		s.hash.update(blk[:])
	}
}

// crypt applies the AES-CTR keystream to data in place. The counter is
// incremented before each 16-byte block (so the first block uses counter 1).
func (c *Context) crypt(s *msgState, data []byte) {
	var ks [cblkLen]byte
	for off := 0; off < len(data); off += cblkLen {
		s.beIncCtr()
		c.block.Encrypt(ks[:], s.ctr[:])
		end := off + cblkLen
		if end > len(data) {
			end = len(data)
		}
		for i := off; i < end; i++ {
			data[i] ^= ks[i-off]
		}
	}
}

// computeTag finalizes the hash and produces a tag of tagLen bytes. hdrLen and
// dataLen are the byte lengths of the authenticated header and ciphertext.
func (c *Context) computeTag(s *msgState, hdrLen, dataLen int, tagLen int) []byte {
	// Fold in the length block and serialize to 16 big-endian bytes.
	hb := s.hash.finalize(hdrLen, dataLen)

	// Encrypt the final hash value with AES.
	c.block.Encrypt(hb[:], hb[:])

	// S = AES(counter block with the counter field zeroed).
	var sblk [16]byte
	copy(sblk[:], s.ctr[:])
	sblk[12], sblk[13], sblk[14], sblk[15] = 0, 0, 0, 0
	c.block.Encrypt(sblk[:], sblk[:])

	tag := make([]byte, tagLen)
	for i := 0; i < tagLen; i++ {
		tag[i] = hb[i] ^ sblk[i]
	}
	return tag
}

// EncryptMessage encrypts msg in place-style and returns the ciphertext and an
// authentication tag of tagLen bytes. iv must be 11 bytes; hdr is authenticated
// but not encrypted (additional authenticated data) and may be empty.
func (c *Context) EncryptMessage(iv, hdr, msg []byte, tagLen int) (ciphertext, tag []byte) {
	ct := make([]byte, len(msg))
	copy(ct, msg)

	var s msgState
	c.initMessage(&s, iv)
	c.absorb(&s, hdr)
	// finalize header block boundary vs data: absorb() already padded the
	// header's partial block, so ciphertext absorption starts on a fresh block.
	c.crypt(&s, ct)
	c.absorb(&s, ct)
	tag = c.computeTag(&s, len(hdr), len(ct), tagLen)
	return ct, tag
}

// DecryptMessage authenticates and decrypts ct in place-style. It returns the
// recovered plaintext and whether the tag verified. iv must be 11 bytes.
func (c *Context) DecryptMessage(iv, hdr, ct, tag []byte) (plaintext []byte, ok bool) {
	var s msgState
	c.initMessage(&s, iv)
	c.absorb(&s, hdr)
	c.absorb(&s, ct)
	want := c.computeTag(&s, len(hdr), len(ct), len(tag))
	if subtle.ConstantTimeCompare(want, tag) != 1 {
		return nil, false
	}
	pt := make([]byte, len(ct))
	copy(pt, ct)
	c.crypt(&s, pt)
	return pt, true
}
