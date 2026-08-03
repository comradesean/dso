// Package core holds the server lifecycle: it loads the RSA key, constructs the
// per-service listeners, and runs them until the context is cancelled or a
// service fails fatally.
package core

import (
	"context"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/crypto/keys"
	"github.com/sstreight/dso/internal/identity"
	"github.com/sstreight/dso/internal/server/authtoken"
)

// Service is a long-running server component (login, auth, game).
type Service interface {
	Name() string
	// Serve runs until ctx is cancelled (returning nil) or a fatal error occurs.
	Serve(ctx context.Context) error
}

// Server owns shared state and orchestrates services.
type Server struct {
	Config config.Config
	Logger *slog.Logger
	Key    *rsa.PrivateKey

	// Tokens maps a game-server auth token to its CWC key; written by the auth
	// service, read by the game service.
	Tokens *authtoken.Registry
	// Auth validates platform tickets.
	Auth identity.Validator

	services []Service
}

// New constructs a server, loading or generating the RSA key pair and selecting
// the ticket validator from configuration.
func New(cfg config.Config, logger *slog.Logger) (*Server, error) {
	key, err := keys.LoadOrGenerate(cfg.PrivateKeyPath(), cfg.PublicKeyPath(), 2048)
	if err != nil {
		return nil, fmt.Errorf("core: load key: %w", err)
	}
	validator, err := newValidator(cfg.AuthModeValue)
	if err != nil {
		return nil, err
	}
	return &Server{
		Config: cfg,
		Logger: logger,
		Key:    key,
		Tokens: authtoken.NewRegistry(),
		Auth:   validator,
	}, nil
}

// newValidator selects a ticket validator. Only the no-op validator is wired up
// for now; psn/steam are future work.
func newValidator(mode config.AuthMode) (identity.Validator, error) {
	switch mode {
	case config.AuthNoop, "":
		return identity.Noop{}, nil
	default:
		return nil, fmt.Errorf("core: auth mode %q not implemented yet", mode)
	}
}

// AddService registers a service to be run by Run.
func (s *Server) AddService(svc Service) { s.services = append(s.services, svc) }

// Run starts all registered services and blocks until ctx is cancelled or any
// service returns a non-nil error. On the first fatal error it cancels the rest
// and returns that error.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, len(s.services))

	for _, svc := range s.services {
		wg.Add(1)
		go func(svc Service) {
			defer wg.Done()
			if err := svc.Serve(ctx); err != nil {
				errCh <- fmt.Errorf("%s: %w", svc.Name(), err)
				cancel()
			}
		}(svc)
	}

	<-ctx.Done()
	cancel()
	wg.Wait()
	close(errCh)

	// Return the first fatal error, if any (context cancellation is not an error).
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// AdvertisedAddressFor returns the address to hand to a client, choosing the
// private advertise address when the peer is on a private/loopback network and
// one is configured.
func (s *Server) AdvertisedAddressFor(peer netip.Addr) string {
	if s.Config.AdvertisePrivateAddress != "" && isPrivate(peer) {
		return s.Config.AdvertisePrivateAddress
	}
	return s.Config.AdvertiseAddress
}

// isPrivate reports whether addr is on a LAN-like network (RFC1918/ULA,
// loopback, or link-local), mirroring the reference's IsPrivateNetwork.
func isPrivate(addr netip.Addr) bool {
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast()
}
