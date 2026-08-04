// Command dsoserver runs the dso game server. The active game is selected by
// DSO_GAME; all configuration comes from DSO_-prefixed environment variables.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/crypto/keys"
	"github.com/sstreight/dso/internal/logging"
	"github.com/sstreight/dso/internal/server/auth"
	"github.com/sstreight/dso/internal/server/bootstrap"
	"github.com/sstreight/dso/internal/server/core"
	"github.com/sstreight/dso/internal/server/dnsredirect"
	"github.com/sstreight/dso/internal/server/login"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel, cfg.LogFormat)

	srv, err := core.New(cfg, logger)
	if err != nil {
		return err
	}

	// Startup banner. The public-key fingerprint lets the operator cross-check
	// the key patched into the client.
	fp := publicKeyFingerprint(srv)
	logger.Info("starting dso server",
		"game", cfg.Game, "platform", cfg.Platform,
		"advertise", cfg.AdvertiseAddress,
		"login_port", cfg.LoginPort, "auth_port", cfg.AuthPort, "game_port", cfg.GamePort,
		"auth_mode", cfg.AuthModeValue, "pubkey_fp", fp)
	if cfg.AuthModeValue == config.AuthNoop {
		logger.Warn("auth mode is noop: any ticket is accepted; do not expose this on an untrusted network")
	}

	srv.AddService(login.New(srv))
	srv.AddService(auth.New(srv))
	if cfg.BootstrapHTTPEnabled {
		srv.AddService(bootstrap.New(srv))
	}
	if cfg.DNSEnabled {
		srv.AddService(dnsredirect.New(srv))
	}
	// the game (UDP) service is added as it is implemented.

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return srv.Run(ctx)
}

func publicKeyFingerprint(srv *core.Server) string {
	pem := keys.PublicPEM(&srv.Key.PublicKey)
	sum := sha256.Sum256(pem)
	return hex.EncodeToString(sum[:8])
}
