// Package bootstrap serves the HTTP "calibration" preflight that the Dark Souls
// 2 PS3 client performs before it will go online. Pressing "Go Online" makes the
// game GET http://frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com/contents_0101.bin
// (its version/regulation check); if that fails it drops to offline mode. We
// redirect that host to this server (RPCS3 IP/Hosts switch) and answer here.
//
// The exact expected body is still being reverse-engineered, so this service
// logs every request in full and serves a configurable response, letting us
// iterate on what the client accepts.
package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
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

	// If a contents file is configured, serve it for the calibration path.
	if s.srv.Config.BootstrapContentsFile != "" {
		if data, err := os.ReadFile(s.srv.Config.BootstrapContentsFile); err == nil {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
			log.Info("served contents file", "bytes", len(data))
			return
		} else {
			log.Warn("could not read configured contents file", "err", err)
		}
	}

	// Default: an empty 200. We iterate on this based on how the client reacts.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	log.Info("served empty 200")
}
