// Package client is a client-side emulator of the Frpg2 handshake, mirroring the
// real client's login -> auth flow. It is the primary automated driver for the
// milestone checkpoints and a development accelerator, not the acceptance gate
// (that is a real client under RPCS3).
package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
	"github.com/sstreight/dso/internal/frpg/message"
	"github.com/sstreight/dso/internal/proto/sharedpb"
)

// Config configures the emulator.
type Config struct {
	LoginAddr   string // host:port of the login server
	PublicKey   *rsa.PublicKey
	SteamID     string
	AppVersion  uint64
	Ticket      []byte // platform ticket bytes (any bytes for the noop validator)
	DialTimeout time.Duration
}

// AuthResult is what the auth handshake yields.
type AuthResult struct {
	AuthToken    [8]byte
	GameKey      []byte
	GameServerIP string
	GamePort     int
}

func (c Config) dialTimeout() time.Duration {
	if c.DialTimeout == 0 {
		return 5 * time.Second
	}
	return c.DialTimeout
}

// RunLoginAuth performs the login query and the full auth handshake, returning
// the game-server address, auth token, and negotiated game CWC key.
func (c Config) RunLoginAuth(ctx context.Context) (AuthResult, error) {
	authAddr, err := c.login(ctx)
	if err != nil {
		return AuthResult{}, fmt.Errorf("login: %w", err)
	}
	res, err := c.auth(ctx, authAddr)
	if err != nil {
		return AuthResult{}, fmt.Errorf("auth: %w", err)
	}
	return res, nil
}

// login connects to the login server and returns the auth server's address.
func (c Config) login(ctx context.Context) (string, error) {
	conn, err := dial(ctx, c.LoginAddr, c.dialTimeout())
	if err != nil {
		return "", err
	}
	defer conn.Close()

	stream := message.NewStream(conn)
	enc, dec := frpgcipher.NewRSAClient(c.PublicKey)
	stream.SetCiphers(enc, dec)

	req := &sharedpb.RequestQueryLoginServerInfo{
		SteamId:    proto.String(c.SteamID),
		AppVersion: proto.Uint64(c.AppVersion),
	}
	body, err := proto.Marshal(req)
	if err != nil {
		return "", err
	}
	if err := stream.Send(message.Message{Type: message.RequestQueryLoginServerInfo, Index: 1, Payload: body}); err != nil {
		return "", err
	}
	reply, err := stream.Recv()
	if err != nil {
		return "", err
	}
	var resp sharedpb.RequestQueryLoginServerInfoResponse
	if err := proto.Unmarshal(reply.Payload, &resp); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", resp.GetServerIp(), resp.GetPort()), nil
}

// auth performs the four-step auth handshake.
func (c Config) auth(ctx context.Context, addr string) (AuthResult, error) {
	conn, err := dial(ctx, addr, c.dialTimeout())
	if err != nil {
		return AuthResult{}, err
	}
	defer conn.Close()

	stream := message.NewStream(conn)
	enc, dec := frpgcipher.NewRSAClient(c.PublicKey)
	stream.SetCiphers(enc, dec)

	var index uint32 = 1

	// Step 1: handshake — send a chosen CWC key, receive the 27-byte blob,
	// switch to AES-CWC.
	cwcKey := make([]byte, 16)
	if _, err := rand.Read(cwcKey); err != nil {
		return AuthResult{}, err
	}
	hs := &sharedpb.RequestHandshake{AesCwcKey: cwcKey}
	body, err := proto.Marshal(hs)
	if err != nil {
		return AuthResult{}, err
	}
	if err := stream.Send(message.Message{Type: message.RequestHandshake, Index: index, Payload: body}); err != nil {
		return AuthResult{}, err
	}
	stream.SetCiphers(nil, nil)
	blob, err := stream.Recv()
	if err != nil {
		return AuthResult{}, err
	}
	if len(blob.Payload) != 27 {
		return AuthResult{}, fmt.Errorf("handshake blob is %d bytes, want 27", len(blob.Payload))
	}
	cwc, err := frpgcipher.NewCWCTCP(cwcKey)
	if err != nil {
		return AuthResult{}, err
	}
	stream.SetCiphers(cwc, cwc)

	// Step 2: service status.
	index++
	ss := &sharedpb.GetServiceStatus{
		Id:         proto.Int64(1),
		SteamId:    proto.String(c.SteamID),
		AppVersion: proto.Int64(int64(c.AppVersion)),
	}
	if body, err = proto.Marshal(ss); err != nil {
		return AuthResult{}, err
	}
	if err := stream.Send(message.Message{Type: message.GetServiceStatus, Index: index, Payload: body}); err != nil {
		return AuthResult{}, err
	}
	if _, err := stream.Recv(); err != nil {
		return AuthResult{}, err
	}

	// Step 3: key material — send 8 random bytes, receive the 16-byte game key.
	index++
	half := make([]byte, 8)
	if _, err := rand.Read(half); err != nil {
		return AuthResult{}, err
	}
	if err := stream.Send(message.Message{Type: message.KeyMaterial, Index: index, Payload: half}); err != nil {
		return AuthResult{}, err
	}
	keyReply, err := stream.Recv()
	if err != nil {
		return AuthResult{}, err
	}
	if len(keyReply.Payload) != 16 {
		return AuthResult{}, fmt.Errorf("game key is %d bytes, want 16", len(keyReply.Payload))
	}
	gameKey := keyReply.Payload

	// Step 4: ticket — send [gameKey(16) | ticket], receive the 184-byte info.
	index++
	ticketMsg := make([]byte, 0, 16+len(c.Ticket))
	ticketMsg = append(ticketMsg, gameKey...)
	ticketMsg = append(ticketMsg, c.Ticket...)
	if err := stream.Send(message.Message{Type: message.SteamTicket, Index: index, Payload: ticketMsg}); err != nil {
		return AuthResult{}, err
	}
	infoReply, err := stream.Recv()
	if err != nil {
		return AuthResult{}, err
	}
	return parseGameServerInfo(infoReply.Payload, gameKey)
}

// parseGameServerInfo decodes the 56-byte Frpg2GameServerInfo the real DS2 PS3
// client expects. The client enforces the length with a hard equality check and
// silently discards a mismatched struct, so this emulator is strict too — being
// lenient here would hide exactly the bug the real client hits.
//
// See internal/server/auth/gameserverinfo.go for the recovered layout and the
// addresses it came from.
func parseGameServerInfo(buf, gameKey []byte) (AuthResult, error) {
	if len(buf) != 56 {
		return AuthResult{}, fmt.Errorf("game server info is %d bytes, want 56", len(buf))
	}
	var res AuthResult
	copy(res.AuthToken[:], buf[0:8])
	res.GameKey = gameKey
	// game_server_ip is a raw binary u32 (a.b.c.d), not an ASCII string.
	res.GameServerIP = netip.AddrFrom4([4]byte(buf[8:12])).String()
	res.GamePort = int(binary.BigEndian.Uint16(buf[12:14]))
	return res, nil
}

func dial(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "tcp", addr)
}
