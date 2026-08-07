package game

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"
)

// The Majula obelisk's text is string id 100 of the regulation FMG — a file that
// exists to hold that one string and nothing else, in eleven per-language
// copies. Offline it reads "The letters are worn beyond recognition."
//
// Replacing it means pushing a whole FMG over 0x038B, which is why this builds
// one from scratch rather than editing bytes in a file: asking an operator to
// hex-edit a binary to change a sentence is friction with no payoff. The general
// mechanism (RegulationPushFile) is still there for arbitrary payloads.
//
// See tasks/regulation-push-038b.md for how the key and the route were found.
const (
	// obeliskResourceName is the key the client registered the resource under.
	//
	// Bare, and NOT the BND4 entry name "regulationEnglish.fmg" nor the load path
	// "text:/Text/English/regulation.fmg". Read out of live memory at the
	// resource object's +4; the repository bucket was walked and holds exactly one
	// resource under this name, which is what makes pushing to it safe.
	obeliskResourceName = "regulation.fmg"

	// obeliskStringID is the FMG string id the obelisk reads.
	obeliskStringID = 100

	// obeliskHeaderLen is where the text starts. Everything before it is fixed.
	obeliskHeaderLen = 0x2C

	// obeliskMaxBytes is the hard cap on the payload.
	//
	// Two independent limits happen to agree. The applier refuses anything larger
	// (0x76A0F0 and 0x76BB30 both `cmpwi r5,1024; ble`), and the destination
	// buffer for this particular resource is exactly 1024 bytes — read live from
	// the resource object's +156, which holds 0x400 for a 128-byte file.
	//
	// That second limit is the one that matters, because NEITHER route compares
	// the payload against the destination. The 1024 gate is a payload gate, not a
	// destination gate: overrun it and the client dies on the next read.
	obeliskMaxBytes = 1024
)

// obeliskHeader is the FMG header, byte-for-byte from the stock
// regulationEnglish.fmg and verified against the client's live buffer at guest
// 0x312883F0. Only the size at +0x04 varies with the text.
//
//	+0x00  00010000  version/flags
//	+0x04  size      total file length, rewritten below
//	+0x08  01ff0000
//	+0x0c  00000001  group count
//	+0x10  00000001  string count
//	+0x14  00000028  offset of the string-offset table; the applier relocates
//	                 this one field in place (*(dst+20) += dst), which is the
//	                 whole of its "diff" handling
//	+0x18  00000000
//	+0x1c  00000000
//	+0x20  id        first id in the group
//	+0x24  id        last id in the group
//	+0x28  0000002c  offset of string 0
var obeliskHeader = [obeliskHeaderLen]byte{
	0x00, 0x01, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x80,
	0x01, 0xff, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x01,
	0x00, 0x00, 0x00, 0x28,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x64,
	0x00, 0x00, 0x00, 0x64,
	0x00, 0x00, 0x00, 0x2c,
}

// buildObeliskFMG returns a complete regulation FMG carrying text as string 100.
//
// The text is UTF-16BE, as all PS3 game text is. `\n` in the configured value
// starts a new line, since an env var cannot carry a real one.
//
// Oversize is an error rather than a truncation. A silently shortened message
// would look exactly like a working one, and this project has already paid for
// that class of failure more than once.
func buildObeliskFMG(text string) ([]byte, error) {
	units := utf16.Encode([]rune(strings.ReplaceAll(text, `\n`, "\n")))

	// Terminator, then pad so the file length stays a round number as the stock
	// file's does. Nothing reads the padding; it just keeps a dump legible.
	size := obeliskHeaderLen + (len(units)+1)*2
	if rem := size % 16; rem != 0 {
		size += 16 - rem
	}
	if size > obeliskMaxBytes {
		return nil, fmt.Errorf("obelisk text is %d characters, which needs %d bytes; "+
			"the client's buffer for this resource holds %d",
			len(units), size, obeliskMaxBytes)
	}

	out := make([]byte, size)
	copy(out, obeliskHeader[:])
	binary.BigEndian.PutUint32(out[0x04:], uint32(size))
	binary.BigEndian.PutUint32(out[0x20:], obeliskStringID)
	binary.BigEndian.PutUint32(out[0x24:], obeliskStringID)
	for i, u := range units {
		binary.BigEndian.PutUint16(out[obeliskHeaderLen+i*2:], u)
	}
	return out, nil
}

// obeliskPush returns the push replacing the obelisk text, or nothing if no text
// is configured.
func (s *Service) obeliskPush(log logger) []resourcePush {
	text := s.srv.Config.ObeliskText
	if text == "" {
		return nil
	}

	data, err := buildObeliskFMG(text)
	if err != nil {
		log.Warn("obelisk: cannot build FMG", "err", err)
		return nil
	}
	return []resourcePush{{path: obeliskResourceName, data: data}}
}
