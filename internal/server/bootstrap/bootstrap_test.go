package bootstrap

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/server/core"
)

func testService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	contents := filepath.Join(dir, "contents_0101.bin")
	if err := os.WriteFile(contents, []byte("MANIFEST"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "regulation_0101.bin"), []byte("REGULATION-PAYLOAD"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.BootstrapContentsFile = contents
	return &Service{srv: &core.Server{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}}, dir
}

func get(t *testing.T, s *Service, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handle(rec, httptest.NewRequest(http.MethodGet, path, nil))
	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body)
}

// TestServesManifestAndPayloadSeparately is the regression that matters.
//
// The client does not make one request. It fetches contents_0101.bin, decrypts
// it, reads regulation_0101.bin's name and SizeEnc out of the manifest, and then
// asks for that file too. Serving the manifest for every path handed it 640 bytes
// where 674992 were promised — a body that fails its size and DIGEST checks and
// looks nothing like a missing file.
func TestServesManifestAndPayloadSeparately(t *testing.T) {
	s, _ := testService(t)

	if code, body := get(t, s, "/contents_0101.bin"); code != http.StatusOK || body != "MANIFEST" {
		t.Errorf("manifest request: got %d %q, want 200 %q", code, body, "MANIFEST")
	}
	if code, body := get(t, s, "/regulation_0101.bin"); code != http.StatusOK || body != "REGULATION-PAYLOAD" {
		t.Errorf("payload request: got %d %q, want 200 %q — serving the manifest here "+
			"is the bug this test exists for", code, body, "REGULATION-PAYLOAD")
	}
}

// TestUnknownPathIs404 pins that we do not invent a body. A wrong-length 200 is
// worse than a 404: it fails the client's DIGEST check in a way that looks like
// corruption rather than a file we simply do not host.
func TestUnknownPathIs404(t *testing.T) {
	s, _ := testService(t)
	if code, _ := get(t, s, "/no_such_file.bin"); code != http.StatusNotFound {
		t.Errorf("unknown path: got %d, want 404", code)
	}
}

// TestResolveRejectsTraversal keeps the handler from serving anything outside the
// bootstrap directory. This is reachable by anything that can hit port 80.
func TestResolveRejectsTraversal(t *testing.T) {
	s, dir := testService(t)
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(secret) })

	for _, p := range []string{
		"/../secret.txt",
		"/../../etc/passwd",
		"/%2e%2e/secret.txt",
		"/subdir/../../secret.txt",
	} {
		if code, body := get(t, s, p); code == http.StatusOK {
			t.Errorf("%s: served %d %q; must not escape the bootstrap directory", p, code, body)
		}
	}
}
