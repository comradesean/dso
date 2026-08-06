package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Invasion (break-in) opcodes.
const (
	opRequestGetBreakInTargetList uint32 = 0x03D2
	opRequestBreakInTarget        uint32 = 0x03D3
	opRequestRejectBreakInTarget  uint32 = 0x03D4
)

// CONFIRMED 2026-08-05: 0x03B9 is PushRequestBreakInTarget for mode 0 (Red Eye
// Orb). A real invader selected a target, the server pushed with this id, and the
// target's client tunnelled its "allow" back through RequestSendMessageToPlayers
// — which it would not have done had the push gone unrecognised.
//
// The PC value from the protos is 0x3FB, useless here: no code exists for 0x3FB,
// 0x3FC or 0x3FD anywhere in this client.

func (s *Service) handleGetBreakInTargetList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetBreakInTargetList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetBreakInTargetList: %w", err)
	}
	cs.areaID = req.GetOnlineAreaId()

	var targets []*ds2pb.BreakInTargetData
	max := int(req.GetMaxTargets())
	filtering := matchmakingEnabled()
	invaderSM := req.GetMatchingParameter().GetSoulMemory()
	cs.profile.applyMatchingParameter(req.GetMatchingParameter())

	var skippedNotInvadable, skippedLocation, skippedSoul int
	for _, other := range s.sessions {
		if other.playerID == 0 || other.playerID == cs.playerID {
			continue
		}
		if filtering {
			// Bonfire, burnt effigy, or nowhere online.
			if !other.profile.isInvadable() {
				skippedNotInvadable++
				continue
			}
			// The host must be in the place being asked about.
			//
			// This is what broke Dark Chasm of Old invasions. A Cracked Red Eye
			// Orb cycles the three chasms, so the client asks about cells the
			// invader is NOT standing in — and with no location filter we
			// answered every one of them with a host who was somewhere else. The
			// host's client then received an invasion tagged for a chasm it was
			// not in and refused inside 120ms, which the invader reads as
			// "unable to find a world to invade".
			//
			// online_activity_area_id and the request's cell_id share an id
			// space: both are the 6-digit mmmmbb form, and the chasm cells
			// 400310/400320/400330 appeared in both during the same session.
			if other.profile.onlineActivityArea != uint32(req.GetCellId()) {
				skippedLocation++
				continue
			}
			if !soulMemoryMatches(invaderSM, other.profile.effectiveSoulMemory(),
				breakInTierWindow) {
				skippedSoul++
				continue
			}
		}
		targets = append(targets, &ds2pb.BreakInTargetData{
			PlayerId: proto.Uint32(other.playerID),
			PsnId:    proto.String(other.accountID),
		})
		if max > 0 && len(targets) >= max {
			break
		}
	}

	// With the rejection harness on, offer a synthetic target when nothing real is
	// available. Without it a lone client gets an empty list, never sends
	// RequestBreakInTarget at all, and shows "Unable to find a world to invade" of
	// its own accord — which looks exactly like a working rejection but exercises
	// nothing. The id is deliberately one no player can hold.
	if s.srv.Config.DebugForceBreakInReject && len(targets) == 0 {
		targets = append(targets, &ds2pb.BreakInTargetData{
			PlayerId: proto.Uint32(debugPhantomTargetID),
			PsnId:    proto.String("dso-reject-test"),
		})
		log.Info("DEBUG: injected a synthetic break-in target so the client will "+
			"send RequestBreakInTarget", "player_id", cs.playerID)
	}

	log.Info("break-in target list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cell_id", req.GetCellId(), "type", req.GetType(),
		"max", req.GetMaxTargets(), "returned", len(targets),
		"filtering", filtering, "invader_soul_memory", invaderSM,
		"skipped_not_invadable", skippedNotInvadable,
		"skipped_location", skippedLocation, "skipped_soul_memory", skippedSoul)

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

	if s.srv.Config.DebugForceBreakInReject {
		cs.breakInType = req.GetType()
		log.Info("DEBUG: forcing break-in rejection instead of invading",
			"invader_player_id", cs.playerID, "target_player_id", req.GetPlayerId(),
			"push_id", fmt.Sprintf("%#04x", s.rejectPushID(req.GetType())))
		s.pushBreakInRejected(log, cs, req.GetPlayerId())
		return proto.Marshal(&ds2pb.RequestBreakInTargetResponse{})
	}

	target, live := s.sessionForPlayerLocked(req.GetPlayerId())
	if !live {
		log.Info("break-in target is offline",
			"invader_player_id", cs.playerID, "target_player_id", req.GetPlayerId())
		s.pushBreakInRejected(log, cs, req.GetPlayerId())
		return proto.Marshal(&ds2pb.RequestBreakInTargetResponse{})
	}

	// Remember the invasion type: RequestRejectBreakInTarget does not carry it,
	// but the rejection push must use the same mode's alias.
	cs.breakInType = req.GetType()

	push := &ds2pb.PushRequestBreakInTarget{
		PushMessageId: ds2pb.PushMessageId(breakInPushIDFor(req.GetType(), breakInRoleTarget)).Enum(),
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
		"push_id", fmt.Sprintf("%#04x", breakInPushIDFor(req.GetType(), breakInRoleTarget)),
		"type", req.GetType(), "payload_bytes", len(body))

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

// debugPhantomTargetID is the fake target offered by the rejection harness. No
// real player can hold it: ids are AUTOINCREMENT from 100000 and this is below
// that floor, while still being non-zero.
const debugPhantomTargetID = 1

// Push alias layout, CONFIRMED at the instruction level in both v1.00 and v1.10.
//
//	opcode = breakInPushBase + 4*mode + role
//
// The sixteen aliases are NOT four aliases per message type. Each manager is
// instantiated once per gameplay mode, and every instance registers the same
// message types at a different quartet — so a "group" holds one of every type,
// for one mode. The shared callback re-derives the role from opcode-0x3B9 with
// the bitmasks 0x1111 (target), 0x2222 (reject), 0x4444 (allow).
//
// Getting this wrong cost three live test cycles: 0x3BD, 0x3C1 and 0x3C5 were
// tried as rejections and all ignored, because they are TARGET pushes for modes
// 1, 2 and 3. An invader naturally discards those.
//
// mode is the BreakInType from the request: RedEyeOrb=0, BlueEyeOrb=2.
const breakInPushBase = 0x03B9

// Roles within a mode's quartet. Role 3 exists numerically but has no handler —
// the callback has no 0x8888 branch, so a "remove" push is silently discarded on
// every one of the sixteen ids.
const (
	breakInRoleTarget = 0
	breakInRoleReject = 1
	breakInRoleAllow  = 2
)

// breakInPushIDFor returns the alias for a role within an invasion type.
func breakInPushIDFor(mode ds2pb.BreakInType, role int) int32 {
	return int32(breakInPushBase + 4*int(mode) + role)
}

// rejectPushID is the alias actually sent, honouring the debug override.
func (s *Service) rejectPushID(mode ds2pb.BreakInType) int32 {
	if v := s.srv.Config.BreakInRejectPushID; v != 0 {
		return int32(v)
	}
	return breakInPushIDFor(mode, breakInRoleReject)
}

// pushBreakInRejected tells an invader their attempt failed. Caller holds s.mu.
func (s *Service) pushBreakInRejected(log logger, invader *clientSession, hostID uint32) {
	pushID := s.rejectPushID(invader.breakInType)
	body, err := proto.Marshal(&ds2pb.PushRequestRejectBreakInTarget{
		PushMessageId: ds2pb.PushMessageId(pushID).Enum(),
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
		"invader_player_id", invader.playerID, "host_player_id", hostID,
		"push_id", fmt.Sprintf("%#04x", pushID), "payload_bytes", len(body))
}
