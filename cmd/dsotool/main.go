// Command dsotool provides development and operations helpers for the dso
// server: key generation and RPCS3 patch emission.
package main

import (
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

  ps3-patch  --key FILE --ppu-hash PPU-xxxx [--vaddr 0x17FB338] [--orig-len N] [--title BLUS41045]
             Emit an RPCS3 patch.yml that overwrites the client's login RSA public
             key with our public key. --vaddr defaults to the BLUS41045 login key
             at 0x17FB338; note the EBOOT also holds a second, unrelated key at
             0x189AB48 which must NOT be patched in its place.
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
		// Default to the login key. BLUS41045 embeds two distinct 2048-bit e=3
		// PEMs and only this one is used for the login handshake; the other, at
		// 0x189AB48, is not, and patching it instead yields a client that
		// connects but whose RSA block never decrypts.
		vaddrStr = defaultLoginKeyVaddr
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

	// RPCS3 has no "bytes" patch type: "byte" writes a single byte, so a hex
	// blob never applies. A PEM is ASCII, so "utf8" is the right type — it
	// memcpys the string as-is. ("cutf8" would append a NUL and overrun the
	// fixed-width field.) Newlines are escaped for YAML's double-quoted style.
	yamlStr := yamlQuote(pubPEM)

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
      - [ utf8, 0x%X, %s ]
`, ppu, title, vaddr, len(pubPEM), origLen, vaddr, yamlStr)
	return nil
}

// defaultLoginKeyVaddr is the virtual address of the BLUS41045 login RSA key.
const defaultLoginKeyVaddr = "0x17FB338"

// yamlQuote renders b as a YAML double-quoted scalar, escaping the characters
// that style defines. The PEM is ASCII, so only newlines, quotes and backslashes
// can occur in practice.
func yamlQuote(b []byte) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, c := range b {
		switch c {
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte('"')
	return sb.String()
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
