package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnv(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dso.env")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestRealEnvironmentWins is the ordering that makes one-off overrides possible:
// the file supplies defaults, an explicit variable beats it.
func TestRealEnvironmentWins(t *testing.T) {
	t.Setenv("DSO_TEST_PRESET", "from-environment")
	p := writeEnv(t, "DSO_TEST_PRESET=from-file\nDSO_TEST_UNSET=from-file\n")

	if err := LoadEnvFile(p); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DSO_TEST_PRESET"); got != "from-environment" {
		t.Errorf("preset variable = %q, want it left alone", got)
	}
	if got := os.Getenv("DSO_TEST_UNSET"); got != "from-file" {
		t.Errorf("unset variable = %q, want %q", got, "from-file")
	}
	os.Unsetenv("DSO_TEST_UNSET")
}

// TestMissingFileIsNotAnError — running with no env file is normal.
func TestMissingFileIsNotAnError(t *testing.T) {
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "absent.env")); err != nil {
		t.Fatalf("missing file should be silently accepted, got %v", err)
	}
}

func TestEnvFileSyntax(t *testing.T) {
	p := writeEnv(t, `
# a comment
   # indented comment

DSO_TEST_PLAIN=value
DSO_TEST_SPACED  =  spaced value
DSO_TEST_QUOTED="quoted value "
DSO_TEST_SINGLE='single'
DSO_TEST_EMPTY=
DSO_TEST_URL=http://example.com/x?a=1&b=2
DSO_TEST_INNERQ=say "hi"
`)
	if err := LoadEnvFile(p); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, want string }{
		{"DSO_TEST_PLAIN", "value"},
		{"DSO_TEST_SPACED", "spaced value"},
		// Quotes are stripped, so a deliberate trailing space survives.
		{"DSO_TEST_QUOTED", "quoted value "},
		{"DSO_TEST_SINGLE", "single"},
		{"DSO_TEST_EMPTY", ""},
		// A value containing '=' must not be split again — the RPCS3 hosts string
		// and any URL with a query would otherwise be mangled.
		{"DSO_TEST_URL", "http://example.com/x?a=1&b=2"},
		// Only a surrounding pair is stripped.
		{"DSO_TEST_INNERQ", `say "hi"`},
	} {
		if got := os.Getenv(tc.key); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
		os.Unsetenv(tc.key)
	}
}

func TestEnvFileRejectsMalformedLine(t *testing.T) {
	p := writeEnv(t, "DSO_TEST_OK=1\nthis line has no equals sign\n")
	err := LoadEnvFile(p)
	if err == nil {
		t.Fatal("expected an error naming the bad line; a silently ignored typo " +
			"is how a config change appears not to take effect")
	}
	os.Unsetenv("DSO_TEST_OK")
}

// TestExampleFileParses keeps the committed example honest — it is the thing
// users copy, so a syntax error in it is a broken first run.
func TestExampleFileParses(t *testing.T) {
	if err := LoadEnvFile("../../dso.env.example"); err != nil {
		t.Fatalf("dso.env.example does not parse: %v", err)
	}
}
