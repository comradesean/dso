package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Mirror Knight opcodes.
//
// This is the Belfry Sol arena: players place a sign to be summoned as a phantom
// during the Mirror Knight boss fight. Mechanically it is the summon-sign system
// with one structural difference — there is no placement. RequestCreateMirrorKnightSign
// carries no online_area_id and no cell_id, because the arena is a single global
// location, so the listing cannot filter by position the way ordinary signs do.
//
// All six request opcodes and all three push ids are individually confirmed in
// the decompilation (docs/protocol-map-ps3.md §4.1), so unlike the Visitor and
// QuickMatch blocks there is no alias guesswork here. Note 0x03A3 is unused
// within the block — the sequence is 0x039E-0x03A2 then 0x03A4.
const (
	opRequestCreateMirrorKnightSign  uint32 = 0x039E
	opRequestUpdateMirrorKnightSign  uint32 = 0x039F
	opRequestRemoveMirrorKnightSign  uint32 = 0x03A0
	opRequestGetMirrorKnightSignList uint32 = 0x03A1
	opRequestSummonMirrorKnightSign  uint32 = 0x03A2
	opRequestRejectMirrorKnightSign  uint32 = 0x03A4
	opRequestNotifyMirrorKnight      uint32 = 0x03D8
)

// toMirrorKnightProto renders a Mirror Knight sign as SignData.
//
// SignData is shared with ordinary signs and its online_area_id, cell_id and
// sign_type are all `required`, so they must be present even though Mirror Knight
// has no placement — an unset required field produces a message the client's
// proto2 parser rejects outright. They are sent as zero, which is the only honest
// value available: the request never supplied them.
func (sg *sign) toMirrorKnightProto() *ds2pb.SignData {
	return &ds2pb.SignData{
		SignInfo: &ds2pb.SignInfo{
			PlayerId: proto.Uint32(sg.ownerID),
			SignId:   proto.Uint32(sg.id),
		},
		OnlineAreaId:      proto.Int64(0),
		MatchingParameter: sg.matching,
		PlayerStruct:      sg.player,
		PlayerPsnId:       proto.String(sg.psnID),
		CellId:            proto.Int64(0),
		SignType:          ds2pb.SignType(sg.signType).Enum(),
	}
}

func (s *Service) handleCreateMirrorKnightSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestCreateMirrorKnightSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestCreateMirrorKnightSign: %w", err)
	}

	sg := &sign{
		ownerID:  cs.playerID,
		owner:    cs,
		matching: req.GetMatchingParameter(),
		// The field is named `data` here rather than `player_struct`, but it is the
		// same opaque character blob, handed to the summoner verbatim.
		player: append([]byte(nil), req.GetData()...),
		psnID:  cs.accountID,
	}
	id := s.mirrorKnight.add(sg)

	log.Info("mirror knight sign created",
		"player_id", cs.playerID, "sign_id", id, "data_bytes", len(sg.player))

	return proto.Marshal(&ds2pb.RequestCreateMirrorKnightSignResponse{
		SignId: proto.Int64(int64(id)),
	})
}

// handleUpdateMirrorKnightSign is the host's keepalive, ignored like the ordinary
// sign one — signs live until removed or the host disconnects.
func (s *Service) handleUpdateMirrorKnightSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdateMirrorKnightSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdateMirrorKnightSign: %w", err)
	}
	log.Debug("mirror knight sign keepalive",
		"player_id", cs.playerID, "sign_id", req.GetSignId())
	return proto.Marshal(&ds2pb.RequestUpdateMirrorKnightSignResponse{})
}

func (s *Service) handleRemoveMirrorKnightSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRemoveMirrorKnightSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRemoveMirrorKnightSign: %w", err)
	}
	if sg, ok := s.mirrorKnight.remove(uint32(req.GetSignId())); ok {
		log.Info("mirror knight sign removed", "player_id", cs.playerID, "sign_id", sg.id)
		s.pushMirrorKnightSignRemoved(log, sg)
	}
	return proto.Marshal(&ds2pb.RequestRemoveMirrorKnightSignResponse{})
}

// handleGetMirrorKnightSignList lists arena signs.
//
// Unlike RequestGetSignListResponse there is no SignInfo field to return signs the
// client already holds, so every match is sent in full each time.
func (s *Service) handleGetMirrorKnightSignList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetMirrorKnightSignList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetMirrorKnightSignList: %w", err)
	}

	found := s.mirrorKnight.all(cs.playerID, int(req.GetMaxSigns()))
	datas := make([]*ds2pb.SignData, 0, len(found))
	for _, sg := range found {
		// Seeing a sign makes the viewer aware of it, so they get the removal push.
		s.mirrorKnight.markAware(sg.id, cs.playerID)
		datas = append(datas, sg.toMirrorKnightProto())
	}

	log.Info("mirror knight sign list",
		"player_id", cs.playerID, "max_signs", req.GetMaxSigns(), "returned", len(datas))

	return proto.Marshal(&ds2pb.RequestGetMirrorKnightSignListResponse{SignData: datas})
}

// handleSummonMirrorKnightSign is a summoner claiming an arena sign. As with
// ordinary signs the server only brokers: it tells the host, and the two clients
// establish the peer-to-peer session themselves.
func (s *Service) handleSummonMirrorKnightSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestSummonMirrorKnightSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestSummonMirrorKnightSign: %w", err)
	}
	signID := req.GetSignInfo().GetSignId()

	sg, claimed := s.mirrorKnight.claim(signID, cs.playerID)
	if sg == nil {
		log.Info("mirror knight summon rejected: sign no longer exists",
			"player_id", cs.playerID, "sign_id", signID)
		s.pushMirrorKnightReject(log, cs, 0, signID, ds2pb.SummonErrorId_SummonErrorId_SignHasDisappeared)
		return proto.Marshal(&ds2pb.RequestSummonMirrorKnightSignResponse{})
	}
	if !claimed {
		log.Info("mirror knight summon rejected: sign already claimed",
			"player_id", cs.playerID, "sign_id", signID, "claimed_by", sg.summonedBy)
		s.pushMirrorKnightReject(log, cs, sg.ownerID, signID, ds2pb.SummonErrorId_SummonErrorId_SignAlreadyUsed)
		return proto.Marshal(&ds2pb.RequestSummonMirrorKnightSignResponse{})
	}

	host, live := s.sessionForPlayerLocked(sg.ownerID)
	if !live {
		s.mirrorKnight.release(signID)
		log.Info("mirror knight summon rejected: host is offline",
			"player_id", cs.playerID, "sign_id", signID, "host_player_id", sg.ownerID)
		s.pushMirrorKnightReject(log, cs, sg.ownerID, signID, ds2pb.SummonErrorId_SummonErrorId_SignHasDisappeared)
		return proto.Marshal(&ds2pb.RequestSummonMirrorKnightSignResponse{})
	}

	body, err := proto.Marshal(&ds2pb.PushRequestSummonMirrorKnightSign{
		PushMessageId: ds2pb.PushMessageId_PushID_PushRequestSummonMirrorKnightSign.Enum(),
		PlayerId:      proto.Int64(int64(cs.playerID)),
		SignId:        proto.Int64(int64(signID)),
		PlayerStruct:  req.GetPlayerStruct(),
		PlayerPsnId:   proto.String(cs.accountID),
	})
	if err != nil {
		return nil, fmt.Errorf("build PushRequestSummonMirrorKnightSign: %w", err)
	}
	host.conn.SendPush(body)

	log.Info("pushed mirror knight summon to host",
		"summoner_player_id", cs.playerID, "host_player_id", sg.ownerID,
		"sign_id", signID, "payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestSummonMirrorKnightSignResponse{})
}

// handleRejectMirrorKnightSign is the host declining, relayed to the summoner.
func (s *Service) handleRejectMirrorKnightSign(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRejectMirrorKnightSign
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRejectMirrorKnightSign: %w", err)
	}
	signID := uint32(req.GetSignId())

	sg, ok := s.mirrorKnight.get(signID)
	if !ok {
		return proto.Marshal(&ds2pb.RequestRejectMirrorKnightSignResponse{})
	}
	summoner := sg.summonedBy
	// Free it so another player can try.
	s.mirrorKnight.release(signID)

	if summoner != 0 {
		if target, live := s.sessionForPlayerLocked(summoner); live {
			s.pushMirrorKnightReject(log, target, sg.ownerID, signID, req.GetError())
		}
	}
	log.Info("host rejected mirror knight summon",
		"host_player_id", cs.playerID, "sign_id", signID,
		"summoner_player_id", summoner, "error", req.GetError())
	return proto.Marshal(&ds2pb.RequestRejectMirrorKnightSignResponse{})
}

// handleNotifyMirrorKnight records the client reporting Mirror Knight activity.
// Fire-and-forget: no reply, and nothing consumes it yet.
func (s *Service) handleNotifyMirrorKnight(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyMirrorKnight
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyMirrorKnight: %w", err)
	}
	log.Info("mirror knight notification",
		"player_id", cs.playerID, "field_1", req.GetField_1())
	return nil, nil
}

// pushMirrorKnightReject tells a summoner their attempt failed. Caller holds s.mu.
func (s *Service) pushMirrorKnightReject(log logger, target *clientSession, ownerID, signID uint32, reason ds2pb.SummonErrorId) {
	body, err := proto.Marshal(&ds2pb.PushRequestRejectMirrorKnightSign{
		PushMessageId: ds2pb.PushMessageId_PushID_PushRequestRejectMirrorKnightSign.Enum(),
		SignInfo: &ds2pb.SignInfo{
			PlayerId: proto.Uint32(ownerID),
			SignId:   proto.Uint32(signID),
		},
		Error:       reason.Enum(),
		PlayerPsnId: proto.String(target.accountID),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRejectMirrorKnightSign", "err", err)
		return
	}
	target.conn.SendPush(body)
	log.Info("pushed mirror knight rejection",
		"target_player_id", target.playerID, "sign_id", signID, "error", reason)
}

// pushMirrorKnightSignRemoved tells everyone who had seen a sign that it is gone.
// Caller holds s.mu.
func (s *Service) pushMirrorKnightSignRemoved(log logger, sg *sign) {
	body, err := proto.Marshal(&ds2pb.PushRequestRemoveMirrorKnightSign{
		PushMessageId: ds2pb.PushMessageId_PushID_PushRequestRemoveMirrorKnightSign.Enum(),
		PlayerId:      proto.Int64(int64(sg.ownerID)),
		SignId:        proto.Int64(int64(sg.id)),
		PlayerPsnId:   proto.String(sg.psnID),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRemoveMirrorKnightSign", "err", err)
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
		log.Info("pushed mirror knight sign removal", "sign_id", sg.id, "recipients", sent)
	}
}

// dropMirrorKnightSignsForPlayer removes a departing player's arena signs. Same
// reasoning as dropSignsForPlayer: a lingering sign fails to summon in a way that
// looks like a server bug rather than a host who logged off. Caller holds s.mu.
func (s *Service) dropMirrorKnightSignsForPlayer(log logger, playerID uint32) {
	for _, sg := range s.mirrorKnight.removeOwnedBy(playerID) {
		log.Info("removing mirror knight sign for departed host",
			"sign_id", sg.id, "player_id", playerID)
		s.pushMirrorKnightSignRemoved(log, sg)
	}
}
