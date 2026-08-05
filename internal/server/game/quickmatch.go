package game

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Quick match — DS2's duelling arenas. Matches are 1v1 only; "Undead Match" is
// DS3 terminology and does not apply here.
//
// QuickMatchGameMode names a VENUE, not a format: Blue (0) is the Cathedral of
// Blue and Brotherhood (1) is Undead Purgatory. That is why the schema's sample
// online_area_id values come in two distinct sets. Soul Memory is ignored for
// arena matchmaking, so area, cell and mode are the whole filter.
//
// Unlike every other mode, this one is advertised rather than brokered: a player
// registers themselves as available at an arena and stays registered until they
// withdraw or disconnect. Other players search that venue and join. So this file
// keeps state, where visitor.go and breakin.go do not.
//
// Map selection never crosses this server. Each arena has three statues that pick
// the map, and nothing the client sends us carries the choice: five captured
// RequestRegisterQuickMatch payloads from different statues are byte-identical,
// MatchingParameter included. The one client-to-client channel we relay
// (0x0320, see relay.go) carries only a NexusRevolution2 join handshake —
// signature, version, the sender's PSN id twice, a session id — with no map field.
// That library exchanges "session properties" directly peer-to-peer once the P2P
// session is up, which is where anything like a map choice lives.
// See docs/protocol-map-ps3.md §5.2.8.
const (
	opRequestRegisterQuickMatch   uint32 = 0x03D9
	opRequestUnregisterQuickMatch uint32 = 0x03DA
	opRequestUpdateQuickMatch     uint32 = 0x03DB
	opRequestSearchQuickMatch     uint32 = 0x03DC
	opRequestJoinQuickMatch       uint32 = 0x03DD
	opRequestRejectQuickMatch     uint32 = 0x03DE
)

// Push alias layout, CONFIRMED at the instruction level in v1.10 (and v1.00).
//
//	opcode = quickMatchPushBase + 2*role + (1 - mode)   // mode-MINOR *and* INVERTED
//
// Eight aliases are two per role, one per venue, dispatched through an 8-entry
// jump table whose entries pair up (0,1)(2,3)(4,5)(6,7) — so the receive side
// separates the ROLE only, never the venue.
//
// The venue parity is the reverse of the enum. The manager constructor
// (v1.10 `0x15DDEC0`) takes the mode in r5 and branches:
//
//	mode == 0 (Blue)        -> registers 0x3E1 0x3E3 0x3E5 0x3E7   (ODD)
//	mode == 1 (Brotherhood) -> registers 0x3E0 0x3E2 0x3E4 0x3E6   (EVEN)
//
// and the client's own relayed "allow" picks its opcode the same way
// (v1.10 `0x15DEAE0`: `lwz r0,48(this)`; mode==0 -> `li r0,0x3E5`,
// mode==1 -> `li r0,0x3E4`). A live BLUS41045 capture of a Brotherhood duel
// carries push_message_id 0x3E4 — a client-generated id, which settles it.
//
// The earlier `+ mode` form had the parity backwards. It still worked live
// because BOTH manager instances are constructed unconditionally at
// `0x15C25E0`/`0x15C25F4` (`li r5,0` then `li r5,1`), so every client has all
// eight aliases registered, both aliases of a role reach the same handler, and
// the venue travels in the message body (`PushRequestJoinQuickMatch.mode`)
// rather than in the opcode. Sending the venue-matched alias is the correct
// behaviour; sending the other one is merely tolerated.
const quickMatchPushBase = 0x03E0

const (
	quickMatchRoleJoin   = 0
	quickMatchRoleReject = 1
	quickMatchRoleAllow  = 2
	quickMatchRoleRemove = 3
)

// quickMatchPushIDFor returns the alias for a role at a venue.
func quickMatchPushIDFor(mode ds2pb.QuickMatchGameMode, role int) int32 {
	// Mirror the client exactly: only mode 1 takes the even parity, anything
	// else falls to the odd one (the binary's `cmpwi 1` / default path).
	parity := 1
	if mode == ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood {
		parity = 0
	}
	return int32(quickMatchPushBase + 2*role + parity)
}

// PushRequestAllowQuickMatch (role 2) is deliberately never sent. As with the
// invasion "allow", acceptance is built by the HOST's client and tunnelled back
// through RequestSendMessageToPlayers (0x0320) rather than originating here —
// that relay is what made invasions work at all.
//
// Those two allows are the ONLY things a client tunnels: scanning v1.10 .text for
// the relay opcode finds exactly two send sites, 0x15DF124 (this one) and
// 0x16040FC (PushRequestAllowBreakInTarget). Every other push block, Visitor
// included, is server-originated only.

// quickMatch is one player advertising themselves at an arena location.
type quickMatch struct {
	playerID uint32
	psnID    string
	areaID   int64
	cellID   int64
	mode     ds2pb.QuickMatchGameMode
	matching *ds2pb.MatchingParameter

	// awareOf is every player who has seen this entry in a search, so each can be
	// told when it goes away rather than discovering it on a failed join.
	awareOf map[uint32]bool
}

// quickMatchStore holds live arena registrations.
//
// Memory-only and deliberately so: a registration is meaningful only while its
// player is connected, and persisting one would advertise a player who is gone.
type quickMatchStore struct {
	mu sync.Mutex
	// byPlayer holds at most one registration per player. Re-registering replaces
	// rather than accumulating, which is what the client's own register/update
	// cycle implies.
	byPlayer map[uint32]*quickMatch
}

func newQuickMatchStore() *quickMatchStore {
	return &quickMatchStore{byPlayer: make(map[uint32]*quickMatch)}
}

func (q *quickMatchStore) put(m *quickMatch) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if existing, ok := q.byPlayer[m.playerID]; ok {
		// Carry the audience across a re-registration so a player who moves
		// between arenas still gets a removal push for the old location.
		m.awareOf = existing.awareOf
	} else {
		m.awareOf = make(map[uint32]bool)
	}
	q.byPlayer[m.playerID] = m
}

func (q *quickMatchStore) get(playerID uint32) (*quickMatch, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	m, ok := q.byPlayer[playerID]
	return m, ok
}

func (q *quickMatchStore) remove(playerID uint32) (*quickMatch, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	m, ok := q.byPlayer[playerID]
	if ok {
		delete(q.byPlayer, playerID)
	}
	return m, ok
}

// search returns opponents at a venue, preferring the caller's own map.
//
// Each arena has THREE STATUES, and praying at one is a vote for a different map
// — on the Brotherhood side a bridge over a lethal drop, a two-level labyrinth,
// and a circular scaffolded stage. The documented behaviour is that you are
// matched at your own map when someone is queued there, and paired across maps
// when nobody is, with the higher covenant rank deciding which map is used.
//
// So the match is on area and mode (the venue), with the cell ordered first
// rather than filtered on. A strict cell filter would leave two players queued at
// different statues waiting forever while each was visible to the other.
//
// CONFIRMED BY CAPTURE, in both directions: the statue never reaches the network.
// Five RequestRegisterQuickMatch payloads from different statues are byte-identical
// in every field including the whole MatchingParameter, and three relayed
// PushRequestAllowQuickMatch bodies from the same player at three different
// statues differ only in a trailing session counter. Map selection is negotiated
// peer-to-peer through NexusRevolution2 session-property packets.
//
// cell_id does NOT carry the statue either. Registering at the left and
// middle statues of Undead Purgatory both logged cell_id=102350, so the cell is
// fixed per venue and the map choice rides elsewhere — almost certainly inside the
// opaque MatchingParameter, which we store and hand over without interpreting.
//
// That makes this ordering a no-op in practice rather than a wrong answer, and it
// is kept because it costs nothing and is correct if the map ever does reach us.
// Map selection demonstrably works in game regardless, so whatever channel
// carries it does not need us.
//
// KNOWN ROUGH EDGE: if both players register and neither searches, both sit
// advertising and no match forms — observed live as a ~23-second stall that broke
// the moment one player re-queued. The server is passive here by design: it
// advertises and lets a client choose to join. Whether the real server actively
// paired two waiting registrations is unknown.
func (q *quickMatchStore) search(areaID, cellID int64, mode ds2pb.QuickMatchGameMode, exclude uint32, limit int) []*quickMatch {
	q.mu.Lock()
	defer q.mu.Unlock()

	var sameCell, otherCell []*quickMatch
	for _, m := range q.byPlayer {
		if m.playerID == exclude || m.areaID != areaID || m.mode != mode {
			continue
		}
		if m.cellID == cellID {
			sameCell = append(sameCell, m)
		} else {
			otherCell = append(otherCell, m)
		}
	}

	out := append(sameCell, otherCell...)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (q *quickMatchStore) markAware(playerID, viewerID uint32) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if m, ok := q.byPlayer[playerID]; ok {
		m.awareOf[viewerID] = true
	}
}

func (m *quickMatch) toProto() *ds2pb.QuickMatchData {
	return &ds2pb.QuickMatchData{
		PlayerId:          proto.Int64(int64(m.playerID)),
		OnlineAreaId:      proto.Int64(m.areaID),
		CellId:            proto.Int64(m.cellID),
		MatchingParameter: m.matching,
		PlayerPsnId:       proto.String(m.psnID),
		Mode:              m.mode.Enum(),
	}
}

func (s *Service) handleRegisterQuickMatch(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRegisterQuickMatch
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRegisterQuickMatch: %w", err)
	}

	s.quickMatch.put(&quickMatch{
		playerID: cs.playerID,
		psnID:    cs.accountID,
		areaID:   req.GetOnlineAreaId(),
		cellID:   req.GetCellId(),
		mode:     req.GetMode(),
		matching: req.GetMatchingParameter(),
	})

	log.Info("quick match registered",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cell_id", req.GetCellId(), "mode", req.GetMode())

	if s.srv.Config.QuickMatchAutoPair {
		s.autoPair(log, cs, req.GetOnlineAreaId(), req.GetCellId(), req.GetMode())
	}

	return proto.Marshal(&ds2pb.RequestRegisterQuickMatchResponse{})
}

// autoPair introduces a newly registered player to someone already waiting.
//
// Both have declared availability by registering; this only removes the need for
// one of them to happen to be in the searching half of its cycle at the right
// moment. The longest-waiting player is treated as the host, mirroring the
// natural flow where a searcher joins an advertiser.
//
// Caller holds s.mu.
func (s *Service) autoPair(log logger, joiner *clientSession, areaID, cellID int64, mode ds2pb.QuickMatchGameMode) {
	waiting := s.quickMatch.search(areaID, cellID, mode, joiner.playerID, 1)
	if len(waiting) == 0 {
		return
	}
	host, live := s.sessionForPlayerLocked(waiting[0].playerID)
	if !live {
		return
	}

	body, err := proto.Marshal(&ds2pb.PushRequestJoinQuickMatch{
		PushMessageId: ds2pb.PushMessageId(quickMatchPushIDFor(mode, quickMatchRoleJoin)).Enum(),
		PlayerId:      proto.Int64(int64(joiner.playerID)),
		PlayerPsnId:   proto.String(joiner.accountID),
		OnlineAreaId:  proto.Int64(areaID),
		CellId:        proto.Int64(cellID),
		Mode:          mode.Enum(),
	})
	if err != nil {
		log.Warn("auto-pair: failed to build PushRequestJoinQuickMatch", "err", err)
		return
	}
	host.conn.SendPush(body)
	log.Info("auto-paired two waiting arena players",
		"host_player_id", host.playerID, "joiner_player_id", joiner.playerID,
		"mode", mode, "push_id", fmt.Sprintf("%#04x", quickMatchPushIDFor(mode, quickMatchRoleJoin)))
}

func (s *Service) handleUnregisterQuickMatch(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUnregisterQuickMatch
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUnregisterQuickMatch: %w", err)
	}
	if m, ok := s.quickMatch.remove(cs.playerID); ok {
		log.Info("quick match unregistered",
			"player_id", cs.playerID, "mode", req.GetMode())
		s.pushQuickMatchRemoved(log, m)
	}
	return proto.Marshal(&ds2pb.RequestUnregisterQuickMatchResponse{})
}

// handleUpdateQuickMatch is the registration keepalive. Registrations live until
// withdrawn or the player disconnects, so nothing needs refreshing — but the
// opcode is request/response and must still be answered.
func (s *Service) handleUpdateQuickMatch(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdateQuickMatch
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdateQuickMatch: %w", err)
	}
	log.Debug("quick match keepalive",
		"player_id", cs.playerID, "mode", req.GetMode())
	return proto.Marshal(&ds2pb.RequestUpdateQuickMatchResponse{})
}

// handleSearchQuickMatch lists other players advertising at the same arena.
//
// Filtered by area, cell and mode — all three come from the request, so unlike
// the sign and invasion listings this one is genuinely selective without needing
// the status blob. Soul-level and covenant filtering from MatchingParameter is
// still not applied; that is the same matchmaking gap as everywhere else.
func (s *Service) handleSearchQuickMatch(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestSearchQuickMatch
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestSearchQuickMatch: %w", err)
	}

	// A search is a declaration that this player is still queuing, so re-assert
	// their advertisement.
	//
	// The client unregisters immediately BEFORE it searches, which leaves an
	// actively-looking player briefly invisible — if the other player searches in
	// that window they see nobody and give up. Re-asserting closes the race
	// without deciding anything: the searcher wanted a match either way, and the
	// other side still chooses whether to join.
	s.quickMatch.put(&quickMatch{
		playerID: cs.playerID,
		psnID:    cs.accountID,
		areaID:   req.GetOnlineAreaId(),
		cellID:   req.GetCellId(),
		mode:     req.GetMode(),
		matching: req.GetMatchingParameter(),
	})

	found := s.quickMatch.search(req.GetOnlineAreaId(), req.GetCellId(),
		req.GetMode(), cs.playerID, int(req.GetMaxResults()))

	matches := make([]*ds2pb.QuickMatchData, 0, len(found))
	for _, m := range found {
		// Seeing an entry makes the viewer aware of it, so they get the removal
		// push rather than discovering it on a failed join.
		s.quickMatch.markAware(m.playerID, cs.playerID)
		matches = append(matches, m.toProto())
	}

	// Log whether any result came from a different statue, since that is the
	// evidence needed to confirm what cell_id actually encodes.
	crossMap := 0
	for _, m := range found {
		if m.cellID != req.GetCellId() {
			crossMap++
		}
	}
	log.Info("quick match search",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cell_id", req.GetCellId(), "mode", req.GetMode(),
		"returned", len(matches), "cross_map", crossMap)

	return proto.Marshal(&ds2pb.RequestSearchQuickMatchResponse{Matches: matches})
}

// handleJoinQuickMatch tells a host that someone wants into their match.
func (s *Service) handleJoinQuickMatch(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestJoinQuickMatch
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestJoinQuickMatch: %w", err)
	}
	hostID := uint32(req.GetPlayerId())

	host, live := s.sessionForPlayerLocked(hostID)
	if !live {
		log.Info("quick match host is offline",
			"joiner_player_id", cs.playerID, "host_player_id", hostID)
		s.pushQuickMatchRejected(log, cs, hostID, req.GetOnlineAreaId(),
			req.GetCellId(), req.GetMode())
		return proto.Marshal(&ds2pb.RequestJoinQuickMatchResponse{})
	}

	body, err := proto.Marshal(&ds2pb.PushRequestJoinQuickMatch{
		PushMessageId: ds2pb.PushMessageId(quickMatchPushIDFor(req.GetMode(), quickMatchRoleJoin)).Enum(),
		PlayerId:      proto.Int64(int64(cs.playerID)),
		PlayerPsnId:   proto.String(cs.accountID),
		OnlineAreaId:  proto.Int64(req.GetOnlineAreaId()),
		CellId:        proto.Int64(req.GetCellId()),
		Mode:          req.GetMode().Enum(),
	})
	if err != nil {
		return nil, fmt.Errorf("build PushRequestJoinQuickMatch: %w", err)
	}
	host.conn.SendPush(body)

	log.Info("pushed quick match join to host",
		"joiner_player_id", cs.playerID, "host_player_id", hostID,
		"mode", req.GetMode(),
		"push_id", fmt.Sprintf("%#04x", quickMatchPushIDFor(req.GetMode(), quickMatchRoleJoin)),
		"payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestJoinQuickMatchResponse{})
}

// handleRejectQuickMatch relays a host's refusal back to the joiner.
func (s *Service) handleRejectQuickMatch(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRejectQuickMatch
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRejectQuickMatch: %w", err)
	}
	joinerID := uint32(req.GetPlayerId())

	if joiner, live := s.sessionForPlayerLocked(joinerID); live {
		s.pushQuickMatchRejected(log, joiner, cs.playerID,
			req.GetOnlineAreaId(), req.GetCellId(), req.GetMode())
	}
	log.Info("host rejected quick match",
		"host_player_id", cs.playerID, "joiner_player_id", joinerID)
	return proto.Marshal(&ds2pb.RequestRejectQuickMatchResponse{})
}

// pushQuickMatchRejected tells a joiner their attempt failed. Caller holds s.mu.
func (s *Service) pushQuickMatchRejected(log logger, joiner *clientSession, hostID uint32, areaID, cellID int64, mode ds2pb.QuickMatchGameMode) {
	body, err := proto.Marshal(&ds2pb.PushRequestRejectQuickMatch{
		PushMessageId: ds2pb.PushMessageId(quickMatchPushIDFor(mode, quickMatchRoleReject)).Enum(),
		PlayerId:      proto.Int64(int64(hostID)),
		PlayerPsnId:   proto.String(joiner.accountID),
		OnlineAreaId:  proto.Int64(areaID),
		CellId:        proto.Int64(cellID),
		Mode:          mode.Enum(),
		Unknown_7:     proto.Int64(0),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRejectQuickMatch", "err", err)
		return
	}
	joiner.conn.SendPush(body)
	log.Info("pushed quick match rejection",
		"joiner_player_id", joiner.playerID, "host_player_id", hostID)
}

// pushQuickMatchRemoved tells everyone who saw a registration that it is gone.
// Caller holds s.mu.
func (s *Service) pushQuickMatchRemoved(log logger, m *quickMatch) {
	body, err := proto.Marshal(&ds2pb.PushRequestRemoveQuickMatch{
		PushMessageId: ds2pb.PushMessageId(quickMatchPushIDFor(m.mode, quickMatchRoleRemove)).Enum(),
		PlayerId:      proto.Int64(int64(m.playerID)),
		OnlineAreaId:  proto.Int64(m.areaID),
		CellId:        proto.Int64(m.cellID),
		PlayerPsnId:   proto.String(m.psnID),
		Mode:          m.mode.Enum(),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRemoveQuickMatch", "err", err)
		return
	}
	sent := 0
	for playerID := range m.awareOf {
		if playerID == m.playerID {
			continue
		}
		if target, live := s.sessionForPlayerLocked(playerID); live {
			target.conn.SendPush(body)
			sent++
		}
	}
	if sent > 0 {
		log.Info("pushed quick match removal",
			"player_id", m.playerID, "recipients", sent)
	}
}

// dropQuickMatchForPlayer withdraws a departing player's registration.
//
// Without this the arena keeps advertising someone who has gone, and joining them
// fails in a way that looks like a server fault. Caller holds s.mu.
func (s *Service) dropQuickMatchForPlayer(log logger, playerID uint32) {
	if m, ok := s.quickMatch.remove(playerID); ok {
		log.Info("removing quick match for departed player", "player_id", playerID)
		s.pushQuickMatchRemoved(log, m)
	}
}
