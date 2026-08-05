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
// KNOWN GAP: each arena has three statues that pick the map, and nothing in
// RequestRegisterQuickMatch carries that choice — so either it rides in the
// opaque MatchingParameter, or map selection is negotiated client-to-client after
// the join.
const (
	opRequestRegisterQuickMatch   uint32 = 0x03D9
	opRequestUnregisterQuickMatch uint32 = 0x03DA
	opRequestUpdateQuickMatch     uint32 = 0x03DB
	opRequestSearchQuickMatch     uint32 = 0x03DC
	opRequestJoinQuickMatch       uint32 = 0x03DD
	opRequestRejectQuickMatch     uint32 = 0x03DE
)

// Push alias layout, CONFIRMED at the instruction level in both v1.00 and v1.10.
//
//	opcode = quickMatchPushBase + 2*role + mode      (mode-MINOR, unlike the others)
//
// Eight aliases are two per role, one per venue, dispatched through an 8-entry
// jump table. Note the operand order is reversed relative to BreakIn and Visitor:
// here the mode is the fast-moving index.
//
// mode is the QuickMatchGameMode: Blue (Cathedral of Blue) = 0, Brotherhood
// (Undead Purgatory) = 1.
//
// This replaces an earlier guess of a fixed 0x3E1/0x3E3/0x3E5/0x3E7 taken from
// the PC enum. Those are the four roles for mode 1 only — right for Undead
// Purgatory, silently wrong for the Cathedral of Blue.
const quickMatchPushBase = 0x03E0

const (
	quickMatchRoleJoin   = 0
	quickMatchRoleReject = 1
	quickMatchRoleAllow  = 2
	quickMatchRoleRemove = 3
)

// quickMatchPushIDFor returns the alias for a role at a venue.
func quickMatchPushIDFor(mode ds2pb.QuickMatchGameMode, role int) int32 {
	return int32(quickMatchPushBase + 2*role + int(mode))
}

// PushRequestAllowQuickMatch (role 2) is deliberately never sent. As with the
// invasion "allow", acceptance is built by the HOST's client and tunnelled back
// through RequestSendMessageToPlayers (0x0320) rather than originating here —
// that relay is what made invasions work at all.

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
// WHICH FIELD CARRIES THE STATUE IS UNCONFIRMED. cell_id is the only per-venue
// field the request has and is the obvious candidate — the schema's own samples
// pair area 10230000 with cell 102350 and area 10310000 with cell 103140 — but we
// have only ever observed one cell per venue live, because both test clients used
// the same statue. If a capture shows the cell fixed per venue, the statue is
// carried elsewhere (most likely inside the opaque MatchingParameter) and this
// ordering becomes a no-op rather than a wrong answer.
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

	return proto.Marshal(&ds2pb.RequestRegisterQuickMatchResponse{})
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
