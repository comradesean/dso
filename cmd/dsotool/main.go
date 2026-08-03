// Command dsotool provides development and operations helpers for the dso
// server: key generation and RPCS3 patch emission.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sstreight/dso/internal/crypto/keys"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = cmdKeygen(os.Args[2:])
	case "ps3-patch":
		err = cmdPS3Patch(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `dsotool <command>

commands:
  keygen     --out DIR [--bits 2048] [--exponent 3]
             Generate an RSA key pair and write server.private.pem / server.public.pem.

  ps3-patch  --key FILE --ppu-hash PPU-xxxx --vaddr 0x189AB48 [--orig-len N] [--title BLUS41045]
             Emit an RPCS3 patch.yml that overwrites the client's login RSA public
             key (at the given virtual address) with our public key.
`)
}

func parseFlags(args []string) map[string]string {
	m := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			key := strings.TrimPrefix(a, "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				m[key] = args[i+1]
				i++
			} else {
				m[key] = "true"
			}
		}
	}
	return m
}

func cmdKeygen(args []string) error {
	f := parseFlags(args)
	out := f["out"]
	if out == "" {
		return fmt.Errorf("keygen: --out DIR is required")
	}
	bits := atoiDefault(f["bits"], 2048)
	exp := atoiDefault(f["exponent"], 65537)

	priv, err := keys.GenerateWithExponent(bits, exp)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out+"/server.private.pem", keys.PrivatePEM(priv), 0o600); err != nil {
		return err
	}
	pubPEM := keys.PublicPEM(&priv.PublicKey)
	if err := os.WriteFile(out+"/server.public.pem", pubPEM, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote key pair to %s (bits=%d exponent=%d)\n", out, bits, exp)
	fmt.Printf("public key PEM is %d bytes\n", len(pubPEM))
	return nil
}

func cmdPS3Patch(args []string) error {
	f := parseFlags(args)
	keyFile := f["key"]
	if keyFile == "" {
		return fmt.Errorf("ps3-patch: --key FILE is required")
	}
	vaddrStr := f["vaddr"]
	if vaddrStr == "" {
		return fmt.Errorf("ps3-patch: --vaddr is required")
	}
	vaddr, err := strconv.ParseUint(strings.TrimPrefix(vaddrStr, "0x"), 16, 64)
	if err != nil {
		return fmt.Errorf("ps3-patch: bad --vaddr: %w", err)
	}
	ppu := f["ppu-hash"]
	if ppu == "" {
		ppu = "PPU-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	}
	title := f["title"]
	if title == "" {
		title = "BLUS41045"
	}

	pubPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	// Validate it parses as a PKCS#1 public key.
	if _, err := keys.ParsePublicPEM(pubPEM); err != nil {
		return err
	}

	origLen := atoiDefault(f["orig-len"], len(pubPEM))
	if len(pubPEM) > origLen {
		return fmt.Errorf("ps3-patch: our key PEM (%d bytes) is larger than the original (%d); regenerate with --exponent 3", len(pubPEM), origLen)
	}

	hexStr := hex.EncodeToString(pubPEM)

	fmt.Printf(`Version: 1.2

%s:
  "dso login key redirect":
    Games:
      "DARK SOULS II":
        %s: [ All ]
    Author: "dso"
    Patch Version: "1.0"
    Notes: |
      Replaces the login server RSA public key at 0x%X with the dso server's
      public key so the client's login/auth handshake targets this server.
      Our PEM is %d bytes; original field is %d bytes.
    Patch:
      - [ bytes, 0x%X, "%s" ]
`, ppu, title, vaddr, len(pubPEM), origLen, vaddr, hexStr)
	return nil
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	// Support 0x-prefixed values too.
	if strings.HasPrefix(s, "0x") {
		if v, err := strconv.ParseInt(s[2:], 16, 64); err == nil {
			return int(v)
		}
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}
