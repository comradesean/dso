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
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
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
