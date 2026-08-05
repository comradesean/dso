package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Visitor opcodes — the covenant auto-summon systems (Bell Keepers, Rat King,
// Blue Sentinels). A player registers as available and is pulled into someone
// else's world, rather than placing a sign and waiting.
//
// Mechanically this is the invasion flow, not the sign flow: there is no stored
// object to list, claim and remove. The server brokers a request between two live
// sessions and steps out, exactly as breakin.go does — which is why this file is
// stateless and reuses the same session lookup.
const (
	opRequestGetVisitorList uint32 = 0x03D5
	opRequestVisit          uint32 = 0x03D6
	opRequestRejectVisit    uint32 = 0x03D7
)

// Visitor push ids.
//
// The client registers NINE push handlers across 0x03C9-0x03D1 for only three
// push classes (PushRequestVisit @0x157C20C, PushRequestRejectVisit @0x157C6CC,
// PushRequestRemoveVisitor @0x157C658) — three aliases per message type. Static
// analysis could not say which alias maps to which type: every registration site
// loads the same callback vtable and the distinguishing state is passed through
// the callback object at runtime.
//
// These three are the best-supported choice available, and the situation is
// materially better than it was for BreakIn:
//
//   - The PC/SOTFS protos assign exactly these values to exactly these three
//     types, and unlike the BreakIn pushes (PC says 0x03FB-0x03FD, which appear
//     NOWHERE in this binary) the decompilation confirms all three DO exist here,
//     as the last group of the block.
//   - Three consecutive values landing on three different message types is
//     consistent with the aliases being interleaved rather than run in blocks.
//     That pattern is real in this client: the QuickMatch block is documented as
//     two interleaved groups of four, odds then evens.
//
// So this is an inference with positive evidence behind it, not a coin flip. It
// is still unverified in-game. If a visit silently does nothing, the alternative
// hypothesis is contiguous groups — try 0x03C9 / 0x03CC / 0x03CF, the first alias
// of each group, which is the shape that turned out to be right for BreakIn where
// 0x03B9 led its group.
const (
	pushVisitID       = 0x03CF
	pushRejectVisitID = 0x03D0
	// pushRemoveVisitorID is recorded but NOT sent: telling a host their visitor
	// is gone requires knowing which host a departing player was visiting, and no
	// visit session is tracked. Until that exists the phantom clears on the
	// clients' own session timeout. Wiring this up is the natural companion to a
	// visit-session concept, which QuickMatch will need anyway.
	pushRemoveVisitorID = 0x03D1
)

var _ = pushRemoveVisitorID // referenced above; kept for when visits are tracked

// handleGetVisitorList offers candidate worlds to visit.
//
// As with break-in targets, every other online player is offered: the matchmaking
// filters (soul memory band, covenant, area) live in the player status blob,
// which nothing consumes yet. Correct for a small test server, obviously wrong
// for a busy one.
func (s *Service) handleGetVisitorList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetVisitorList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetVisitorList: %w", err)
	}

	var targets []*ds2pb.VisitorData
	max := int(req.GetMaxTargets())
	for _, other := range s.sessions {
		if other.playerID == 0 || other.playerID == cs.playerID {
			continue
		}
		targets = append(targets, &ds2pb.VisitorData{
			PlayerId:    proto.Int64(int64(other.playerID)),
			PlayerPsnId: proto.String(other.accountID),
		})
		if max > 0 && len(targets) >= max {
			break
		}
	}

	log.Info("visitor target list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cell_id", req.GetCellId(), "type", req.GetType(),
		"max", req.GetMaxTargets(), "returned", len(targets))

	// online_area_id and cell_id are `required` in the response and are echoed
	// back from the request — the client matches the reply to the area it asked
	// about.
	return proto.Marshal(&ds2pb.RequestGetVisitorListResponse{
		OnlineAreaId: proto.Int64(req.GetOnlineAreaId()),
		CellId:       proto.Int64(req.GetCellId()),
		TargetData:   targets,
	})
}

// handleVisit is a visitor asking to enter a host's world. The server notifies
// the host; the two clients negotiate the session themselves from there.
func (s *Service) handleVisit(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestVisit
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestVisit: %w", err)
	}
	targetID := uint32(req.GetPlayerId())

	target, live := s.sessionForPlayerLocked(targetID)
	if !live {
		log.Info("visit target is offline",
			"visitor_player_id", cs.playerID, "target_player_id", targetID)
		s.pushVisitRejected(log, cs, targetID, req.GetType())
		return proto.Marshal(&ds2pb.RequestVisitResponse{})
	}

	body, err := proto.Marshal(&ds2pb.PushRequestVisit{
		PushMessageId: ds2pb.PushMessageId(pushVisitID).Enum(),
		PlayerId:      proto.Int64(int64(cs.playerID)),
		PlayerPsnId:   proto.String(cs.accountID),
		PlayerStruct:  req.GetPlayerStruct(),
		Type:          req.GetType().Enum(),
		OnlineAreaId:  proto.Int64(req.GetOnlineAreaId()),
		CellId:        proto.Int64(req.GetCellId()),
	})
	if err != nil {
		return nil, fmt.Errorf("build PushRequestVisit: %w", err)
	}
	target.conn.SendPush(body)

	log.Info("pushed visit to host",
		"visitor_player_id", cs.playerID, "target_player_id", targetID,
		"type", req.GetType(), "push_id", fmt.Sprintf("%#04x", pushVisitID),
		"payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestVisitResponse{})
}

// handleRejectVisit is a host declining a visitor, relayed back to them.
func (s *Service) handleRejectVisit(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRejectVisit
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRejectVisit: %w", err)
	}
	visitorID := uint32(req.GetPlayerId())

	if visitor, live := s.sessionForPlayerLocked(visitorID); live {
		s.pushVisitRejected(log, visitor, cs.playerID, req.GetType())
	}
	log.Info("host rejected visit",
		"host_player_id", cs.playerID, "visitor_player_id", visitorID,
		"reason", req.GetUnknown_2())
	return proto.Marshal(&ds2pb.RequestRejectVisitResponse{})
}

// pushVisitRejected tells a visitor their attempt failed. Caller holds s.mu.
func (s *Service) pushVisitRejected(log logger, visitor *clientSession, hostID uint32, vType ds2pb.VisitorType) {
	body, err := proto.Marshal(&ds2pb.PushRequestRejectVisit{
		PushMessageId: ds2pb.PushMessageId(pushRejectVisitID).Enum(),
		PlayerId:      proto.Int64(int64(hostID)),
		Unknown_3:     proto.Int64(0),
		PsnId:         proto.String(visitor.accountID),
		Type:          vType.Enum(),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRejectVisit", "err", err)
		return
	}
	visitor.conn.SendPush(body)
	log.Info("pushed visit rejection",
		"visitor_player_id", visitor.playerID, "host_player_id", hostID)
}
