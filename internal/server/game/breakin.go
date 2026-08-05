package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Invasion (break-in) opcodes. The request/response side is unambiguous; the
// push id is not — see breakInPushID below.
const (
	opRequestGetBreakInTargetList uint32 = 0x03D2
	opRequestBreakInTarget        uint32 = 0x03D3
	opRequestRejectBreakInTarget  uint32 = 0x03D4
)

// breakInPushID is the PushMessageId used for PushRequestBreakInTarget, and it
// is a GUESS among sixteen candidates.
//
// The PC value from the protos is 1019 (0x3FB), which is useless here:
// decompilation found no code for 0x3FB, 0x3FC or 0x3FD anywhere in the PS3
// client. Instead the BreakIn subsystem registers SIXTEEN push handlers spanning
// 0x03B9-0x03C8 — four message types with four aliases each — and static analysis
// could not say which alias maps to which type, because every registration site
// loads the same callback vtable and the distinguishing state is passed through
// the callback object at runtime.
//
// Registration order from the disassembly, in groups of four:
//
//	group 1: 0x3BD 0x3BE 0x3C0 0x3BF
//	group 2: 0x3C1 0x3C2 0x3C4 0x3C3
//	group 3: 0x3B9 0x3BA 0x3BC 0x3BB
//	group 4: 0x3C5 0x3C6 0x3C8 0x3C7
//
// If an invasion prompt never reaches the target, this constant is the first
// thing to change — work through the group leaders (0x3BD, 0x3C1, 0x3B9, 0x3C5)
// before trying the rest. One successful invasion identifies the group and
// collapses sixteen unknowns at once, which is why it is worth testing directly
// rather than reasoning further.
const breakInPushID = 0x03B9

func (s *Service) handleGetBreakInTargetList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetBreakInTargetList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetBreakInTargetList: %w", err)
	}

	// Candidate hosts are other live players. Matchmaking filters (soul memory
	// band, area, covenant) come from the status blob, which nothing consumes
	// yet — so for now every other online player is offered, which is the right
	// behaviour for a two-player test server and obviously not for a busy one.
	var targets []*ds2pb.BreakInTargetData
	max := int(req.GetMaxTargets())
	for _, other := range s.sessions {
		if other.playerID == 0 || other.playerID == cs.playerID {
			continue
		}
		targets = append(targets, &ds2pb.BreakInTargetData{
			PlayerId: proto.Uint32(other.playerID),
			PsnId:    proto.String(other.accountID),
		})
		if max > 0 && len(targets) >= max {
			break
		}
	}

	log.Info("break-in target list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cell_id", req.GetCellId(), "type", req.GetType(),
		"max", req.GetMaxTargets(), "returned", len(targets))

	return proto.Marshal(&ds2pb.RequestGetBreakInTargetListResponse{
		OnlineAreaId: proto.Uint32(req.GetOnlineAreaId()),
		CellId:       proto.Uint32(req.GetCellId()),
		TargetData:   targets,
	})
}

// handleBreakInTarget is an invader picking a host. The server notifies the host
// and steps out; the two clients negotiate the session themselves.
func (s *Service) handleBreakInTarget(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestBreakInTarget
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestBreakInTarget: %w", err)
	}

	target, live := s.sessionForPlayerLocked(req.GetPlayerId())
	if !live {
		log.Info("break-in target is offline",
			"invader_player_id", cs.playerID, "target_player_id", req.GetPlayerId())
		s.pushBreakInRejected(log, cs, req.GetPlayerId())
		return proto.Marshal(&ds2pb.RequestBreakInTargetResponse{})
	}

	push := &ds2pb.PushRequestBreakInTarget{
		PushMessageId: ds2pb.PushMessageId(breakInPushID).Enum(),
		PlayerId:      proto.Uint32(cs.playerID),
		PsnId:         proto.String(cs.accountID),
		Type:          req.GetType().Enum(),
		OnlineAreaId:  proto.Uint32(req.GetOnlineAreaId()),
		CellId:        proto.Uint32(req.GetCellId()),
	}
	body, err := proto.Marshal(push)
	if err != nil {
		return nil, fmt.Errorf("build PushRequestBreakInTarget: %w", err)
	}
	target.conn.SendPush(body)

	log.Info("pushed break-in to target",
		"invader_player_id", cs.playerID, "target_player_id", req.GetPlayerId(),
		"push_id", fmt.Sprintf("%#04x", breakInPushID), "payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestBreakInTargetResponse{})
}

// handleRejectBreakInTarget is a host declining an invasion, relayed back.
func (s *Service) handleRejectBreakInTarget(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRejectBreakInTarget
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRejectBreakInTarget: %w", err)
	}
	invaderID := uint32(req.GetPlayerId())
	if invader, live := s.sessionForPlayerLocked(invaderID); live {
		s.pushBreakInRejected(log, invader, cs.playerID)
	}
	log.Info("host rejected break-in",
		"host_player_id", cs.playerID, "invader_player_id", invaderID)
	return proto.Marshal(&ds2pb.RequestRejectBreakInTargetResponse{})
}

// pushBreakInRejected tells an invader their attempt failed. Caller holds s.mu.
//
// Uses the alias immediately after breakInPushID on the assumption that the four
// message types occupy consecutive groups; like breakInPushID itself this is
// unverified.
func (s *Service) pushBreakInRejected(log logger, invader *clientSession, hostID uint32) {
	body, err := proto.Marshal(&ds2pb.PushRequestRejectBreakInTarget{
		PushMessageId: ds2pb.PushMessageId(breakInPushID + 1).Enum(),
		PlayerId:      proto.Int64(int64(hostID)),
		Unknown_3:     proto.Int64(0),
		PsnId:         proto.String(invader.accountID),
		Unknown_5:     proto.Int64(0),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRejectBreakInTarget", "err", err)
		return
	}
	invader.conn.SendPush(body)
	log.Info("pushed break-in rejection",
		"invader_player_id", invader.playerID, "host_player_id", hostID)
}
