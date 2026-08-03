package cwc

import "math/big"

// The CWC authentication tag is built from a Carter-Wegman polynomial hash over
// the prime field GF(2^127 - 1). For each 12-byte (96-bit) block m the running
// hash updates as h = ((h + m) * Z) mod (2^127 - 1), where Z is the 128-bit hash
// key derived from the AES key. This is implemented with math/big for clarity
// and exactness; message sizes here are tiny so performance is a non-issue.
//
// Block/word conventions (verified empirically against the reference's test
// vectors by recovering its true final-hash bytes):
//   - Z is the 16-byte AES output read as a big-endian integer (top bit cleared).
//   - A 12-byte block m is read as a big-endian integer.
//   - The length block folded in at the end is hdrLen*2^64 + dataLen.

// ablkLen is the authentication block size (CWC_ABLK_SIZE): 12 bytes.
const ablkLen = 12

// cblkLen is the cipher block size (CWC_CBLK_SIZE / AES block): 16 bytes.
const cblkLen = 16

// prime is 2^127 - 1.
var prime = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))

// polyHash is the running Carter-Wegman hash state for one message.
type polyHash struct {
	z *big.Int // the hash key Z
	h *big.Int // running accumulator
}

func newPolyHash(z *big.Int) polyHash {
	return polyHash{z: z, h: new(big.Int)}
}

// blockToInt converts a 12-byte auth block to its field element (big-endian).
func blockToInt(buf []byte) *big.Int {
	return new(big.Int).SetBytes(buf)
}

// update absorbs one 12-byte block: h = ((h + block) * Z) mod p.
func (p *polyHash) update(buf []byte) {
	p.h.Add(p.h, blockToInt(buf))
	p.h.Mul(p.h, p.z)
	p.h.Mod(p.h, prime)
}

// finalize folds in the length block and returns the 16-byte big-endian value
// that is fed to AES to produce the tag.
func (p *polyHash) finalize(hdrLen, dataLen int) [16]byte {
	lenBlk := new(big.Int).SetUint64(uint64(hdrLen))
	lenBlk.Lsh(lenBlk, 64).Or(lenBlk, new(big.Int).SetUint64(uint64(dataLen)))
	p.h.Add(p.h, lenBlk)
	p.h.Mod(p.h, prime)

	var out [16]byte
	p.h.FillBytes(out[:]) // big-endian, left-padded to 16 bytes
	return out
}
