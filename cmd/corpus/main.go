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
	"strings"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
)

type message struct {
	dir      string
	opcode   uint32
	payload  []byte
	protoOff int
	clean    bool
	push     bool
}

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

// protoStart locates where the protobuf begins inside a reassembled message.
//
// The message header is NOT a fixed size: requests and responses carry different
// amounts of preamble after the opcode, and assuming one offset produced garbage
// for half the corpus. Rather than invent a layout, this tries the plausible
// offsets and takes the first that parses cleanly all the way to the end — a
// whole-buffer parse is a strong check, since a wrong offset almost always hits
// an invalid wire type or runs off the end.
//
// The chosen offset is recorded per message, so the real layout can be worked
// out FROM the corpus rather than assumed before reading it.
func protoStart(full []byte) (int, bool) {
	for _, off := range []int{8, 12, 16, 20, 24, 28, 32, 36, 40} {
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
	i, fields := 0, 0
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
		default:
			return false
		}
		if i > len(b) {
			return false
		}
		fields++
	}
	return fields > 0
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
}

func newAssembler() *assembler { return &assembler{parts: map[uint16]*partial{}} }

func (a *assembler) push(body []byte) ([]byte, bool) {
	if len(body) < 12 {
		return nil, false
	}
	id := binary.BigEndian.Uint16(body[0:2])
	compressed := body[2] != 0
	total := int(binary.BigEndian.Uint16(body[6:8]))
	idx := body[9]
	fragLen := int(binary.BigEndian.Uint16(body[10:12]))

	off := 12
	p := a.parts[id]
	if idx == 0 || p == nil {
		p = &partial{total: total, compressed: compressed}
		a.parts[id] = p
	}
	if compressed && idx == 0 {
		if len(body) < off+4 {
			return nil, false
		}
		p.inflated = binary.BigEndian.Uint32(body[off : off+4])
		off += 4
	}
	if len(body) < off+fragLen {
		fragLen = len(body) - off
	}
	if fragLen <= 0 {
		return nil, false
	}
	p.buf = append(p.buf, body[off:off+fragLen]...)
	p.got += fragLen
	if p.got < p.total {
		return nil, false
	}
	delete(a.parts, id)

	out := p.buf
	if p.compressed {
		zr, err := zlib.NewReader(bytes.NewReader(out))
		if err != nil {
			return nil, false
		}
		inflated, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			return nil, false
		}
		out = inflated
	}
	return out, true
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
		dir, hx, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		raw, err := hex.DecodeString(hx)
		if err != nil {
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
		full, done := asm[dir].push(pt[7:])
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
		fmt.Fprintf(&b, "session:   %s\ndirection: %s\nopcode:    %#04x  %s\nbytes:     %d\nproto-off: %d%s\n\n",
			*session, m.dir, m.opcode, name, len(m.payload), m.protoOff, note)
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

	fmt.Fprintf(os.Stderr, "%s: %d datagrams decrypted, %d failed, %d non-RUDP -> %d messages\n",
		*session, decrypted, failed, notRUDP, len(msgs))
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
		default:
			fmt.Fprintf(&sb, "%s<wire type %d, stopping>\n", indent, wire)
			return sb.String()
		}
	}
	return sb.String()
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
