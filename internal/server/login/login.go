// Package login implements the Frpg2 login service (TCP). It answers a single
// message type, RequestQueryLoginServerInfo, directing the client to the auth
// server. The whole stream is RSA-encrypted with the server key pair.
package login

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
	"github.com/sstreight/dso/internal/frpg/message"
	"github.com/sstreight/dso/internal/proto/sharedpb"
	"github.com/sstreight/dso/internal/server/core"
)

// clientTimeout bounds how long a login connection may stay idle.
const clientTimeout = 120 * time.Second

// Service is the login TCP service.
type Service struct {
	srv *core.Server
}

// New creates a login service bound to the given server.
func New(srv *core.Server) *Service { return &Service{srv: srv} }

// Name implements core.Service.
func (s *Service) Name() string { return "login" }

// Serve listens and accepts connections until ctx is cancelled.
func (s *Service) Serve(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.srv.Config.BindAddress, s.srv.Config.LoginPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s.srv.Logger.Info("login service listening", "addr", addr)

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
	log := s.srv.Logger.With("service", "login", "peer", conn.RemoteAddr().String())

	stream := message.NewStream(conn)
	enc, dec := frpgcipher.NewRSAServer(s.srv.Key)
	stream.SetCiphers(enc, dec)

	for {
		if ctx.Err() != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(clientTimeout))

		msg, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Debug("connection closed", "err", err)
			}
			return
		}
		if msg.Type != message.RequestQueryLoginServerInfo {
			log.Warn("unexpected message type on login stream", "type", msg.Type.String())
			return
		}

		var req sharedpb.RequestQueryLoginServerInfo
		if err := proto.Unmarshal(msg.Payload, &req); err != nil {
			log.Warn("failed to parse RequestQueryLoginServerInfo", "err", err)
			return
		}

		serverIP := s.srv.Config.AdvertiseAddress
		if peer, ok := peerAddr(conn); ok {
			serverIP = s.srv.AdvertisedAddressFor(peer)
		}

		resp := &sharedpb.RequestQueryLoginServerInfoResponse{
			ServerIp: proto.String(serverIP),
			Port:     proto.Int64(int64(s.srv.Config.AuthPort)),
		}
		body, err := proto.Marshal(resp)
		if err != nil {
			log.Error("failed to marshal login response", "err", err)
			return
		}
		if err := stream.Send(message.Message{Type: message.Reply, Index: msg.Index, Payload: body}); err != nil {
			log.Warn("failed to send login response", "err", err)
			return
		}
		log.Info("directed client to auth server",
			"steam_id", req.GetSteamId(), "app_version", req.GetAppVersion(),
			"auth_ip", serverIP, "auth_port", s.srv.Config.AuthPort)
	}
}

// peerAddr extracts the remote IP as a netip.Addr.
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
