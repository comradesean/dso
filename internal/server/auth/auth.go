// Package auth implements the Frpg2 auth service (TCP). It runs a four-step
// handshake — RSA handshake with a CWC key exchange, service-status/version
// gate, game-key material exchange, and ticket validation — then hands the
// client a game-server address plus an auth token registered for the UDP game
// service.
package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"net/netip"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
	"github.com/sstreight/dso/internal/frpg/message"
	"github.com/sstreight/dso/internal/netdebug"
	"github.com/sstreight/dso/internal/proto/sharedpb"
	"github.com/sstreight/dso/internal/server/authtoken"
	"github.com/sstreight/dso/internal/server/core"
)

const clientTimeout = 120 * time.Second

// Service is the auth TCP service.
type Service struct {
	srv *core.Server
}

// New creates an auth service bound to the given server.
func New(srv *core.Server) *Service { return &Service{srv: srv} }

// Name implements core.Service.
func (s *Service) Name() string { return "auth" }

// Serve listens and accepts connections until ctx is cancelled.
func (s *Service) Serve(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.srv.Config.BindAddress, s.srv.Config.AuthPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s.srv.Logger.Info("auth service listening", "addr", addr)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go s.handle(ctx, conn)
	}
}

func (s *Service) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	log := s.srv.Logger.With("service", "auth", "peer", conn.RemoteAddr().String())
	log.Info("auth connection opened")

	if s.srv.Config.DebugRaw {
		conn = netdebug.Wrap(conn, log)
	}
	stream := message.NewStream(conn)

	// Stage 1: RSA handshake + CWC key exchange.
	rsaEnc, rsaDec := frpgcipher.NewRSAServer(s.srv.Key)
	stream.SetCiphers(rsaEnc, rsaDec)

	// A stage failing part-way through is a real problem (bad crypto, an
	// unexpected message, a version rejection) and must be visible at the
	// default log level; only an ordinary disconnect is demoted to Debug.
	if err := s.doHandshake(conn, stream, log); err != nil {
		logStageFailure(log, "handshake failed", err)
		return
	}

	// Stage 2: service status / version gate.
	psnID, err := s.doServiceStatus(conn, stream, log)
	if err != nil {
		logStageFailure(log, "service status failed", err)
		return
	}

	// Stage 3: game-key material exchange.
	gameKey, err := s.doKeyMaterial(conn, stream, log)
	if err != nil {
		logStageFailure(log, "key material failed", err)
		return
	}

	// Stage 4: ticket + game server info.
	if err := s.doPSNTicket(ctx, conn, stream, log, psnID, gameKey); err != nil {
		logStageFailure(log, "ticket exchange failed", err)
		return
	}

	// Authentication complete; the client disconnects and moves to UDP.
}

// logStageFailure reports a handshake stage that did not complete. An ordinary
// disconnect is routine and stays at Debug; anything else is a genuine failure
// and is logged at Warn so it is visible without turning on debug logging.
func logStageFailure(log logger, msg string, err error) {
	if core.IsBenignDisconnect(err) {
		log.Debug(msg, "err", err)
		return
	}
	log.Warn(msg, "err", err)
}

// recv reads the next message with the read deadline applied and checks its type.
func (s *Service) recv(conn net.Conn, stream *message.Stream, want message.Type) (message.Message, error) {
	_ = conn.SetReadDeadline(time.Now().Add(clientTimeout))
	msg, err := stream.Recv()
	if err != nil {
		return message.Message{}, err
	}
	if msg.Type != want {
		return message.Message{}, fmt.Errorf("expected %v, got %v", want, msg.Type)
	}
	return msg, nil
}

func (s *Service) doHandshake(conn net.Conn, stream *message.Stream, log logger) error {
	msg, err := s.recv(conn, stream, message.RequestHandshake)
	if err != nil {
		return err
	}
	var req sharedpb.RequestHandshake
	if err := proto.Unmarshal(msg.Payload, &req); err != nil {
		return fmt.Errorf("parse RequestHandshake: %w", err)
	}
	cwcKey := req.GetAesCwcKey()
	if len(cwcKey) != 16 {
		return fmt.Errorf("handshake cwc key is %d bytes, want 16", len(cwcKey))
	}

	// Send the 27-byte plaintext key-exchange blob (11 random + 16 zero), then
	// switch the stream to AES-CWC.
	stream.SetCiphers(nil, nil)
	blob := make([]byte, 27)
	if _, err := rand.Read(blob[:11]); err != nil {
		return err
	}
	if err := stream.Send(message.Message{Type: message.Reply, Index: msg.Index, Payload: blob}); err != nil {
		return err
	}

	cwc, err := frpgcipher.NewCWCTCP(cwcKey)
	if err != nil {
		return err
	}
	stream.SetCiphers(cwc, cwc)
	return nil
}

func (s *Service) doServiceStatus(conn net.Conn, stream *message.Stream, log logger) (string, error) {
	msg, err := s.recv(conn, stream, message.GetServiceStatus)
	if err != nil {
		return "", err
	}
	var req sharedpb.GetServiceStatus
	if err := proto.Unmarshal(msg.Payload, &req); err != nil {
		return "", fmt.Errorf("parse GetServiceStatus: %w", err)
	}

	appVersion := req.GetAppVersion()
	if !s.versionAccepted(uint64(appVersion)) {
		// Mirror the reference: send an empty response, which the client reads
		// as "update required".
		log.Warn("rejecting unsupported app version", "app_version", appVersion)
		_ = stream.Send(message.Message{Type: message.Reply, Index: msg.Index})
		return "", fmt.Errorf("unsupported app version %d", appVersion)
	}
	log.Info("service status", "psn_id", req.GetPsnId(), "app_version", appVersion)

	resp := &sharedpb.GetServiceStatusResponse{
		Id:         proto.Int64(2),
		PsnId:      proto.String(""),
		Unknown_1:  proto.Int64(0),
		AppVersion: proto.Int64(appVersion),
	}
	body, err := proto.Marshal(resp)
	if err != nil {
		return "", err
	}
	if err := stream.Send(message.Message{Type: message.Reply, Index: msg.Index, Payload: body}); err != nil {
		return "", err
	}
	return req.GetPsnId(), nil
}

func (s *Service) doKeyMaterial(conn net.Conn, stream *message.Stream, log logger) ([]byte, error) {
	msg, err := s.recv(conn, stream, message.KeyMaterial)
	if err != nil {
		return nil, err
	}
	if len(msg.Payload) != 8 {
		return nil, fmt.Errorf("key material payload is %d bytes, want 8", len(msg.Payload))
	}
	// gameKey = client's 8 bytes followed by 8 server-random bytes.
	gameKey := make([]byte, 16)
	copy(gameKey[0:8], msg.Payload)
	if _, err := rand.Read(gameKey[8:16]); err != nil {
		return nil, err
	}
	if err := stream.Send(message.Message{Type: message.Reply, Index: msg.Index, Payload: gameKey}); err != nil {
		return nil, err
	}
	return gameKey, nil
}

func (s *Service) doPSNTicket(ctx context.Context, conn net.Conn, stream *message.Stream, log logger, psnID string, gameKey []byte) error {
	msg, err := s.recv(conn, stream, message.PSNTicket)
	if err != nil {
		return err
	}
	if len(msg.Payload) < 16 {
		return fmt.Errorf("psn ticket payload is %d bytes, want >= 16", len(msg.Payload))
	}
	ticket := msg.Payload[16:]

	id, err := s.srv.Auth.Validate(ctx, psnID, ticket)
	if err != nil {
		return fmt.Errorf("ticket validation: %w", err)
	}

	// Choose the game-server address to advertise.
	serverIP := s.srv.Config.AdvertiseAddress
	if peer, ok := peerAddr(conn); ok {
		serverIP = s.srv.AdvertisedAddressFor(peer)
	}

	// Random 8-byte auth token.
	var token authtoken.Token
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	info, err := encodeGameServerInfo(token, serverIP, uint16(s.srv.Config.GamePort))
	if err != nil {
		return err
	}
	if err := stream.Send(message.Message{Type: message.Reply, Index: msg.Index, Payload: info}); err != nil {
		return err
	}

	s.srv.Tokens.Add(token, gameKey)
	log.Info("authentication complete",
		"identity", id.ID, "platform", id.Platform,
		"game_ip", serverIP, "game_port", s.srv.Config.GamePort,
		"token", fmt.Sprintf("%x", token))
	return nil
}

// versionAccepted applies the (lenient by default) app-version gate.
func (s *Service) versionAccepted(v uint64) bool {
	c := s.srv.Config
	if !c.EnforceAppVersion {
		return true
	}
	return v >= c.AppVersionMin && v <= c.AppVersionMax
}

// logger is the subset of slog.Logger used here.
type logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
}

func peerAddr(conn net.Conn) (netip.Addr, bool) {
	tcp, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return netip.Addr{}, false
	}
	a, ok := netip.AddrFromSlice(tcp.IP)
	if !ok {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}
