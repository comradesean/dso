package cwc

import "math/big"

// The CWC authentication tag is built from a Carter-Wegman polynomial hash over
// GF(2^127 - 1). For each 12-byte (96-bit) block m the running hash updates as
// h = (h + m) * Z, followed by the reference implementation's *lazy* reduction
// rather than a full modular reduction.
//
// That distinction is load-bearing. The reference (Brian Gladman's aes_modes
// cwc.c, the same code the game speaks) reduces by folding 2^128 == 2 and
// 2^127 == 1 with a small fixed number of conditional corrections, all inside a
// 128-bit register with carry-out discarded. The representative it lands on is
// not always the canonical one in [0, 2^127-1), and that exact representative
// is what gets encrypted into the tag. A full `mod p` here yields a different
// tag for the same input, so this must mirror the reference step for step.
//
// Conventions, all verified against cwc.c built from ref/ds3so:
//   - Z is the 16-byte AES output read big-endian, with the top bit cleared.
//     (The reference bswap32s it, which on a little-endian host is exactly a
//     plain big-endian read of the AES output bytes.)
//   - A 12-byte block is read as three *native-endian* uint32 words dropped
//     into big-endian word positions: do_cwc() does data[1]=in[0], data[2]=in[1],
//     data[3]=in[2] with no bswap, where `in` is the buffer cast to uint32*.
//     So each 4-byte group is byte-reversed relative to a plain big-endian read
//     — unlike Z, which *is* swapped. Getting this wrong is what made every tag
//     differ while ciphertext stayed correct.
//   - The length block folded in at the end is hdrLen*2^64 + dataLen.

// ablkLen is the authentication block size (CWC_ABLK_SIZE): 12 bytes.
const ablkLen = 12

// cblkLen is the cipher block size (CWC_CBLK_SIZE / AES block): 16 bytes.
const cblkLen = 16

var (
	// mask128 keeps values in a 128-bit register, discarding carry-out exactly
	// as the reference's add_4 does.
	mask128 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))
	// bit127 is the 2^127 bit the reference tests and folds back in as +1,
	// since 2^127 == 1 (mod 2^127-1).
	bit127 = new(big.Int).Lsh(big.NewInt(1), 127)
	one    = big.NewInt(1)
)

// swapWords reverses the bytes within each 4-byte group of an authentication
// block, converting the reference's native-endian word load into a plain
// big-endian integer. See the block-convention note above.
func swapWords(buf []byte) []byte {
	out := make([]byte, len(buf))
	for i := 0; i+4 <= len(buf); i += 4 {
		out[i+0] = buf[i+3]
		out[i+1] = buf[i+2]
		out[i+2] = buf[i+1]
		out[i+3] = buf[i+0]
	}
	return out
}

// polyHash is the running Carter-Wegman hash state for one message.
type polyHash struct {
	z *big.Int // the hash key Z
	h *big.Int // running accumulator, always < 2^128
}

func newPolyHash(z *big.Int) polyHash {
	return polyHash{z: z, h: new(big.Int)}
}

// fold127 mirrors the reference's
//
//	if(x & 0x80000000) { x &= 0x7fffffff; be_inc(x) }
//
// correction: clear the 2^127 bit and add one, because 2^127 == 1 (mod p).
func fold127(x *big.Int) *big.Int {
	if x.Bit(127) == 1 {
		x.AndNot(x, bit127)
		x.Add(x, one)
		x.And(x, mask128)
	}
	return x
}

// update absorbs one 12-byte block, reproducing do_cwc():
//
//	data = block + h        (128-bit, carry discarded)
//	prod = data * Z         (256-bit)
//	h    = 2*hi + lo        (with the 2^127 folds)
func (p *polyHash) update(buf []byte) {
	data := new(big.Int).SetBytes(swapWords(buf)) // 12 bytes -> low 96 bits
	data.Add(data, p.h)
	data.And(data, mask128)

	prod := new(big.Int).Mul(data, p.z)
	hi := new(big.Int).Rsh(prod, 128)
	lo := new(big.Int).And(prod, mask128)

	// h = 2*hi, carry-out discarded.
	h := hi.Lsh(hi, 1)
	h.And(h, mask128)

	// If lo carries the 2^127 bit, clear it and carry +1 into h.
	if lo.Bit(127) == 1 {
		lo.AndNot(lo, bit127)
		h.Add(h, one)
		h.And(h, mask128)
	}

	h.Add(h, lo)
	h.And(h, mask128)

	p.h = fold127(h)
}

// finalize folds in the length block and returns the 16-byte big-endian value
// fed to AES to produce the tag. Mirrors the USE_LONGS branch of
// cwc_compute_tag: hh = {0, hdrLen, 0, dataLen} added to the hash, then a
// single 2^127 correction.
func (p *polyHash) finalize(hdrLen, dataLen int) [16]byte {
	lenBlk := new(big.Int).SetUint64(uint64(hdrLen))
	lenBlk.Lsh(lenBlk, 64)
	lenBlk.Or(lenBlk, new(big.Int).SetUint64(uint64(dataLen)))

	p.h.Add(p.h, lenBlk)
	p.h.And(p.h, mask128)
	p.h = fold127(p.h)

	var out [16]byte
	p.h.FillBytes(out[:]) // big-endian, left-padded to 16 bytes
	return out
}
