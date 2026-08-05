package login

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/crypto/frpgcipher"
	"github.com/sstreight/dso/internal/crypto/keys"
	"github.com/sstreight/dso/internal/frpg/message"
	"github.com/sstreight/dso/internal/logging"
	"github.com/sstreight/dso/internal/proto/sharedpb"
	"github.com/sstreight/dso/internal/server/core"
)

// TestLoginCheckpoint1 is CP1: a client connects to the login service over TCP,
// sends an RSA-encrypted RequestQueryLoginServerInfo, and gets back the auth
// server address. This exercises TCP framing plus both RSA paddings end-to-end
// over a real socket.
func TestLoginCheckpoint1(t *testing.T) {
	// Pick a free port for the login listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	dir := t.TempDir()
	priv, err := keys.LoadOrGenerate(dir+"/server.private.pem", dir+"/server.public.pem", 2048)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.BindAddress = "127.0.0.1"
	cfg.LoginPort = port
	cfg.AuthPort = 50000
	cfg.AdvertiseAddress = "203.0.113.9"         // public
	cfg.AdvertisePrivateAddress = "192.168.1.50" // used for our loopback client
	cfg.KeyDir = dir

	srv := &core.Server{Config: cfg, Logger: logging.New("error", "text"), Key: priv}
	svc := New(srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		if err := svc.Serve(ctx); err != nil {
			t.Errorf("serve: %v", err)
		}
	}()

	// Wait for the listener to come up.
	var conn net.Conn
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if conn == nil {
		t.Fatalf("could not connect: %v", err)
	}
	defer conn.Close()

	// Client side: RSA client ciphers with the server's public key.
	stream := message.NewStream(conn)
	enc, dec := frpgcipher.NewRSAClient(&priv.PublicKey)
	stream.SetCiphers(enc, dec)

	req := &sharedpb.RequestQueryLoginServerInfo{
		SteamId:    proto.String("76561198000000000"),
		AppVersion: proto.Uint64(17039619),
	}
	body, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(message.Message{Type: message.RequestQueryLoginServerInfo, Index: 42, Payload: body}); err != nil {
		t.Fatal(err)
	}

	reply, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if reply.Type != message.Reply {
		t.Fatalf("reply type = %v, want Reply", reply.Type)
	}
	if reply.Index != 42 {
		t.Errorf("reply index = %d, want 42", reply.Index)
	}
	// PS3 sends all-varint fields, not the schema's string; see serverinfo.go.
	gotIP, gotPort, err := decodeServerInfoPS3(reply.Payload)
	if err != nil {
		t.Fatal(err)
	}
	// The client dials from 127.0.0.1 (loopback => private), so it must get the
	// private advertise address.
	if gotIP.String() != "192.168.1.50" {
		t.Errorf("server_ip = %q, want private address 192.168.1.50", gotIP)
	}
	if gotPort != 50000 {
		t.Errorf("port = %d, want 50000", gotPort)
	}
	t.Logf("CP1 ok: directed to auth server %s:%d", gotIP, gotPort)
}
