// verifykey checks whether a candidate AES-CWC key really decrypts captured
// Frpg2 game traffic, and prints the plaintext if it does.
//
// This exists because a key read out of a live process is a claim, not a fact.
// keydump reports every key the client installs — at least the auth-stream key
// and the game-service key — and nothing at the point of capture says which is
// which. The CWC tag settles it: a wrong key fails authentication, so a datagram
// that verifies is proof.
//
// The framing is the project's own, from internal/crypto/frpgcipher/cwc_udp.go:
//
//	server->client   IV(11) ‖ tag(16) ‖ ciphertext                 AAD = IV
//	client->server   token(8) ‖ IV(11) ‖ tag(16) ‖ ptype(1) ‖ ct   AAD = IV ‖ token ‖ ptype
//
// Usage:
//
//	verifykey <keyhex> <datagramhex> [s2c|c2s]
//	verifykey -keys keys.txt -datagram <hex> [s2c|c2s]
//
// The second form tries every key in a keydump output file and reports which one
// works, which is the normal case: dump the keys, grab one datagram out of the
// capture, and let this pick the right key.
package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
)

func main() {
	keysFile := flag.String("keys", "", "file of candidate keys (keydump output, or one hex key per line)")
	datagram := flag.String("datagram", "", "captured datagram, hex (UDP payload only, no IP/UDP headers)")
	flag.Parse()

	args := flag.Args()
	var keys []string

	switch {
	case *keysFile != "":
		k, err := loadKeys(*keysFile)
		if err != nil {
			fatal("%v", err)
		}
		keys = k
	case len(args) >= 1:
		keys = []string{args[0]}
		args = args[1:]
	default:
		usage()
	}

	dg := *datagram
	if dg == "" {
		if len(args) == 0 {
			usage()
		}
		dg = args[0]
		args = args[1:]
	}

	dir := "s2c"
	if len(args) > 0 {
		dir = strings.ToLower(args[0])
	}
	if dir != "s2c" && dir != "c2s" {
		fatal("direction must be s2c or c2s, got %q", dir)
	}

	raw, err := hex.DecodeString(strings.ReplaceAll(dg, " ", ""))
	if err != nil {
		fatal("datagram is not hex: %v", err)
	}

	fmt.Printf("datagram %d bytes, direction %s, %d candidate key(s)\n\n", len(raw), dir, len(keys))

	for i, ks := range keys {
		key, err := hex.DecodeString(strings.ReplaceAll(ks, " ", ""))
		if err != nil || len(key) != 16 {
			fmt.Printf("  [%d] %-32s SKIP (not 16 hex bytes)\n", i+1, ks)
			continue
		}

		// NOTE: Open returns (plaintext, connectionPrefix, error). The bool is
		// NOT a success flag — it marks the SYN datagram whose plaintext carries
		// the 35-byte connection prefix. Authentication failure comes back as a
		// non-nil error. Reading that bool as "ok" makes a correct key look
		// wrong, which is a miserable thing to debug during a live session.
		pt, connPrefix, err := open(key, raw, dir)
		switch {
		case err != nil:
			fmt.Printf("  [%d] %s  %v\n", i+1, ks, err)
		default:
			fmt.Printf("  [%d] %s  ** VERIFIED **\n\n", i+1, ks)
			if connPrefix {
				fmt.Println("this is the SYN datagram (carries the 35-byte connection prefix)")
			}
			fmt.Printf("plaintext %d bytes:\n%s\n", len(pt), hex.Dump(pt))
			// The reliable-UDP layer starts with magic F5 02; seeing it is a
			// second, independent confirmation that the key is right.
			if len(pt) >= 2 && pt[0] == 0xF5 && pt[1] == 0x02 {
				fmt.Println("RUDP magic F5 02 present — this is real Frpg2 traffic.")
			} else {
				fmt.Println("NOTE: no F5 02 magic at the start. The tag verified, so the key is")
				fmt.Println("right, but this datagram may be a handshake rather than a game message.")
			}
			return
		}
	}

	fmt.Println("\nno candidate key verified this datagram.")
	fmt.Println("Check that the hex is the UDP PAYLOAD only (no Ethernet/IP/UDP headers),")
	fmt.Println("and that the direction matches which way the datagram travelled.")
	os.Exit(1)
}

// open picks the cipher whose Open() handles the direction asked for.
//
// Returns (plaintext, connectionPrefix, error) — matching the UDPCipher
// interface. Failure is the error, not the bool.
//
// Note the naming is by ROLE, not by direction: ServerUDPCipher is what a server
// uses, so its Open() reads client->server datagrams, and ClientUDPCipher's
// Open() reads server->client ones. Getting this backwards produces a tag
// failure that looks exactly like a wrong key.
func open(key, raw []byte, dir string) ([]byte, bool, error) {
	if dir == "s2c" {
		// Server->client datagrams carry no auth token and their AAD is the IV
		// alone, so the token here is unused; zero is fine.
		c, err := frpgcipher.NewClientUDPCipher(key, [8]byte{})
		if err != nil {
			return nil, false, err
		}
		return c.Open(raw)
	}

	// Client->server: the token is inside the datagram and forms part of the
	// AAD, which the server-side Open() pulls out for itself.
	if _, ok := frpgcipher.TokenFromDatagram(raw); !ok {
		return nil, false, fmt.Errorf("datagram too short to carry an 8-byte auth token")
	}
	c, err := frpgcipher.NewServerUDPCipher(key)
	if err != nil {
		return nil, false, err
	}
	return c.Open(raw)
}

// loadKeys accepts either bare hex keys or keydump's own output lines, so the
// file it writes can be passed straight back in.
func loadKeys(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		for _, field := range strings.Fields(sc.Text()) {
			if len(field) != 32 {
				continue
			}
			if _, err := hex.DecodeString(field); err != nil {
				continue
			}
			if !seen[field] {
				seen[field] = true
				out = append(out, field)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no 16-byte hex keys found in %s", path)
	}
	return out, nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `verifykey — check a candidate CWC key against a captured datagram

  verifykey <keyhex> <datagramhex> [s2c|c2s]
  verifykey -keys keys.txt -datagram <hex> [s2c|c2s]

The datagram must be the UDP payload only, with no Ethernet/IP/UDP headers.`)
	os.Exit(2)
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "verifykey: "+format+"\n", a...)
	os.Exit(1)
}
