// decodecap decrypts a captured Frpg2 game flow and prints the reliable-UDP
// layer underneath.
//
// Input is whatever tools/pcap/udpdump.py emits with --tagged: one datagram per
// line as "<c2s|s2c> <hex>". The key comes from cmd/keydump; cmd/verifykey tells
// you which of the dumped keys is the game one.
//
//	python3 tools/pcap/udpdump.py cap.pcapng --port 50000 --tagged \
//	  | go run ./cmd/decodecap -key <hex>
//
// The RUDP header is 7 bytes and the message framing sits inside it; see
// internal/frpg/rudp. This prints enough to find message types and payloads
// without reassembling fragments — the point is to see WHAT the server sent,
// which is the question the whole PC-capture effort exists to answer.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
)

func main() {
	keyHex := flag.String("key", "", "16-byte game CWC key, hex")
	showAll := flag.Bool("all", false, "print every datagram, not just ones carrying a message")
	grep := flag.String("grep", "", "only show plaintext containing this hex substring")
	flag.Parse()

	key, err := hex.DecodeString(strings.TrimSpace(*keyHex))
	if err != nil || len(key) != 16 {
		fmt.Fprintln(os.Stderr, "decodecap: -key must be 32 hex characters")
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

	var ok, failed int
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		dir, payload, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		raw, err := hex.DecodeString(strings.TrimSpace(payload))
		if err != nil {
			continue
		}

		var pt []byte
		var synPrefix bool
		// Named by role: the SERVER cipher opens client->server datagrams.
		if dir == "c2s" {
			pt, synPrefix, err = server.Open(raw)
		} else {
			pt, synPrefix, err = client.Open(raw)
		}
		if err != nil {
			failed++
			continue
		}
		ok++

		if *grep != "" && !strings.Contains(hex.EncodeToString(pt), strings.ToLower(*grep)) {
			continue
		}

		describe(dir, pt, synPrefix, *showAll)
	}

	fmt.Fprintf(os.Stderr, "\ndecoded %d datagram(s), %d failed\n", ok, failed)
	if failed > 0 && ok == 0 {
		fmt.Fprintln(os.Stderr, "nothing decoded — wrong key, or the direction tags are inverted")
	}
}

// describe prints the RUDP header fields and the payload.
//
// The 7-byte header is magic(2) ‖ flags/type ‖ packed 12-bit sequence counters;
// full framing is in internal/frpg/rudp. This deliberately does not reassemble
// fragments: the goal is to see which messages the server sent and when, and a
// raw view is more honest about what is actually in the capture.
func describe(dir string, pt []byte, synPrefix, showAll bool) {
	if len(pt) < 7 {
		if showAll {
			fmt.Printf("%s  short (%d bytes): %s\n", dir, len(pt), hex.EncodeToString(pt))
		}
		return
	}
	if pt[0] != 0xF5 || pt[1] != 0x02 {
		fmt.Printf("%s  NOT RUDP (%d bytes): %s\n", dir, len(pt), hex.EncodeToString(pt))
		return
	}

	ptype := pt[2]
	body := pt[7:]

	tag := ""
	if synPrefix {
		tag = "  [SYN/connection-prefix]"
	}
	fmt.Printf("%s  type=%#02x  %d bytes%s\n", dir, ptype, len(body), tag)

	// Message payloads carry a big-endian opcode a little way in. Print the
	// leading bytes rather than claiming to parse a structure we have not
	// verified for the PC build.
	n := len(body)
	if n > 64 && !showAll {
		n = 64
	}
	if n > 0 {
		fmt.Printf("      %s\n", hex.EncodeToString(body[:n]))
	}
	// The message opcode is a big-endian u32 at offset 16 of the RUDP body.
	// OBSERVED, not derived: a captured RequestWaitForUserLogin carried
	// 00000386 there and a RequestGetAnnounceMessageList carried 000003ec, both
	// matching docs/protocol-map.md. Server replies put something else in that
	// slot, so it is reported as a raw field unless it matches a known opcode.
	if len(body) >= 20 {
		op := binary.BigEndian.Uint32(body[16:])
		if name, known := opcodeNames[op]; known {
			fmt.Printf("      opcode %#04x  %s\n", op, name)
		} else {
			fmt.Printf("      field@16 %#08x\n", op)
		}
		switch op {
		case 0x03EE:
			fmt.Println("      *** BELL RING (client->server) ***")
		case 0x03EF:
			fmt.Println("      *** BELL TOLL PUSH (server->client) ***")
		case 0x038B:
			fmt.Println("      *** REGULATION FILE UPDATE PUSH ***")
		}
	}
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
	fmt.Fprintf(os.Stderr, "decodecap: "+format+"\n", a...)
	os.Exit(1)
}
