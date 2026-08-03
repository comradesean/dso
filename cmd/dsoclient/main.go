// Command dsoclient drives the login and auth handshake against a dso server
// using the client emulator. It is the automated checkpoint driver and a
// convenience for verifying a running server.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sstreight/dso/internal/client"
	"github.com/sstreight/dso/internal/crypto/keys"
)

func main() {
	loginAddr := flag.String("login-addr", "127.0.0.1:50050", "login server host:port")
	pubKeyPath := flag.String("public-key", "data/keys/server.public.pem", "server public key PEM")
	steamID := flag.String("steam-id", "0110000100000000", "identity string sent to the server")
	appVersion := flag.Uint64("app-version", 100, "app version reported to the server")
	ticket := flag.String("ticket", "dev-ticket", "platform ticket bytes")
	flag.Parse()

	pem, err := os.ReadFile(*pubKeyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading public key:", err)
		os.Exit(1)
	}
	pub, err := keys.ParsePublicPEM(pem)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error parsing public key:", err)
		os.Exit(1)
	}

	emu := client.Config{
		LoginAddr:  *loginAddr,
		PublicKey:  pub,
		SteamID:    *steamID,
		AppVersion: *appVersion,
		Ticket:     []byte(*ticket),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := emu.RunLoginAuth(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "handshake failed:", err)
		os.Exit(1)
	}
	fmt.Printf("OK login+auth complete\n")
	fmt.Printf("  game server: %s:%d\n", res.GameServerIP, res.GamePort)
	fmt.Printf("  auth token:  %x\n", res.AuthToken)
	fmt.Printf("  game key:    %x\n", res.GameKey)
}
