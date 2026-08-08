package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// DefaultEnvFile is loaded at startup when present.
const DefaultEnvFile = "dso.env"

// LoadEnvFile reads a KEY=VALUE file and populates the process environment.
//
// Variables already set in the real environment are left alone, so the file
// supplies defaults and an explicit `DSO_X=... ./dsoserver` still wins. That
// ordering is what makes one-off overrides possible without editing the file.
//
// A missing file is not an error: running without one is normal.
//
// Syntax is deliberately small — no interpolation, no export keyword, no multi-
// line values. Anything fancier belongs in a real config format rather than
// growing here:
//
//	# comment
//	DSO_SERVER_ADVERTISE_ADDRESS=192.168.1.100
//	DSO_MANAGEMENT_TEXT="quotes are stripped, so trailing spaces survive"
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("config: open env file %q: %w", path, err)
	}
	defer f.Close()

	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return fmt.Errorf("config: %s:%d: expected KEY=VALUE, got %q", path, line, text)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("config: %s:%d: empty key", path, line)
		}
		// Only the surrounding pair is stripped, so a value that is genuinely
		// meant to contain quotes keeps them.
		value = strings.TrimSpace(value)
		quoted := false
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value, quoted = value[1:len(value)-1], true
			}
		}
		// Strip a trailing ` # comment` from UNQUOTED values.
		//
		// Without this, `DSO_BELL_TEST_MAP=10190000  # Belfry Sol` sets the value
		// to "10190000  # Belfry Sol", every numeric parse of it fails, and the
		// code silently falls back to its default — which is exactly the kind of
		// quiet wrong-value this project keeps paying for. It bit us once
		// already, and only harmlessly because the default happened to match.
		//
		// Quoted values are taken literally, so free text that really wants a #
		// (an obelisk message, say) just needs quoting.
		if !quoted {
			if i := strings.IndexAny(value, " \t"); i >= 0 {
				if j := strings.Index(value[i:], "#"); j >= 0 &&
					strings.TrimSpace(value[i:i+j]) == "" {
					value = strings.TrimSpace(value[:i])
				}
			}
		}
		// FIRST occurrence wins, so a duplicate further down the file is silently
		// ignored. That is survivable but confusing enough to say out loud —
		// editing the second copy and seeing nothing change is a bad afternoon.
		if seen[key] {
			fmt.Fprintf(os.Stderr, "config: %s:%d: %s appears more than once; "+
				"the FIRST value is used and this line is ignored\n", path, line, key)
		}
		seen[key] = true
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("config: %s:%d: set %s: %w", path, line, key, err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("config: read env file %q: %w", path, err)
	}
	return nil
}
