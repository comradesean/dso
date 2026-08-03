package auth_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/sstreight/dso/internal/client"
	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/crypto/keys"
	"github.com/sstreight/dso/internal/identity"
	"github.com/sstreight/dso/internal/logging"
	"github.com/sstreight/dso/internal/server/auth"
	"github.com/sstreight/dso/internal/server/authtoken"
	"github.com/sstreight/dso/internal/server/core"
	"github.com/sstreight/dso/internal/server/login"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestAuthCheckpoint2 is CP2, the milestone-1 hard gate: a client completes the
// login query and the full four-step auth handshake (RSA -> CWC key exchange ->
// service status -> game key -> ticket) and receives a well-formed 184-byte
// Frpg2GameServerInfo, with the auth token registered for the game service.
func TestAuthCheckpoint2(t *testing.T) {
	loginPort := freePort(t)
	authPort := freePort(t)

	dir := t.TempDir()
	priv, err := keys.LoadOrGenerate(dir+"/server.private.pem", dir+"/server.public.pem", 2048)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.BindAddress = "127.0.0.1"
	cfg.AdvertiseAddress = "127.0.0.1" // so the emulator can dial the auth/game address
	cfg.LoginPort = loginPort
	cfg.AuthPort = authPort
	cfg.GamePort = 50010
	cfg.KeyDir = dir

	srv := &core.Server{
		Config: cfg,
		Logger: logging.New("error", "text"),
		Key:    priv,
		Tokens: authtoken.NewRegistry(),
		Auth:   identity.Noop{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, svc := range []core.Service{login.New(srv), auth.New(srv)} {
		go func(s core.Service) {
			if err := s.Serve(ctx); err != nil {
				t.Errorf("%s serve: %v", s.Name(), err)
			}
		}(svc)
	}
	waitForPort(t, loginPort)
	waitForPort(t, authPort)

	emu := client.Config{
		LoginAddr:  net.JoinHostPort("127.0.0.1", strconv.Itoa(loginPort)),
		PublicKey:  &priv.PublicKey,
		SteamID:    "76561198000000000",
		AppVersion: 17039619,
		Ticket:     []byte("fake-psn-ticket-bytes"),
	}
	res, err := emu.RunLoginAuth(ctx)
	if err != nil {
		t.Fatalf("login+auth handshake failed: %v", err)
	}

	if res.GameServerIP != "127.0.0.1" {
		t.Errorf("game server ip = %q, want 127.0.0.1", res.GameServerIP)
	}
	if res.GamePort != 50010 {
		t.Errorf("game port = %d, want 50010", res.GamePort)
	}
	if len(res.GameKey) != 16 {
		t.Errorf("game key length = %d, want 16", len(res.GameKey))
	}
	if res.AuthToken == ([8]byte{}) {
		t.Error("auth token is all zero")
	}

	// The token must be registered with the game key for the UDP service.
	key, ok := srv.Tokens.Lookup(authtoken.Token(res.AuthToken))
	if !ok {
		t.Fatal("auth token was not registered in the game service")
	}
	if string(key) != string(res.GameKey) {
		t.Error("registered CWC key does not match the negotiated game key")
	}
	t.Logf("CP2 ok: game server %s:%d token=%x", res.GameServerIP, res.GamePort, res.AuthToken)
}

func waitForPort(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("port %d did not come up", port)
}
