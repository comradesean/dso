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
// BND4 HMAC-SHA1s to exactly the DIGEST above. Its 252 params include
// ItemLotParam2_SvrEvent.param — the plausible delivery route for event items —
// but the 2014 payload is byte-identical to the on-disc regulation apart from a
// single version byte, so in practice it only rotates a version stamp.
package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
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
	s.srv.Logger.Info("bootstrap http listening", "addr", addr,
		"contents_file", s.srv.Config.BootstrapContentsFile)

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
			// the file's own. Report the file's real mtime rather than now: the real
			// origin serves 2014 dates and these payloads genuinely never change.
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

// resolve maps a request path to a file on disk.
//
// Files live alongside the configured contents file, so pointing
// DSO_BOOTSTRAP_CONTENTS_FILE at data/contents_0101.bin also serves
// data/regulation_0101.bin — which the client asks for immediately afterwards,
// having read its name out of the manifest.
func (s *Service) resolve(urlPath string) (string, bool) {
	configured := s.srv.Config.BootstrapContentsFile
	if configured == "" {
		return "", false
	}

	name := path.Base(urlPath)
	// path.Base cannot escape the directory, but be explicit: only plain names.
	if name == "" || name == "." || name == "/" || strings.ContainsAny(name, `/\`) {
		return "", false
	}
	// The manifest fetch itself keeps working even if the deployed file has been
	// renamed, since that is the path the EBOOT hardcodes.
	if name == path.Base(filepath.ToSlash(configured)) {
		return configured, true
	}

	candidate := filepath.Join(filepath.Dir(configured), name)
	if _, err := os.Stat(candidate); err != nil {
		return "", false
	}
	return candidate, true
}
