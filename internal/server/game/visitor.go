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

// Push alias layout, CONFIRMED at the instruction level in both v1.00 and v1.10.
//
//	opcode = visitorPushBase + 3*mode + role
//
// The nine aliases are three per MODE, not three per message type: the manager is
// instantiated once per covenant, and each instance registers all three message
// types at its own triple. The shared callback re-derives the role from
// opcode-0x3C9 with the masks 0x049 / 0x092 / 0x124, which together cover all
// nine — so every alias is live.
//
// mode is the VisitorType: BlueSentinels=0, BellKeepers=1, Rat=2.
//
// This replaces an earlier guess of a fixed 0x3CF/0x3D0/0x3D1, which happened to
// be mode 2's triple — correct only for the Rat covenant, silently wrong for the
// other two.
const visitorPushBase = 0x03C9

const (
	visitorRoleVisit  = 0
	visitorRoleReject = 1
	visitorRoleRemove = 2
)

// visitorPushIDFor returns the alias for a role within a covenant.
// visitorModes is how many covenants the block covers: nine aliases over three
// roles. VisitorType_None (-1) and VisitorType_3 both fall outside it.
const visitorModes = 3

func visitorPushIDFor(mode ds2pb.VisitorType, role int) int32 {
	m := int(mode)
	if m < 0 || m >= visitorModes {
		// Clamp rather than compute out of the block.
		//
		// VisitorType_None is -1 and VisitorType_3 is 3, and the block is only
		// nine aliases wide — so type 3 would produce 0x3D2..0x3D4, past the end.
		// That is not merely a dead id: 0x3D2 is RequestGetBreakInTargetList's
		// opcode, so a push sent there would collide with an unrelated message
		// rather than being quietly dropped.
		//
		// Only types 0-2 (BlueSentinels, BellKeepers, Rat) have ever been seen on
		// the wire. If a type 3 visit ever appears this will send a mode-0 push,
		// which is wrong but harmless, and the log line below is the signal to
		// come back and work out what the fourth covenant actually is.
		m = 0
	}
	return int32(visitorPushBase + 3*m + role)
}

// visitorModeInRange reports whether a VisitorType maps onto the alias block.
func visitorModeInRange(mode ds2pb.VisitorType) bool {
	return int(mode) >= 0 && int(mode) < visitorModes
}

// handleGetVisitorList offers candidate worlds to visit.
//
// As with break-in targets, every other online player is offered: the matchmaking
// filters (soul memory band, covenant, area) live in the player status blob,
// which nothing consumes yet. Correct for a small test server, obviously wrong
// for a busy one.
//
// This is more visible than it sounds. The covenant auto-summon is a CLIENT poll
// on a fixed ~20.5s timer (measured 20.4-20.7s across 12 consecutive rat polls),
// and it re-asks for as long as the crest is equipped. Because we filter on
// nothing, we hand back the same target every poll — so an ineligible target is
// re-offered indefinitely and the visitor sees a refusal every 20 seconds. On
// 2026-08-05 that produced 54 consecutive rejections before the target moved.
//
// The real server could not have known about the specific ineligibility below
// (bonfire state is transient and there is no evidence the status blob carries
// it), but with a populated world and soul-memory filtering it would have
// returned DIFFERENT candidates each poll, so the loop would never be visible.
// Two players plus no filter is the pathological case.
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

	if !visitorModeInRange(req.GetType()) {
		log.Warn("visit with a covenant outside the push-alias block; "+
			"falling back to mode 0 — capture this",
			"player_id", cs.playerID, "type", req.GetType())
	}

	target, live := s.sessionForPlayerLocked(targetID)
	if !live {
		log.Info("visit target is offline",
			"visitor_player_id", cs.playerID, "target_player_id", targetID)
		s.pushVisitRejected(log, cs, targetID, req.GetType())
		return proto.Marshal(&ds2pb.RequestVisitResponse{})
	}

	body, err := proto.Marshal(&ds2pb.PushRequestVisit{
		PushMessageId: ds2pb.PushMessageId(visitorPushIDFor(req.GetType(), visitorRoleVisit)).Enum(),
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
		"type", req.GetType(),
		"push_id", fmt.Sprintf("%#04x", visitorPushIDFor(req.GetType(), visitorRoleVisit)),
		"payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestVisitResponse{})
}

// handleRejectVisit is a host declining a visitor, relayed back to them.
//
// The reason (unknown_2) is CLIENT-AUTHORED and 2 is the only value ever seen —
// 54 of 55 visits in the project's history carried it. It does NOT mean the
// covenant or the target was wrong.
//
// PROVEN 2026-08-05: reason 2 means the target is not currently invadable, and
// resting at a bonfire is one such state. Two independent observations:
//
//   - 18:20:44 the same pair in the same direction was refused; 21s later at
//     18:21:05 the identical request was ACCEPTED. Same code, same push id, same
//     players — so this is transient target state, not identity.
//   - A rat summon was refused 15 consecutive times while the target sat at a
//     bonfire, then succeeded within one poll of the target walking away.
//
// So a reject is normal traffic, not an error, and must stay cheap: the client
// will re-ask every ~20.5s for as long as the crest is equipped.
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
		PushMessageId: ds2pb.PushMessageId(visitorPushIDFor(vType, visitorRoleReject)).Enum(),
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
		"visitor_player_id", visitor.playerID, "host_player_id", hostID,
		"type", vType, "push_id", fmt.Sprintf("%#04x", visitorPushIDFor(vType, visitorRoleReject)))
}
