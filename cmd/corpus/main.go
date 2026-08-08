// corpus decrypts captured Frpg2 sessions and files every message on disk by
// opcode, so the protocol can be read as a whole rather than a datagram at a
// time.
//
// It does what decodecap does not: reassembles fragmented messages and inflates
// the compressed ones. That matters more than it sounds. Server replies of any
// size arrive compressed, so a per-datagram view shows almost no server opcodes
// at all — which is exactly the misleading picture we had before this existed.
//
// Message framing inside the reliable-UDP body, confirmed against captures and
// matching internal/frpg/rudp/message.go:
//
//	[0:2]   message id, shared by every fragment of one message
//	[2:6]   flags; byte 2 non-zero means the payload is zlib-compressed
//	[6:8]   total length across all fragments
//	[9]     fragment index
//	[10:12] fragment length
//	[12:]   payload — for compressed fragment 0, a u32 inflated size comes first
//
// and the reassembled payload is:
//
//	[0:4]   header size (12)
//	[4:8]   opcode
//	[8:]    protobuf
//
// Usage:
//
//	corpus -out DIR  -session NAME -key HEX  < tagged-datagrams
//
// where the input is `tools/pcap/udpdump.py --tagged` output.
package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
)

type message struct {
	dir      string
	opcode   uint32
	payload  []byte
	protoOff int
	clean    bool
	push     bool

	// tsFirst/tsLast are the capture times of this message's first and last
	// FRAGMENT, in Unix seconds with microsecond resolution, or 0 when the input
	// carried no timestamps.
	//
	// Both, not one, because a message is reassembled from datagrams and the
	// spread between them is itself data — a 1660-byte reply arriving as two
	// fragments tells you something about the link that a single number hides.
	// For rate questions (the ~20.5s auto-summon poll, whether 0x038C's periods
	// drive anything, how long the server took to turn a trigger into a push)
	// tsLast is the moment the message actually existed.
	tsFirst float64
	tsLast  float64

	// index is the message-layer Index at full[8:12], LITTLE-endian while the two
	// fields before it are big-endian. A reply carries the Index of the request it
	// answers (rudp.MessageConn.SendReply), so it is the only thing that pairs a
	// response with its request — and responses are half this corpus, filed under
	// opcode 0 because their header has no opcode of its own.
	index    uint32
	hasIndex bool

	// repliesTo is the request opcode this message answers, resolved after the
	// whole session is read. Zero when unmatched.
	repliesTo uint32
}

// pushIndex is the message Index every push carries instead of a real one, so a
// push is never mistaken for a reply to some request that happened to share it.
const pushIndex uint32 = 0xFFFFFFFF

// pushWrapperOpcode is the message opcode every server push arrives under. The
// push's own id is protobuf field 1 of the body.
const pushWrapperOpcode = 0x0320

// pushID extracts a push's real id from a wrapper body.
//
// The body begins with four 0xFF bytes before the protobuf, which is what makes
// a wrapper recognisable without a schema.
func pushID(body []byte) (uint32, bool) {
	b := body
	if len(b) >= 4 && b[0] == 0xff && b[1] == 0xff && b[2] == 0xff && b[3] == 0xff {
		b = b[4:]
	}
	if len(b) < 2 || b[0] != 0x08 {
		return 0, false
	}
	v, n := uvarint(b[1:])
	if n == 0 || v == 0 || v > 0xFFFF {
		return 0, false
	}
	return uint32(v), true
}

// Message-layer header sizes, matching internal/frpg/rudp/message.go.
const (
	msgHeaderSize     = 12 // size(4) opcode(4) index(4)
	msgRespHeaderSize = 16 // replies carry this much again before the body
)

// protoStart locates where the protobuf begins inside a reassembled message.
//
// The header is SELF-DESCRIBING and does not need to be guessed: [0:4] is its
// own size (12), [4:8] the opcode, [8:12] the message index. A reply carries
// msgRespHeaderSize more bytes and is identifiable by a zero opcode.
//
// This used to probe candidate offsets starting at 8 and take the first that
// parsed cleanly. Offset 8 is the INDEX field, and four bytes of index parse as
// valid protobuf often enough that the probe accepted them: 6,515 of 15,573
// files in the corpus were written with the index prepended to their payload and
// a decoded tree built from it — 42% of the corpus, silently wrong. Verified
// after the fact: for every one of those files the first four payload bytes read
// as a little-endian uint32 equal the file's own index header, 6,515/6,515.
//
// So: derive the offset, and only fall back to probing if the derived one does
// not parse — and never probe 8.
func protoStart(full []byte) (int, bool) {
	if len(full) >= msgHeaderSize {
		if hdr := int(binary.BigEndian.Uint32(full[0:4])); hdr == msgHeaderSize {
			off := hdr
			if binary.BigEndian.Uint32(full[4:8]) == 0 {
				off += msgRespHeaderSize // a reply
			}
			// An empty body is legitimate — most replies carry none — and must
			// not be rejected by a parser that requires at least one field.
			if off == len(full) || (off < len(full) && parsesCleanly(full[off:])) {
				return off, true
			}
		}
	}
	for _, off := range []int{12, 28, 16, 20, 24, 32, 36, 40} {
		if off >= len(full) {
			break
		}
		if parsesCleanly(full[off:]) {
			return off, true
		}
	}
	return 0, false
}

// parsesCleanly reports whether b is a complete, well-formed protobuf message.
func parsesCleanly(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	i, fields, depth := 0, 0, 0
	for i < len(b) {
		tag, n := uvarint(b[i:])
		if n == 0 {
			return false
		}
		i += n
		field, wire := tag>>3, tag&7
		if field == 0 {
			return false
		}
		switch wire {
		case 0:
			_, n := uvarint(b[i:])
			if n == 0 {
				return false
			}
			i += n
		case 1:
			i += 8
		case 5:
			i += 4
		case 2:
			ln, n := uvarint(b[i:])
			if n == 0 {
				return false
			}
			i += n + int(ln)
		// Wire types 3 and 4 are GROUPS — the deprecated protobuf construct.
		// DS2 still uses them: RequestGetRightMatchingArea's response and
		// RequestNotifyKillEnemy both do, and rejecting them left 136 messages
		// unparseable and filed at a fallback offset.
		case 3:
			depth++
		case 4:
			depth--
			if depth < 0 {
				return false
			}
		default:
			return false
		}
		if i > len(b) {
			return false
		}
		fields++
	}
	return fields > 0 && depth == 0
}

// parseTagged reads one line of udpdump --tagged output.
//
// The line is "<dir> <ts> <hex>", and the TIMESTAMP IS OPTIONAL: the older form
// "<dir> <hex>" must keep working, because dumps made before timestamps were
// wired through are still valid input and re-capturing is not always possible —
// the session keys are derived per login, so a capture taken without keydump
// running can never be read again.
//
// Disambiguation is by parse, not by field count: hex and a decimal timestamp
// are both "a run of digits", so the second field is a timestamp only if it
// parses as a float AND leaves something behind to be the hex.
func parseTagged(line string) (dir string, ts float64, raw []byte, ok bool) {
	dir, rest, ok := strings.Cut(strings.TrimSpace(line), " ")
	if !ok {
		return "", 0, nil, false
	}
	hx := rest
	if maybeTS, tail, split := strings.Cut(rest, " "); split {
		if v, err := strconv.ParseFloat(maybeTS, 64); err == nil {
			ts, hx = v, tail
		}
	}
	raw, err := hex.DecodeString(hx)
	if err != nil {
		return "", 0, nil, false
	}
	return dir, ts, raw, true
}

// assembler accumulates fragments for one direction, keyed by message id.
type assembler struct {
	parts map[uint16]*partial
}

type partial struct {
	buf        []byte
	total      int
	got        int
	compressed bool
	inflated   uint32
	tsFirst    float64
	tsLast     float64
}

func newAssembler() *assembler { return &assembler{parts: map[uint16]*partial{}} }

func (a *assembler) push(body []byte, ts float64) ([]byte, bool, float64, float64) {
	if len(body) < 12 {
		return nil, false, 0, 0
	}
	id := binary.BigEndian.Uint16(body[0:2])
	compressed := body[2] != 0
	total := int(binary.BigEndian.Uint16(body[6:8]))
	idx := body[9]
	fragLen := int(binary.BigEndian.Uint16(body[10:12]))

	off := 12
	p := a.parts[id]
	if idx == 0 || p == nil {
		p = &partial{total: total, compressed: compressed, tsFirst: ts}
		a.parts[id] = p
	}
	p.tsLast = ts
	if compressed && idx == 0 {
		if len(body) < off+4 {
			return nil, false, 0, 0
		}
		p.inflated = binary.BigEndian.Uint32(body[off : off+4])
		off += 4
	}
	if len(body) < off+fragLen {
		fragLen = len(body) - off
	}
	if fragLen <= 0 {
		return nil, false, 0, 0
	}
	p.buf = append(p.buf, body[off:off+fragLen]...)
	p.got += fragLen
	if p.got < p.total {
		return nil, false, 0, 0
	}
	delete(a.parts, id)

	out := p.buf
	if p.compressed {
		zr, err := zlib.NewReader(bytes.NewReader(out))
		if err != nil {
			return nil, false, 0, 0
		}
		inflated, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			return nil, false, 0, 0
		}
		out = inflated
	}
	return out, true, p.tsFirst, p.tsLast
}

func main() {
	outDir := flag.String("out", "corpus", "directory to write per-opcode files into")
	session := flag.String("session", "session", "label for this capture, used in filenames")
	keyHex := flag.String("key", "", "16-byte game CWC key, hex")
	flag.Parse()

	key, err := hex.DecodeString(strings.TrimSpace(*keyHex))
	if err != nil || len(key) != 16 {
		fmt.Fprintln(os.Stderr, "corpus: -key must be 32 hex characters")
		os.Exit(2)
	}
	server, err := frpgcipher.NewServerUDPCipher(key)
	if err != nil {
		fatal("%v", err)
	}
	client, err := frpgcipher.NewClientUDPCipher(key, [8]byte{})
	if err != nil {
		fatal("%v", err)
	}

	asm := map[string]*assembler{"c2s": newAssembler(), "s2c": newAssembler()}
	var msgs []message
	var decrypted, failed, notRUDP int

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		dir, ts, raw, ok := parseTagged(sc.Text())
		if !ok {
			continue
		}
		var pt []byte
		if dir == "c2s" {
			pt, _, err = server.Open(raw)
		} else {
			pt, _, err = client.Open(raw)
		}
		if err != nil {
			failed++
			continue
		}
		decrypted++
		if len(pt) < 7 || pt[0] != 0xF5 || pt[1] != 0x02 {
			notRUDP++
			continue
		}
		full, done, tsFirst, tsLast := asm[dir].push(pt[7:], ts)
		if !done || len(full) < 8 {
			continue
		}
		off, ok := protoStart(full)
		if !ok {
			off = 8
		}
		m := message{
			dir: dir, opcode: binary.BigEndian.Uint32(full[4:8]),
			payload: full[off:], protoOff: off, clean: ok,
			tsFirst: tsFirst, tsLast: tsLast,
		}
		if len(full) >= 12 {
			m.index, m.hasIndex = binary.LittleEndian.Uint32(full[8:12]), true
		}
		// Pushes all ride the wrapper opcode 0x0320 and carry their REAL id in
		// protobuf field 1. Filing them under the wrapper buries every push type
		// in one bucket — which is exactly how four bell tolls ended up looking
		// like they were missing from the corpus.
		// ONLY messages arriving under the wrapper opcode are pushes. Testing the
		// body shape alone matches any message whose first field is a small
		// varint, which invented half a dozen push types that do not exist.
		if id, isPush := pushID(m.payload); isPush && m.opcode == pushWrapperOpcode {
			m.opcode = id
			m.push = true
		}
		msgs = append(msgs, m)
	}

	// Pair each response to its request by Index. Half this corpus is responses
	// filed under opcode 0, because a response header carries no opcode — the
	// Index is the only thing that says what they answer.
	//
	// Keyed on the LAST request seen with a given Index rather than the first:
	// the counter wraps and gets reused over a long session, and the nearest
	// preceding request is the one being answered.
	reqOpcode := map[uint32]uint32{}
	matched := 0
	for i := range msgs {
		m := &msgs[i]
		if !m.hasIndex {
			continue
		}
		if m.dir == "c2s" && m.opcode != 0 {
			reqOpcode[m.index] = m.opcode
			continue
		}
		// Pushes carry Index 0xFFFFFFFF and answer nothing.
		if m.dir == "s2c" && m.opcode == 0 && m.index != pushIndex {
			if op, ok := reqOpcode[m.index]; ok {
				m.repliesTo = op
				matched++
			}
		}
	}

	counts := map[uint32]int{}
	for _, m := range msgs {
		counts[m.opcode]++
	}

	for i, m := range msgs {
		name := opcodeNames[m.opcode]
		if name == "" {
			name = "unknown"
		}
		kind := ""
		if m.push {
			kind = "PUSH_"
		}
		dir := filepath.Join(*outDir, fmt.Sprintf("%s%#04x_%s", kind, m.opcode, name))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fatal("%v", err)
		}
		f := filepath.Join(dir, fmt.Sprintf("%s_%s_%04d.txt", *session, m.dir, i))
		var b strings.Builder
		note := ""
		if !m.clean {
			note = "  (NO CLEAN PARSE — offset is a fallback)"
		}
		fmt.Fprintf(&b, "session:   %s\ndirection: %s\nopcode:    %#04x  %s\nbytes:     %d\nproto-off: %d%s\n",
			*session, m.dir, m.opcode, name, len(m.payload), m.protoOff, note)
		// Absent when the input carried no timestamps, so an old dump produces
		// the old header rather than a line of zeroes pretending to be data.
		if m.hasIndex {
			fmt.Fprintf(&b, "index:     %d\n", m.index)
		}
		if m.repliesTo != 0 {
			rn := opcodeNames[m.repliesTo]
			if rn == "" {
				rn = "unknown"
			}
			fmt.Fprintf(&b, "replies-to: %#04x  %s\n", m.repliesTo, rn)
		}
		if m.tsLast > 0 {
			t := time.Unix(0, int64(m.tsLast*1e9)).UTC()
			fmt.Fprintf(&b, "time:      %s  (%.6f)\n", t.Format("2006-01-02T15:04:05.000000Z"), m.tsLast)
			// Only worth printing when the message actually spanned datagrams.
			if d := m.tsLast - m.tsFirst; d > 0 {
				fmt.Fprintf(&b, "assembled: %.6fs across fragments\n", d)
			}
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "hex:\n%s\n\n", hex.EncodeToString(m.payload))
		fmt.Fprintf(&b, "protobuf:\n%s\n", describeProto(m.payload, "  "))
		if err := os.WriteFile(f, []byte(b.String()), 0o644); err != nil {
			fatal("%v", err)
		}
	}

	ops := make([]uint32, 0, len(counts))
	for op := range counts {
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool { return counts[ops[i]] > counts[ops[j]] })

	fmt.Fprintf(os.Stderr, "%s: %d datagrams decrypted, %d failed, %d non-RUDP -> %d messages, %d responses paired\n",
		*session, decrypted, failed, notRUDP, len(msgs), matched)
	for _, op := range ops {
		name := opcodeNames[op]
		if name == "" {
			name = "unknown"
		}
		fmt.Printf("%6d  %#04x  %s\n", counts[op], op, name)
	}
}

// describeProto renders a protobuf message without a schema.
//
// Deliberately structural: it reports field numbers, wire types and values, and
// does not try to name anything. Naming is what we are trying to CHECK against
// our schema, so inventing it here would defeat the purpose.
func describeProto(b []byte, indent string) string {
	var sb strings.Builder
	i := 0
	for i < len(b) {
		tag, n := uvarint(b[i:])
		if n == 0 {
			fmt.Fprintf(&sb, "%s<undecodable tail %d bytes>\n", indent, len(b)-i)
			break
		}
		i += n
		field, wire := tag>>3, tag&7
		switch wire {
		case 0:
			v, n := uvarint(b[i:])
			if n == 0 {
				return sb.String()
			}
			i += n
			fmt.Fprintf(&sb, "%sfield %d  varint  %d\n", indent, field, v)
		case 2:
			ln, n := uvarint(b[i:])
			if n == 0 || i+n+int(ln) > len(b) {
				return sb.String()
			}
			i += n
			val := b[i : i+int(ln)]
			i += int(ln)
			if isPrintable(val) {
				fmt.Fprintf(&sb, "%sfield %d  string  %q\n", indent, field, string(val))
			} else if len(val) == 0 {
				fmt.Fprintf(&sb, "%sfield %d  bytes   (empty)\n", indent, field)
			} else {
				fmt.Fprintf(&sb, "%sfield %d  bytes   %d: %s\n", indent, field, len(val), shortHex(val))
				if sub := describeProto(val, indent+"    "); strings.Count(sub, "\n") > 0 && looksNested(val) {
					sb.WriteString(sub)
				}
			}
		case 5:
			if i+4 > len(b) {
				return sb.String()
			}
			fmt.Fprintf(&sb, "%sfield %d  fixed32 %d\n", indent, field, binary.LittleEndian.Uint32(b[i:]))
			i += 4
		case 1:
			if i+8 > len(b) {
				return sb.String()
			}
			fmt.Fprintf(&sb, "%sfield %d  fixed64 %d\n", indent, field, binary.LittleEndian.Uint64(b[i:]))
			i += 8
		// Wire 3/4 are GROUPS, the deprecated construct DS2 still uses — for
		// RequestNotifyKillEnemy and RequestGetRightMatchingArea responses among
		// others. Rendering them as nested keeps those readable instead of
		// stopping the whole tree at the first one.
		case 3:
			end, ok := groupEnd(b, i, field)
			if !ok {
				fmt.Fprintf(&sb, "%sfield %d  group   <unterminated>\n", indent, field)
				return sb.String()
			}
			fmt.Fprintf(&sb, "%sfield %d  group\n", indent, field)
			sb.WriteString(describeProto(b[i:end], indent+"    "))
			i = end
			if _, n := uvarint(b[i:]); n > 0 {
				i += n // step over the matching end-group tag
			}
		case 4:
			// A stray end-group; the caller consumed the opener.
			return sb.String()
		default:
			fmt.Fprintf(&sb, "%s<wire type %d, stopping>\n", indent, wire)
			return sb.String()
		}
	}
	return sb.String()
}

// groupEnd finds the offset of the END_GROUP tag matching a group opened at
// start, honouring nesting so an inner group cannot close the outer one.
func groupEnd(b []byte, start int, field uint64) (int, bool) {
	i, depth := start, 1
	for i < len(b) {
		tag, n := uvarint(b[i:])
		if n == 0 {
			return 0, false
		}
		f, wire := tag>>3, tag&7
		if wire == 4 {
			depth--
			if depth == 0 {
				if f != field {
					return 0, false // closes a different field: malformed
				}
				return i, true
			}
			i += n
			continue
		}
		i += n
		switch wire {
		case 0:
			_, n := uvarint(b[i:])
			if n == 0 {
				return 0, false
			}
			i += n
		case 1:
			i += 8
		case 5:
			i += 4
		case 2:
			ln, n := uvarint(b[i:])
			if n == 0 {
				return 0, false
			}
			i += n + int(ln)
		case 3:
			depth++
		default:
			return 0, false
		}
		if i > len(b) {
			return 0, false
		}
	}
	return 0, false
}

func looksNested(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	tag, n := uvarint(b)
	return n > 0 && tag>>3 > 0 && tag>>3 < 40 && tag&7 <= 5
}

func isPrintable(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

func shortHex(b []byte) string {
	if len(b) > 48 {
		return hex.EncodeToString(b[:48]) + "..."
	}
	return hex.EncodeToString(b)
}

func uvarint(b []byte) (uint64, int) {
	var v uint64
	var s uint
	for i, c := range b {
		if i > 9 {
			return 0, 0
		}
		v |= uint64(c&0x7f) << s
		if c&0x80 == 0 {
			return v, i + 1
		}
		s += 7
	}
	return 0, 0
}

// opcodeNames is generated from the first opcode column of docs/protocol-map.md.
//
// CONFIRMED against live PC traffic: a captured session showed 0x0397, 0x0392,
// 0x03AE, 0x03B2 and 0x03B8 exactly where that column says they should be, so
// DS2 SOTFS on PC shares the numbering we mapped from PS3. That is why the bell
// pair 0x03EE / 0x03EF is expected to be the bell here too.
var opcodeNames = map[uint32]string{
	0x0320: "RequestSendMessageToPlayers",
	0x0386: "RequestWaitForUserLogin",
	0x0389: "ManagementTextMessage",
	0x038b: "RegulationFileUpdatePushMessage",
	0x038c: "PlayerInfoUploadConfigPushMessage",
	0x038d: "ServerPing",
	0x0391: "RequestCreateBloodstain",
	0x0392: "RequestGetBloodstainList",
	0x0393: "RequestGetDeadingGhost",
	0x0394: "RequestCreateSign",
	0x0395: "RequestUpdateSign",
	0x0396: "RequestRemoveSign",
	0x0397: "RequestGetSignList",
	0x0398: "RequestSummonSign",
	0x039a: "RequestRejectSign",
	0x039b: "PushRequestSummonSign",
	0x039c: "PushRequestRejectSign",
	0x039d: "PushRequestRemoveSign",
	0x039e: "RequestCreateMirrorKnightSign",
	0x039f: "RequestUpdateMirrorKnightSign",
	0x03a0: "RequestRemoveMirrorKnightSign",
	0x03a1: "RequestGetMirrorKnightSignList",
	0x03a2: "RequestSummonMirrorKnightSign",
	0x03a3: "RequestBenchmarkThroughput",
	0x03a4: "RequestRejectMirrorKnightSign",
	0x03a5: "PushRequestSummonMirrorKnightSign",
	0x03a6: "PushRequestRejectMirrorKnightSign",
	0x03a7: "PushRequestRemoveMirrorKnightSign",
	0x03a8: "RequestUpdatePlayerCharacter",
	0x03a9: "RequestGetPlayerCharacter",
	0x03aa: "PushRequestEvaluateBloodMessage",
	0x03ab: "RequestCreateBloodMessage",
	0x03ac: "RequestRemoveBloodMessage",
	0x03ad: "RequestReentryBloodMessage",
	0x03ae: "RequestGetBloodMessageList",
	0x03af: "RequestEvaluateBloodMessage",
	0x03b0: "RequestGetBloodMessageEvaluation",
	0x03b1: "RequestCreateGhostData",
	0x03b2: "RequestGetGhostDataList",
	0x03b3: "RequestGetLoginPlayerCharacter",
	0x03b6: "RequestUpdateLoginPlayerCharacter",
	0x03b8: "RequestUpdatePlayerStatus",
	0x03c6: "RequestGetAnnounceMessageList",
	0x03c9: "PushRequestNotifyRingBell",
	0x03cd: "RequestNotifyKillEnemy",
	0x03cf: "PushRequestVisit",
	0x03d0: "PushRequestRejectVisit",
	0x03d1: "PushRequestRemoveVisitor",
	0x03d2: "RequestGetBreakInTargetList",
	0x03d3: "RequestBreakInTarget",
	0x03d4: "RequestRejectBreakInTarget",
	0x03d5: "RequestGetVisitorList",
	0x03d6: "RequestVisit",
	0x03d7: "RequestRejectVisit",
	0x03d8: "RequestNotifyMirrorKnight",
	0x03d9: "RequestRegisterQuickMatch",
	0x03da: "RequestUnregisterQuickMatch",
	0x03db: "RequestUpdateQuickMatch",
	0x03dc: "RequestSearchQuickMatch",
	0x03dd: "RequestJoinQuickMatch",
	0x03de: "RequestRejectQuickMatch",
	0x03e1: "PushRequestJoinQuickMatch",
	0x03e3: "PushRequestRejectQuickMatch",
	0x03e5: "PushRequestAllowQuickMatch",
	0x03e7: "PushRequestRemoveQuickMatch",
	0x03e8: "RequestNotifyJoinGuestPlayer",
	0x03e9: "RequestNotifyLeaveGuestPlayer",
	0x03ea: "RequestNotifyJoinSession",
	0x03eb: "RequestNotifyLeaveSession",
	0x03ec: "RequestGetAnnounceMessageList",
	0x03ed: "RequestNotifyKillPlayer",
	0x03ee: "RequestNotifyRingBell",
	0x03ef: "PushRequestNotifyRingBell",
	0x03f0: "RequestGetTotalDeathCount",
	0x03f1: "RequestNotifyDeath",
	0x03f2: "RequestNotifyOfflineDeathCount",
	0x03f3: "RequestRegisterPowerStoneData",
	0x03f4: "RequestGetPowerStoneRanking",
	0x03f5: "RequestGetPowerStoneMyRanking",
	0x03f6: "RequestNotifyKillEnemy",
	0x03f7: "RequestNotifyBuyItem",
	0x03f8: "RequestGetPowerStoneRankingRecordCount",
	0x03f9: "RequestNotifyDisconnectSession",
	0x03fa: "RequestGetRightMatchingArea",
	0x03fb: "PushRequestBreakInTarget",
	0x03fc: "PushRequestRejectBreakInTarget",
	0x03fd: "PushRequestAllowBreakInTarget",
	0x03ff: "RequestGetAreaBloodMessageList",
	0x0400: "RequestGetAreaBloodstainList",
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "corpus: "+format+"\n", a...)
	os.Exit(1)
}
