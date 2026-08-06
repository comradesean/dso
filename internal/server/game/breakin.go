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

// SIN IS ENFORCED HERE, because the client does not enforce it.
//
// The Cracked Blue Eye Orb targets only hosts carrying enough sin to be a
// Sinner. Left ungated, a Blue Sentinel invaded a player with no sin at all
// simply for sharing a zone — so the rule has to live on this side. sinner_points
// comes from StatsInfo in the status blob, a sub-message the profile had not been
// reading, which is why an earlier hunt for a sin counter across PlayerStatus and
// ItemUsingInfo found nothing while a player was actively earning it.
//
// The orb's other restriction — that only Blue Sentinels may use it — is NOT
// enforced. Nothing has been seen violating it, and covenant membership is the
// invader's own client's business.
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

	var skippedNotInvadable, skippedLocation, skippedSoul, skippedNoSin int
	// Cells of hosts who pass every check except position — the evidence for
	// whether the client ever asks about a cell someone is actually in.
	var availableCells []uint32
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
			// Same area — the coarse 8-digit id, 40030000 for the whole Dark
			// Chasm complex.
			//
			// EXCEPT for the Cracked Blue Eye Orb, which is explicitly not
			// area-bound: it searches the invader's own area first and then
			// looks elsewhere, and the game says outright that "you will not
			// necessarily invade in the area where you used the Orb". Applying
			// the Red Eye Orb's locality to it is the same error as the cell
			// filter that once broke Dark Chasm invasions — a restriction we
			// invented rather than one the game has.
			//
			// Observed live: a Blue Sentinel's orb returned nothing with
			// skipped_location=1, i.e. we had a candidate and discarded them
			// purely for being in another area.
			if req.GetType() != ds2pb.BreakInType_BreakInType_BlueEyeOrb &&
				other.profile.onlineArea != req.GetOnlineAreaId() {
				skippedLocation++
				continue
			}
			if !soulMemoryMatches(invaderSM, other.profile.effectiveSoulMemory(),
				breakInTierWindow) {
				skippedSoul++
				continue
			}
			// A Cracked Blue Eye Orb hunts SINNERS, and THE CLIENT DOES NOT
			// ENFORCE THAT — we must.
			//
			// Proven live: a Blue Sentinel invaded a player with no sin at all,
			// purely because they shared a zone. A gate was briefly left out of
			// here on the reasoning that the orb worked without one and the
			// client presumably applied its own rules. It does not, and that
			// reasoning was wrong.
			//
			// Fails OPEN when StatsInfo has never arrived for that player, so an
			// absent field cannot silently exclude everybody — the failure this
			// project keeps producing by guessing strict. A player we have stats
			// for and who has no sin is excluded; a player we know nothing about
			// is still offered.
			if req.GetType() == ds2pb.BreakInType_BreakInType_BlueEyeOrb &&
				other.profile.statsSeen && other.profile.sinnerPoints == 0 {
				skippedNoSin++
				continue
			}

			// Deliberately NOT filtered on cell either.
			//
			// A Cracked Red Eye Orb is supposed to reach any of the three Dark
			// Chasms in a single use. Filtering by cell made that false: each use
			// is one query for one cell, the client does not keep searching
			// within a use, so a miss burned the attempt and the player had to
			// retry until the cell they were assigned happened to match where
			// the target stood.
			//
			// Cross-cell targets WERE being refused, but the cause was ours: the
			// push echoed the invader's requested cell, so a host in 400330 was
			// told it was being invaded in 400310 and correctly refused a
			// location it was not in. handleBreakInTarget now sends the host's
			// own cell instead. See the note there.
			availableCells = append(availableCells, other.profile.onlineActivityArea)
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
		"skipped_location", skippedLocation, "skipped_soul_memory", skippedSoul,
		"skipped_no_sin", skippedNoSin,
		// The decisive pair. "asked for 400320, hosts are at [400330]" over many
		// queries tells us whether the client cycles cells at all, and whether it
		// ever names its own — which is the thing still unexplained.
		"available_cells", fmt.Sprint(availableCells))

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

	// Tell the host where the session actually happens: THEIR cell, not the one
	// the invader's client happened to ask about.
	//
	// Echoing the invader's cell is what caused cross-chasm invasions to fail. A
	// Cracked Red Eye Orb reaches any of the three Dark Chasms, so the invader
	// routinely queries a cell it is not standing in and gets matched to a host
	// elsewhere. Sending that queried cell told a host in 400330 it was being
	// invaded in 400310; it checked where it was, disagreed, and refused inside
	// ~100ms — which the invader read as "unable to find a world to invade".
	//
	// The invader travels to the host, so the host's location is the correct
	// one for both parties. Falls back to the request when we have no profile
	// for the host, which is only the case before their first status blob.
	areaID, cellID := req.GetOnlineAreaId(), req.GetCellId()
	if target.profile.onlineActivityArea != 0 {
		cellID = target.profile.onlineActivityArea
		if target.profile.onlineArea != 0 {
			areaID = target.profile.onlineArea
		}
	}

	push := &ds2pb.PushRequestBreakInTarget{
		PushMessageId: ds2pb.PushMessageId(breakInPushIDFor(req.GetType(), breakInRoleTarget)).Enum(),
		PlayerId:      proto.Uint32(cs.playerID),
		PsnId:         proto.String(cs.accountID),
		Type:          req.GetType().Enum(),
		OnlineAreaId:  proto.Uint32(areaID),
		CellId:        proto.Uint32(cellID),
	}
	body, err := proto.Marshal(push)
	if err != nil {
		return nil, fmt.Errorf("build PushRequestBreakInTarget: %w", err)
	}
	target.conn.SendPush(body)

	log.Info("pushed break-in to target",
		"invader_player_id", cs.playerID, "target_player_id", req.GetPlayerId(),
		"push_id", fmt.Sprintf("%#04x", breakInPushIDFor(req.GetType(), breakInRoleTarget)),
		"type", req.GetType(), "payload_bytes", len(body),
		// Both, so a rejection can be attributed immediately: if these differ and
		// the host still refuses, the cell echo was not the cause.
		"requested_cell", req.GetCellId(), "pushed_cell", cellID)

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
