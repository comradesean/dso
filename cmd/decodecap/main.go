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

// opcodeNames covers the opcodes this effort actually cares about plus enough
// context to read a session. Numbers from docs/protocol-map.md.
var opcodeNames = map[uint32]string{
	0x0386: "RequestWaitForUserLogin",
	0x0389: "ManagementTextMessage (push)",
	0x038B: "RegulationFileUpdatePushMessage (push)",
	0x038C: "PlayerInfoUploadConfigPushMessage (push)",
	0x038D: "ServerPing",
	0x03B8: "RequestUpdatePlayerStatus",
	0x03EC: "RequestGetAnnounceMessageList",
	0x03EE: "RequestNotifyRingBell",
	0x03EF: "PushRequestNotifyRingBell",
	0x03F0: "RequestGetTotalDeathCount",
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "decodecap: "+format+"\n", a...)
	os.Exit(1)
}
