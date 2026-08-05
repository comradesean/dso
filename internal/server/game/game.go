// Package game implements the Frpg2 game service (UDP). After the auth service
// hands a client a game-server address and an 8-byte auth token, the client
// opens a reliable-UDP session here.
//
// Every client->server datagram is prefixed with that token in the clear, which
// is what lets a connectionless listener demux datagrams and find the per-client
// CWC key before it can decrypt anything. Sessions are keyed by source address;
// the token is re-checked on every datagram so a peer cannot drift onto another
// client's session.
//
// Message handling is deliberately minimal for now: the session is established
// and every decoded message is logged in full. That capture is what the DS2
// boot/player-data handlers will be built from, rather than guessing at the
// message set — the same approach that got the auth handshake working.
package game

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sstreight/dso/internal/crypto/frpgcipher"
	"github.com/sstreight/dso/internal/frpg/rudp"
	"github.com/sstreight/dso/internal/server/authtoken"
	"github.com/sstreight/dso/internal/server/core"
	"github.com/sstreight/dso/internal/server/store"
)

const (
	// maxDatagram bounds a single read. Frpg2 datagrams are far smaller; this
	// is only a guard against a hostile or corrupt length.
	maxDatagram = 4096
	// pumpInterval drives retransmits, heartbeats and connection timeouts.
	pumpInterval = 100 * time.Millisecond
	// sessionIdle is how long a session may go without a datagram before it is
	// dropped and its slot reclaimed.
	sessionIdle = 60 * time.Second
)

// Service is the game UDP service.
type Service struct {
	srv *core.Server

	mu        sync.Mutex
	sessions  map[string]*clientSession // keyed by remote address
	playerSeq uint32                    // hands out player ids

	// store persists blood messages. Ghosts and bloodstains stay in memory: they
	// are high-volume and disposable, and the reference keeps them memory-only by
	// default too. Each has its own lock, so none of these may be touched while
	// holding s.mu... except the store, whose calls are short and serialized by
	// SQLite anyway.
	store       *store.Store
	ghosts      *ghostStore
	bloodstains *bloodstainStore
	signs       *signStore
}

// clientSession is one client's reliable-UDP session and its crypto state.
type clientSession struct {
	token     authtoken.Token
	cipher    *frpgcipher.ServerUDPCipher
	sess      *rudp.Session
	conn      *rudp.MessageConn
	lastSeen  time.Time
	lastState rudp.State

	// Identity, populated by RequestWaitForUserLogin.
	accountID string
	playerID  uint32
	// characterID is the slot assigned by RequestUpdateLoginPlayerCharacter.
	characterID uint32
	// status is the latest opaque AllStatus blob from RequestUpdatePlayerStatus;
	// matchmaking filters will read soul memory and area out of it.
	status []byte
}

// New creates a game service bound to the given server. st persists blood
// messages and must be non-nil.
func New(srv *core.Server, st *store.Store) *Service {
	return &Service{
		srv:         srv,
		sessions:    make(map[string]*clientSession),
		store:       st,
		ghosts:      newGhostStore(),
		bloodstains: newBloodstainStore(),
		signs:       newSignStore(),
	}
}

// Name implements core.Service.
func (s *Service) Name() string { return "game" }

// Serve runs the UDP listener until ctx is cancelled.
func (s *Service) Serve(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.srv.Config.BindAddress, s.srv.Config.GamePort)
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("listen udp %s: %w", addr, err)
	}
	s.srv.Logger.Info("game service listening", "addr", addr)

	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()

	go s.pumpLoop(ctx)

	buf := make([]byte, maxDatagram)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read udp: %w", err)
		}
		datagram := make([]byte, n)
		copy(datagram, buf[:n])
		s.handleDatagram(pc, from, datagram)
	}
}

// handleDatagram demuxes one client->server datagram: find or create the
// session for its token, decrypt, and feed the reliable-UDP layer.
func (s *Service) handleDatagram(pc net.PacketConn, from net.Addr, datagram []byte) {
	tok, ok := frpgcipher.TokenFromDatagram(datagram)
	if !ok {
		s.srv.Logger.Debug("game: datagram too short to carry a token",
			"peer", from.String(), "bytes", len(datagram))
		return
	}

	cs, err := s.session(pc, from, tok)
	if err != nil {
		// Unknown or expired token: stay silent rather than confirming to an
		// unauthenticated peer that anything is listening.
		s.srv.Logger.Debug("game: rejecting datagram",
			"peer", from.String(), "token", hex.EncodeToString(tok[:]), "err", err)
		return
	}

	pt, prefix, err := cs.cipher.Open(datagram)
	if err != nil {
		s.srv.Logger.Warn("game: datagram failed to decrypt",
			"peer", from.String(), "token", hex.EncodeToString(tok[:]), "err", err)
		return
	}

	log := s.srv.Logger.With("service", "game", "peer", from.String(),
		"token", hex.EncodeToString(cs.token[:]))
	if s.srv.Config.DebugRaw {
		log.Info("game raw recv", "bytes", len(pt), "connection_prefix", prefix)
		log.Info("hexdump\n" + hex.Dump(pt))
	}

	s.mu.Lock()
	cs.lastSeen = time.Now()
	cs.sess.Feed(pt)
	s.drain(log, cs)
	s.mu.Unlock()
}

// session returns the session for from/tok, creating it on first sight. The
// token must resolve in the registry, and an existing session's token must
// match, so a peer cannot take over another client's session.
func (s *Service) session(pc net.PacketConn, from net.Addr, tok authtoken.Token) (*clientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := from.String()
	if cs, ok := s.sessions[key]; ok {
		if cs.token != tok {
			return nil, fmt.Errorf("token mismatch for existing session")
		}
		return cs, nil
	}

	cwcKey, ok := s.srv.Tokens.Lookup(tok)
	if !ok {
		return nil, fmt.Errorf("unknown or expired auth token")
	}
	cipher, err := frpgcipher.NewServerUDPCipher(cwcKey)
	if err != nil {
		return nil, fmt.Errorf("build udp cipher: %w", err)
	}

	cs := &clientSession{
		token:    tok,
		cipher:   cipher,
		lastSeen: time.Now(),
	}
	cs.sess = rudp.NewServerSession(func(reliable []byte, connectionPrefix bool) error {
		sealed, err := cipher.Seal(reliable, connectionPrefix)
		if err != nil {
			return err
		}
		_, err = pc.WriteTo(sealed, from)
		return err
	})
	cs.conn = rudp.NewMessageConn(cs.sess)
	cs.lastState = cs.sess.State()
	s.sessions[key] = cs

	s.srv.Logger.Info("game session opened", "service", "game", "peer", key,
		"token", hex.EncodeToString(tok[:]))
	return cs, nil
}

// drain reads whatever the session has decoded and logs it. Caller holds s.mu.
func (s *Service) drain(log logger, cs *clientSession) {
	if st := cs.sess.State(); st != cs.lastState {
		log.Info("game session state", "from", cs.lastState.String(), "to", st.String())
		cs.lastState = st
	}
	for {
		msg, ok, err := cs.conn.Recv()
		if err != nil {
			log.Warn("game: message decode failed", "err", err)
			return
		}
		if !ok {
			return
		}
		log.Info("game message", "type", fmt.Sprintf("%#04x", msg.Type),
			"index", msg.Index, "payload_bytes", len(msg.Payload))
		if s.srv.Config.DebugRaw && len(msg.Payload) > 0 {
			log.Info("hexdump\n" + hex.Dump(msg.Payload))
		}

		reply, handled, err := s.handleMessage(log, cs, msg.Type, msg.Payload)
		if err != nil {
			log.Warn("game: message handler failed",
				"type", fmt.Sprintf("%#04x", msg.Type), "err", err)
			continue
		}
		if handled && reply == nil {
			// Deliberately answered with silence; the client expects no reply.
			continue
		}
		if reply == nil {
			// No handler yet. Staying silent is deliberate: the client tolerates
			// an unanswered message better than a malformed reply, and the
			// hexdump above is what the remaining handlers get built from.
			log.Info("game: no handler, not replying",
				"type", fmt.Sprintf("%#04x", msg.Type))
			continue
		}
		cs.conn.SendReply(msg, reply)
		log.Info("game reply sent", "type", fmt.Sprintf("%#04x", msg.Type),
			"payload_bytes", len(reply))
	}
}

// pumpLoop drives per-session timers (retransmits, heartbeats) and reaps idle
// sessions.
func (s *Service) pumpLoop(ctx context.Context) {
	t := time.NewTicker(pumpInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.pumpOnce()
		}
	}
}

func (s *Service) pumpOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for key, cs := range s.sessions {
		if now.Sub(cs.lastSeen) > sessionIdle {
			s.srv.Logger.Info("game session idle, dropping", "service", "game", "peer", key,
				"token", hex.EncodeToString(cs.token[:]))
			s.dropSession(key, cs)
			continue
		}
		if err := cs.sess.Pump(); err != nil {
			s.srv.Logger.Warn("game: session pump failed", "service", "game", "peer", key, "err", err)
			s.dropSession(key, cs)
			continue
		}
		if st := cs.sess.State(); st == rudp.StateClosed {
			s.srv.Logger.Info("game session closed", "service", "game", "peer", key)
			s.dropSession(key, cs)
		}
	}
}

// dropSession removes a session and cleans up anything that referenced it.
//
// A departing host's summon signs must go with them: otherwise the sign lingers
// in other players' worlds and summoning it fails in a way that looks like a
// server bug rather than a player who logged off. Caller holds s.mu.
func (s *Service) dropSession(key string, cs *clientSession) {
	delete(s.sessions, key)
	if cs.playerID != 0 {
		s.dropSignsForPlayer(s.srv.Logger, cs.playerID)
	}
}

// logger is the subset of *slog.Logger this package uses.
type logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
}
