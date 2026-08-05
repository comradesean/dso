// Package bootstrap serves the HTTP preflight that the Dark Souls 2 PS3 client
// performs before it will go online. Pressing "Go Online" makes the game GET
// http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0101.bin;
// if that fails it drops to offline mode. We redirect that host to this server
// (DNS, or the RPCS3 IP/Hosts switch) and answer here.
//
// contents_0101.bin is NOT a calibration blob or a server-address bootstrap. It
// is an encrypted patch *manifest*, and it has been fully decrypted: a 256-byte
// RSA header followed by AES-128-CBC ciphertext whose plaintext is a plain
// key/value list naming exactly one file to download —
//
//	Patch.List.Count         = 1
//	Patch.List.File0.Name    = regulation.bnd
//	Patch.List.File0.Path    = http://<same host>/regulation_0101.bin
//	Patch.List.File0.SizeEnc = 674992
//	Patch.List.File0.DIGEST  = 6D817AB1...
//
// So a single request is never the whole story: the client follows the manifest
// and asks us for regulation_0101.bin next. Serving one file for every path (as
// this once did) hands the client 640 bytes where the manifest promised 674992,
// which decrypts to garbage. Requests are therefore routed by path.
//
// Verified end to end: decrypting regulation_0101.bin yields a DCX whose inflated
// BND4 HMAC-SHA1s to exactly the DIGEST above.
//
// Ten calibrations were published (0101, 0104, 0107-0114; Jan 2014 - Apr 2015)
// and all are archived locally. The EBOOT hardcodes the 0101 URL, so a real
// client can only ever ask for that one — but DSO_CALIBRATION_VERSION answers
// that request with a different version's manifest, and since each manifest names
// its own regulation file the client follows it from there unaided. Serving 0114
// is how a client gets the final event-item table (which added Human Effigy lots
// and a 90/5/3/2 Titanite Chunk/Slab/Twinkling/Dragon Bone drop).
package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sstreight/dso/internal/server/core"
)

// Service is the HTTP bootstrap/preflight server.
type Service struct {
	srv *core.Server
}

// New creates the bootstrap service.
func New(srv *core.Server) *Service { return &Service{srv: srv} }

// Name implements core.Service.
func (s *Service) Name() string { return "bootstrap-http" }

// Serve listens on the configured HTTP port until ctx is cancelled. It binds the
// advertised address specifically (the IP the client is redirected to), which
// also avoids clashing with any other :80 listener on a different interface.
func (s *Service) Serve(ctx context.Context) error {
	bindHost := s.srv.Config.AdvertiseAddress
	if bindHost == "" {
		bindHost = s.srv.Config.BindAddress
	}
	addr := fmt.Sprintf("%s:%d", bindHost, s.srv.Config.BootstrapHTTPPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Port 80 needs privilege. Log an actionable message but do NOT fail the
		// whole server — login/auth should keep running.
		s.srv.Logger.Error("bootstrap http could not bind; run: sudo sysctl -w net.ipv4.ip_unprivileged_port_start=80 (then restart)",
			"addr", addr, "err", err)
		return nil
	}
	version := s.srv.Config.CalibrationVersion
	if version == "" {
		version = "(as requested)"
	}
	s.srv.Logger.Info("bootstrap http listening", "addr", addr,
		"calibration_dir", s.dir(), "calibration_version", version)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	httpSrv := &http.Server{Handler: mux}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Service) handle(w http.ResponseWriter, r *http.Request) {
	log := s.srv.Logger.With("service", "bootstrap", "peer", r.RemoteAddr)
	// Log the request in full — this is our reverse-engineering surface.
	headers := make([]any, 0, len(r.Header)*2+8)
	headers = append(headers, "method", r.Method, "host", r.Host, "url", r.URL.String(), "proto", r.Proto)
	for k, v := range r.Header {
		headers = append(headers, "hdr:"+k, fmt.Sprint(v))
	}
	log.Info("bootstrap request", headers...)

	if filePath, ok := s.resolve(r.URL.Path); ok {
		if data, err := os.ReadFile(filePath); err == nil {
			w.Header().Set("Content-Type", "application/octet-stream")
			// The client checks SizeEnc against the manifest, so the length must be
			// the file's own.
			//
			// Last-Modified is the file's mtime, which means the mtime has to be set
			// deliberately: the origin serves 2014-01-26 for both payloads, but a
			// plain re-fetch (curl -o, git checkout, a copy) stamps them with today.
			// After re-fetching, restore them:
			//
			//	touch -d '2014-01-26 02:00:02 UTC' data/contents_0101.bin
			//	touch -d '2014-01-26 02:00:01 UTC' data/regulation_0101.bin
			//
			// Nothing is known to depend on this, but a 2026 date on a payload frozen
			// since before the game shipped is a lie we would rather not tell.
			if fi, err := os.Stat(filePath); err == nil {
				w.Header().Set("Last-Modified", fi.ModTime().UTC().Format(http.TimeFormat))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			log.Info("served bootstrap file", "path", filePath, "bytes", len(data))
			return
		} else {
			log.Warn("could not read bootstrap file", "path", filePath, "err", err)
		}
	}

	// 404 rather than an empty 200 or the wrong file. A wrong-length body is worse
	// than no body: the client validates it against the manifest's SizeEnc and
	// DIGEST, so serving something plausible-but-wrong produces a decrypt failure
	// that looks nothing like a missing file.
	http.NotFound(w, r)
	log.Warn("no bootstrap file for request", "url", r.URL.Path)
}

// contentsName matches the manifest filename, capturing its 4-digit calibration
// version. Only contents_ is remapped; regulation_ files are fetched by the name
// the manifest itself gives, so they must be served exactly as asked.
var contentsName = regexp.MustCompile(`^contents_(\d{4})\.bin$`)

// resolve maps a request path to a file on disk.
//
// Two requests arrive per boot and both are served from the calibration
// directory: contents_NNNN.bin (the manifest) and then the regulation file it
// names.
//
// If CalibrationVersion is set, a request for any contents_NNNN.bin is answered
// with that version's manifest instead. That is the whole mechanism for serving a
// calibration other than the one the EBOOT hardcodes — the manifest names its own
// regulation file, so the client follows it there without further help.
func (s *Service) resolve(urlPath string) (string, bool) {
	dir := s.dir()
	if dir == "" {
		return "", false
	}

	name := path.Base(urlPath)
	// path.Base cannot escape the directory, but be explicit: only plain names.
	if name == "" || name == "." || name == "/" || strings.ContainsAny(name, `/\`) {
		return "", false
	}

	if v := s.srv.Config.CalibrationVersion; v != "" {
		if contentsName.MatchString(name) {
			name = "contents_" + v + ".bin"
		}
	}

	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); err != nil {
		// Fall back to the explicitly configured manifest. This keeps a
		// deployment working where that file was renamed or sits outside the
		// calibration directory.
		if cf := s.srv.Config.BootstrapContentsFile; cf != "" &&
			name == path.Base(filepath.ToSlash(cf)) {
			return cf, true
		}
		return "", false
	}
	return candidate, true
}

// dir is the directory calibration payloads are served from.
func (s *Service) dir() string {
	if d := s.srv.Config.BootstrapCalibrationDir; d != "" {
		return d
	}
	if cf := s.srv.Config.BootstrapContentsFile; cf != "" {
		return filepath.Dir(cf)
	}
	return ""
}
