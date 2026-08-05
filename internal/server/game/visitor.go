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
	filtering := matchmakingEnabled()
	window := visitorTierWindows[req.GetType()]
	// Soul memory comes from the requester's own MatchingParameter rather than
	// their status blob: it is `required` here, so it is always present and is
	// the client's own view of what it is matching on.
	hostSM := req.GetMatchingParameter().GetSoulMemory()
	// Record it against the requester too. The status blob arrives in fragments
	// and can leave a player at soul memory 0 for a while; this is complete on
	// every request, so it keeps them matchable in the meantime.
	cs.profile.applyMatchingParameter(req.GetMatchingParameter())

	var skippedPool, skippedSoul int
	for _, other := range s.sessions {
		if other.playerID == 0 || other.playerID == cs.playerID {
			continue
		}
		if filtering {
			// The target must be standing in the pool being asked for. This is
			// the filter that matters: it is what stops us offering someone who
			// is at a bonfire, in the wrong area, or not carrying the seal.
			if visitorPoolFor(other.profile) != req.GetType() {
				skippedPool++
				continue
			}
			if !soulMemoryMatches(hostSM, other.profile.effectiveSoulMemory(), window) {
				skippedSoul++
				continue
			}
		}
		targets = append(targets, &ds2pb.VisitorData{
			PlayerId:    proto.Int64(int64(other.playerID)),
			PlayerPsnId: proto.String(other.accountID),
		})
		if max > 0 && len(targets) >= max {
			break
		}
	}

	// Returning zero is the normal, correct outcome most of the time — nobody is
	// usually standing in a rat area — so this stays at Info with the skip
	// reasons attached. "Returned 0 and skipped 1 for pool" is the difference
	// between a working filter and a broken area constant.
	log.Info("visitor target list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cell_id", req.GetCellId(), "type", req.GetType(),
		"max", req.GetMaxTargets(), "returned", len(targets),
		"filtering", filtering, "host_soul_memory", hostSM,
		"skipped_wrong_pool", skippedPool, "skipped_soul_memory", skippedSoul)

	// online_area_id and cell_id are `required` in the response and are echoed
	// back from the request — the client matches the reply to the area it asked
	// about.
	return proto.Marshal(&ds2pb.RequestGetVisitorListResponse{
		OnlineAreaId: proto.Int64(req.GetOnlineAreaId()),
		CellId:       proto.Int64(req.GetCellId()),
		TargetData:   targets,
	})
}

// handleVisit is a request to pull another player into the requester's world.
// The server notifies that player; the two clients negotiate from there.
//
// DIRECTION, because the message name invites the opposite reading and this has
// already caused one wrong diagnosis: the REQUESTER is the host, and the player
// named in player_id is the guest who travels. Confirmed live on 2026-08-05 by a
// Rat King summon, where the requester's own RequestNotifyJoinGuestPlayer
// recorded itself as host and the named player as guest.
//
// It holds for Bell Keepers too, where the covenant member is the one summoned
// away: the trespasser is the requester and the bell keeper is fetched to them.
// So "visit" is always "bring this player to me", never "send me to them".
func (s *Service) handleVisit(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestVisit
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestVisit: %w", err)
	}
	guestID := uint32(req.GetPlayerId())

	if !visitorModeInRange(req.GetType()) {
		log.Warn("visit with a covenant outside the push-alias block; "+
			"falling back to mode 0 — capture this",
			"player_id", cs.playerID, "type", req.GetType())
	}

	guest, live := s.sessionForPlayerLocked(guestID)
	if !live {
		log.Info("visit guest is offline",
			"host_player_id", cs.playerID, "guest_player_id", guestID)
		s.pushVisitRejected(log, cs, guestID, req.GetType())
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
	guest.conn.SendPush(body)

	log.Info("pushed visit to guest",
		"host_player_id", cs.playerID, "guest_player_id", guestID,
		"type", req.GetType(),
		"push_id", fmt.Sprintf("%#04x", visitorPushIDFor(req.GetType(), visitorRoleVisit)),
		"payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestVisitResponse{})
}

// handleRejectVisit is the GUEST declining to be pulled in, relayed back to the
// host who asked for them. See handleVisit for why that is the way round.
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
	hostID := uint32(req.GetPlayerId())

	if host, live := s.sessionForPlayerLocked(hostID); live {
		s.pushVisitRejected(log, host, cs.playerID, req.GetType())
	}
	log.Info("guest declined visit",
		"guest_player_id", cs.playerID, "host_player_id", hostID,
		"reason", req.GetUnknown_2())
	return proto.Marshal(&ds2pb.RequestRejectVisitResponse{})
}

// pushVisitRejected tells the HOST that the guest they asked for is not coming.
// It is what renders the client's "summoning failed" text. Caller holds s.mu.
func (s *Service) pushVisitRejected(log logger, host *clientSession, guestID uint32, vType ds2pb.VisitorType) {
	body, err := proto.Marshal(&ds2pb.PushRequestRejectVisit{
		PushMessageId: ds2pb.PushMessageId(visitorPushIDFor(vType, visitorRoleReject)).Enum(),
		PlayerId:      proto.Int64(int64(guestID)),
		Unknown_3:     proto.Int64(0),
		PsnId:         proto.String(host.accountID),
		Type:          vType.Enum(),
	})
	if err != nil {
		log.Warn("failed to build PushRequestRejectVisit", "err", err)
		return
	}
	host.conn.SendPush(body)
	log.Info("pushed visit rejection",
		"host_player_id", host.playerID, "guest_player_id", guestID,
		"type", vType, "push_id", fmt.Sprintf("%#04x", visitorPushIDFor(vType, visitorRoleReject)))
}
