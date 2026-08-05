package game

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Summon sign opcodes. Decomp-confirmed for PS3, and unlike the BreakIn pushes
// these are plain single opcodes with no alias blocks, so they can be implemented
// without a live capture to disambiguate.
//
// Note there is deliberately no RequestGetRightMatchingArea (0x03FA) here: that
// opcode has no code at all in the PS3 client. It is a PC/SOTFS addition.
const (
	opRequestCreateSign  uint32 = 0x0394
	opRequestUpdateSign  uint32 = 0x0395
	opRequestRemoveSign  uint32 = 0x0396
	opRequestGetSignList uint32 = 0x0397
	opRequestSummonSign  uint32 = 0x0398
	opRequestRejectSign  uint32 = 0x039A
)

// firstSignID keeps sign ids clear of the low numbers, for the same reason
// message ids start high: the client keys cached state by server-assigned id, and
// reusing one makes it apply stale state to a new sign.
const firstSignID = 100000

// firstMirrorKnightSignID keeps Mirror Knight sign ids in a disjoint range from
// ordinary summon signs. Both live in their own store, so both would otherwise
// start at firstSignID and hand the same id to two different signs — and the
// client keys cached state by sign id without regard for which system produced
// it.
const firstMirrorKnightSignID = 500000

// sign is one placed summon sign.
//
// playerStruct is the game's own opaque blob describing the host character. It is
// stored and handed to the summoner verbatim; the server never interprets it.
type sign struct {
	id       uint32
	ownerID  uint32 // player id of the host who placed it
	owner    *clientSession
	areaID   uint32
	cellID   uint32
	signType uint32
	matching *ds2pb.MatchingParameter
	player   []byte
	psnID    string

	// summonedBy is the player currently attempting to summon through this sign.
	// A sign can only be claimed once at a time; a second summoner is rejected
	// with SignAlreadyUsed rather than both being let through.
	summonedBy uint32

	// awareOf is every player who has seen this sign in a listing. They each get
	// a removal push when it goes away, so the sign disappears from their world
	// rather than lingering until their next poll.
	awareOf map[uint32]bool
}

// signStore holds live signs in memory.
//
// Signs are inherently ephemeral — they vanish when the host is summoned, removes
// them, or disconnects — so there is nothing worth persisting. The reference keeps
// them in memory too.
type signStore struct {
	mu     sync.Mutex
	nextID uint32
	byID   map[uint32]*sign
}

func newSignStore() *signStore { return newSignStoreFrom(firstSignID) }

func newSignStoreFrom(firstID uint32) *signStore {
	return &signStore{nextID: firstID - 1, byID: make(map[uint32]*sign)}
}

// all returns up to limit signs, ignoring area and cell.
//
// Mirror Knight has no placement: the sign is for a single global arena, and
// RequestCreateMirrorKnightSign carries no area or cell at all. So its listing
// cannot filter the way inCells does.
func (s *signStore) all(excludePlayer uint32, limit int) []*sign {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*sign
	for _, sg := range s.byID {
		if sg.ownerID == excludePlayer {
			continue
		}
		out = append(out, sg)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *signStore) add(sg *sign) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	sg.id = s.nextID
	sg.awareOf = make(map[uint32]bool)
	s.byID[sg.id] = sg
	return sg.id
}

func (s *signStore) get(id uint32) (*sign, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sg, ok := s.byID[id]
	return sg, ok
}

func (s *signStore) remove(id uint32) (*sign, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sg, ok := s.byID[id]
	if ok {
		delete(s.byID, id)
	}
	return sg, ok
}

// removeOwnedBy drops every sign a player placed, returning them so the removal
// can be pushed to whoever had seen them. Used when a host disconnects.
func (s *signStore) removeOwnedBy(playerID uint32) []*sign {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*sign
	for id, sg := range s.byID {
		if sg.ownerID == playerID {
			out = append(out, sg)
			delete(s.byID, id)
		}
	}
	return out
}

// inCells returns signs in the area restricted to the requested cells, excluding
// the requester's own signs — a host must not be offered their own sign.
func (s *signStore) inCells(areaID uint32, cells map[uint32]bool, excludePlayer uint32, limit int) []*sign {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*sign
	for _, sg := range s.byID {
		if sg.areaID != areaID || sg.ownerID == excludePlayer {
			continue
		}
		if len(cells) > 0 && !cells[sg.cellID] {
			continue
		}
		out = append(out, sg)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// markAware records that a player has seen a sign, so they can be told when it
// disappears.
func (s *signStore) markAware(id, playerID uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sg, ok := s.byID[id]; ok {
		sg.awareOf[playerID] = true
	}
}

// claim marks a sign as being summoned through. It returns false if someone else
// already claimed it.
func (s *signStore) claim(id, byPlayer uint32) (*sign, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sg, ok := s.byID[id]
	if !ok {
		return nil, false
	}
	if sg.summonedBy != 0 && sg.summonedBy != byPlayer {
		return sg, false
	}
	sg.summonedBy = byPlayer
	return sg, true
}

// release clears a claim, so the sign can be summoned again after a rejection.
func (s *signStore) release(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sg, ok := s.byID[id]; ok {
		sg.summonedBy = 0
	}
}

func (sg *sign) toProto() *ds2pb.SignData {
	return &ds2pb.SignData{
		SignInfo: &ds2pb.SignInfo{
			PlayerId: proto.Uint32(sg.ownerID),
			SignId:   proto.Uint32(sg.id),
		},
		OnlineAreaId:      proto.Int64(int64(sg.areaID)),
		MatchingParameter: sg.matching,
		PlayerStruct:      sg.player,
		PlayerPsnId:       proto.String(sg.psnID),
		CellId:            proto.Int64(int64(sg.cellID)),
		SignType:          ds2pb.SignType(sg.signType).Enum(),
	}
}

func (s *Service) handleCreateSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestCreateSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestCreateSign: %w", err)
	}

	sg := &sign{
		ownerID:  cs.playerID,
		owner:    cs,
		areaID:   req.GetOnlineAreaId(),
		cellID:   req.GetCellId(),
		signType: req.GetSignType(),
		matching: req.GetMatchingParameter(),
		player:   append([]byte(nil), req.GetPlayerStruct()...),
		psnID:    cs.accountID,
	}
	id := s.signs.add(sg)

	log.Info("summon sign created",
		"player_id", cs.playerID, "sign_id", id, "area_id", sg.areaID,
		"cell_id", sg.cellID, "sign_type", sg.signType, "player_struct_bytes", len(sg.player))

	return proto.Marshal(&ds2pb.RequestCreateSignResponse{SignId: proto.Uint32(id)})
}

// handleUpdateSign is the host's keepalive. The reference ignores it entirely and
// so do we — signs live until explicitly removed or the host disconnects.
func (s *Service) handleUpdateSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdateSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdateSign: %w", err)
	}
	log.Debug("sign keepalive", "player_id", cs.playerID, "sign_id", req.GetSignId())
	return proto.Marshal(&ds2pb.RequestUpdateSignResponse{})
}

func (s *Service) handleRemoveSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRemoveSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRemoveSign: %w", err)
	}
	if sg, ok := s.signs.remove(req.GetSignId()); ok {
		log.Info("summon sign removed", "player_id", cs.playerID, "sign_id", sg.id)
		s.pushSignRemoved(log, sg)
	}
	return proto.Marshal(&ds2pb.RequestRemoveSignResponse{})
}

func (s *Service) handleGetSignList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetSignList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetSignList: %w", err)
	}
	cs.areaID = req.GetOnlineAreaId()

	// The client tells us which signs it already holds, per cell. Those come back
	// as bare SignInfo; anything new comes back as full SignData. Sending the full
	// body for a sign the client already has would be wasted bandwidth.
	cells := make(map[uint32]bool, len(req.GetSearchAreas()))
	known := make(map[uint32]bool)
	for _, a := range req.GetSearchAreas() {
		cells[a.GetCellId()] = true
		for _, si := range a.GetLocalSigns() {
			known[si.GetSignId()] = true
		}
	}

	found := s.signs.inCells(req.GetOnlineAreaId(), cells, cs.playerID, int(req.GetMaxSigns()))

	var infos []*ds2pb.SignInfo
	var datas []*ds2pb.SignData
	for _, sg := range found {
		// Requesting a sign makes the requester aware of it, so they get told
		// when it goes away.
		s.signs.markAware(sg.id, cs.playerID)
		if known[sg.id] {
			infos = append(infos, &ds2pb.SignInfo{
				PlayerId: proto.Uint32(sg.ownerID),
				SignId:   proto.Uint32(sg.id),
			})
			continue
		}
		datas = append(datas, sg.toProto())
	}

	log.Info("sign list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cells_requested", len(cells), "already_known", len(infos), "new", len(datas))

	return proto.Marshal(&ds2pb.RequestGetSignListResponse{
		SignInfo: infos,
		SignData: datas,
	})
}

// handleSummonSign is a summoner claiming a sign. The server tells the host, and
// the two clients establish the peer-to-peer session themselves from there — the
// server's involvement ends with this push.
func (s *Service) handleSummonSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestSummonSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestSummonSign: %w", err)
	}
	signID := req.GetSignInfo().GetSignId()

	sg, claimed := s.signs.claim(signID, cs.playerID)
	if sg == nil {
		// The sign is gone — the host removed it or disconnected between the
		// listing and the summon.
		log.Info("summon rejected: sign no longer exists",
			"player_id", cs.playerID, "sign_id", signID)
		s.pushSummonRejected(log, cs, signID, 0, ds2pb.SummonErrorId_SummonErrorId_SignHasDisappeared)
		return proto.Marshal(&ds2pb.RequestSummonSignResponse{})
	}
	if !claimed {
		log.Info("summon rejected: sign already claimed",
			"player_id", cs.playerID, "sign_id", signID, "claimed_by", sg.summonedBy)
		s.pushSummonRejected(log, cs, signID, sg.ownerID, ds2pb.SummonErrorId_SummonErrorId_SignAlreadyUsed)
		return proto.Marshal(&ds2pb.RequestSummonSignResponse{})
	}

	host, live := s.sessionForPlayerLocked(sg.ownerID)
	if !live {
		s.signs.release(signID)
		log.Info("summon rejected: host is offline",
			"player_id", cs.playerID, "sign_id", signID, "host_player_id", sg.ownerID)
		s.pushSummonRejected(log, cs, signID, sg.ownerID, ds2pb.SummonErrorId_SummonErrorId_SignHasDisappeared)
		return proto.Marshal(&ds2pb.RequestSummonSignResponse{})
	}

	push := &ds2pb.PushRequestSummonSign{
		PushMessageId: ds2pb.PushMessageId_PushID_PushRequestSummonSign.Enum(),
		PlayerId:      proto.Int64(int64(cs.playerID)),
		SignId:        proto.Int64(int64(signID)),
		PlayerStruct:  req.GetPlayerStruct(),
		PlayerPsnId:   proto.String(cs.accountID),
	}
	body, err := proto.Marshal(push)
	if err != nil {
		return nil, fmt.Errorf("build PushRequestSummonSign: %w", err)
	}
	host.conn.SendPush(body)

	log.Info("pushed summon to host",
		"summoner_player_id", cs.playerID, "host_player_id", sg.ownerID,
		"sign_id", signID, "payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestSummonSignResponse{})
}

// handleRejectSign is the host declining a summon, relayed to the summoner.
func (s *Service) handleRejectSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRejectSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRejectSign: %w", err)
	}
	signID := uint32(req.GetSignId())

	sg, ok := s.signs.get(signID)
	if !ok {
		return proto.Marshal(&ds2pb.RequestRejectSignResponse{})
	}
	summoner := sg.summonedBy
	// Free the sign so someone else can try.
	s.signs.release(signID)

	if summoner != 0 {
		if target, live := s.sessionForPlayerLocked(summoner); live {
			s.pushRejectTo(log, target, sg.ownerID, signID, req.GetError())
		}
	}
	log.Info("host rejected summon",
		"host_player_id", cs.playerID, "sign_id", signID,
		"summoner_player_id", summoner, "error", req.GetError())
	return proto.Marshal(&ds2pb.RequestRejectSignResponse{})
}

// pushSummonRejected tells a summoner their attempt failed. Caller holds s.mu.
func (s *Service) pushSummonRejected(log logger, summoner *clientSession, signID, ownerID uint32, reason ds2pb.SummonErrorId) {
	s.pushRejectTo(log, summoner, ownerID, signID, reason)
}

func (s *Service) pushRejectTo(log logger, target *clientSession, ownerID, signID uint32, reason ds2pb.SummonErrorId) {
	push := &ds2pb.PushRequestRejectSign{
		PushMessageId: ds2pb.PushMessageId_PushID_PushRequestRejectSign.Enum(),
		SignInfo: &ds2pb.SignInfo{
			PlayerId: proto.Uint32(ownerID),
			SignId:   proto.Uint32(signID),
		},
		Error:       reason.Enum(),
		PlayerPsnId: proto.String(target.accountID),
	}
	body, err := proto.Marshal(push)
	if err != nil {
		log.Warn("failed to build PushRequestRejectSign", "err", err)
		return
	}
	target.conn.SendPush(body)
	log.Info("pushed sign rejection",
		"target_player_id", target.playerID, "sign_id", signID, "error", reason)
}

// pushSignRemoved tells everyone who had seen a sign that it is gone, so it
// disappears from their world immediately rather than at their next poll.
// Caller holds s.mu.
func (s *Service) pushSignRemoved(log logger, sg *sign) {
	body, err := proto.Marshal(&ds2pb.PushRequestRemoveSign{
		PushMessageId: ds2pb.PushMessageId_PushID_PushRequestRemoveSign.Enum(),
		PlayerId:      proto.Int64(int64(sg.ownerID)),
		SignId:        proto.Int64(int64(sg.id)),
		PlayerPsnId:   proto.String(sg.psnID),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRemoveSign", "err", err)
		return
	}
	sent := 0
	for playerID := range sg.awareOf {
		if playerID == sg.ownerID {
			continue
		}
		if target, live := s.sessionForPlayerLocked(playerID); live {
			target.conn.SendPush(body)
			sent++
		}
	}
	if sent > 0 {
		log.Info("pushed sign removal", "sign_id", sg.id, "recipients", sent)
	}
}

// dropSignsForPlayer removes a disconnecting player's signs and tells anyone who
// had seen them. Without this a host's sign lingers in other players' worlds and
// summoning it fails confusingly. Caller holds s.mu.
func (s *Service) dropSignsForPlayer(log logger, playerID uint32) {
	for _, sg := range s.signs.removeOwnedBy(playerID) {
		log.Info("removing sign for departed host", "sign_id", sg.id, "player_id", playerID)
		s.pushSignRemoved(log, sg)
	}
}
