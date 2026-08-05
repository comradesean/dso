package game

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Telemetry and session-lifecycle notifications.
//
// All of these are "M" opcodes: the client sends them through its
// no-response-callback path (EBOOT 0x1587DE8) and never retransmits, so none get
// a reply. They must still be registered in handledOpcodes, or they land in the
// "no handler" log alongside opcodes we genuinely have not built — and that log
// is the main signal for finding the next thing to implement.
const (
	opRequestNotifyJoinGuestPlayer   uint32 = 0x03E8
	opRequestNotifyLeaveGuestPlayer  uint32 = 0x03E9
	opRequestNotifyJoinSession       uint32 = 0x03EA
	opRequestNotifyLeaveSession      uint32 = 0x03EB
	opRequestNotifyRingBell          uint32 = 0x03EE
	opRequestNotifyKillEnemy         uint32 = 0x03F6
	opRequestNotifyBuyItem           uint32 = 0x03F7
	opRequestNotifyDisconnectSession uint32 = 0x03F9
)

// Cumulative world statistics, sharing the counters table with the death counter.
const (
	counterEnemiesKilled = "world.enemies_killed"
	counterItemsBought   = "world.items_bought"
	counterSoulsSpent    = "world.souls_spent"
)

// handleNotifyJoinSession records a player joining a co-op or invasion session.
func (s *Service) handleNotifyJoinSession(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyJoinSession
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyJoinSession: %w", err)
	}
	log.Info("session joined", "player_id", cs.playerID,
		"peer_player_id", req.GetField_1(), "session_kind", req.GetField_2(),
		"field_3", req.GetField_3(), "field_4", req.GetField_4())
	return nil, nil
}

// handleNotifyLeaveSession records a player leaving a session.
func (s *Service) handleNotifyLeaveSession(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyLeaveSession
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyLeaveSession: %w", err)
	}
	log.Info("session left", "player_id", cs.playerID,
		"peer_player_id", req.GetField_1(), "session_kind", req.GetField_2(),
		"field_3", req.GetField_3(), "field_4", req.GetField_4())
	return nil, nil
}

// handleNotifyJoinGuestPlayer records a guest (summon or invader) arriving in the
// sender's world.
//
// Two fields were decoded from live captures on 2026-08-05, by comparing this
// message against RequestNotifyJoinSession sent by the other party for the same
// event — the two are symmetric:
//
//	field_1  the PEER's player id (the guest here, the host in JoinSession)
//	field_7  the online area id, confirmed by matching the sign's own area
//	field_2  the session kind, observed live: 5 = ordinary summon sign,
//	         8 = arena duel, 9 = Mirror Knight squire, 13 and 14 seen during Bell
//	         Keeper covenant summons (14 confirmed as the grey-spirit visit; 13
//	         seen once alongside it and not yet attributed).
//
// field_9 remains an opaque blob and is logged by length only.
func (s *Service) handleNotifyJoinGuestPlayer(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyJoinGuestPlayer
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyJoinGuestPlayer: %w", err)
	}
	log.Info("guest player joined", "host_player_id", cs.playerID,
		"guest_player_id", req.GetField_1(), "session_kind", req.GetField_2(),
		"area_id", req.GetField_7(), "blob_bytes", len(req.GetField_9()))
	return nil, nil
}

// handleNotifyLeaveGuestPlayer records a guest leaving the sender's world.
func (s *Service) handleNotifyLeaveGuestPlayer(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyLeaveGuestPlayer
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyLeaveGuestPlayer: %w", err)
	}
	log.Info("guest player left", "host_player_id", cs.playerID,
		"guest_player_id", req.GetField_1(), "session_kind", req.GetField_2())
	return nil, nil
}

// handleNotifyRingBell records a bell ring.
//
// CONFIRMED BY OBSERVATION 2026-08-05: ringing a belfry bell does NOT send this.
// A player rang the Belfry Sol bell with full packet logging on and no 0x03EE
// ever arrived. DS2 has exactly two bells and neither is struck — you pull a
// lever in a side tower, and ringing is a gate opener and nothing else. It does
// not trigger, end or affect Bell Keeper invasions, which fire on a trespasser's
// presence in the belfry.
//
// So this opcode is misnamed for our purposes and is definitively NOT the
// event-chest trigger, which was the last reason to care about it. It remains
// logged in full because nothing has ever been seen sending it, and the payload
// would be the only evidence of what it actually is.
//
// On the "I can hear other players ringing bells" reports, which are real and go
// back to 2014, and which have to be reconciled with this opcode never firing:
//
// The leading explanation is GHOST REPLAY. Ghosts are recordings of another
// player's actions, fetched via RequestGetGhostDataList and replayed locally with
// no session between the two players — and the blob is opaque to us, so a bell
// pull inside one would reach a listener without any bell-specific message ever
// existing. That fits every part of the reports: heard without being invaded,
// sounding unlike DS1's global toll, genuinely another player's action, and with
// no identifiable trigger. It is INFERRED, not proven; proving it means decoding
// a ghost blob.
//
// An earlier note here claimed the sound was peer-to-peer audio heard only while
// a Bell Keeper phantom is in the host's world. That is true as far as it goes —
// a phantom does hear the host's bell — but it cannot explain why the community
// found the effect mysterious at all: anyone hearing it mid-invasion would know
// exactly what it was. Ghost replay explains the puzzlement; presence does not.
//
// DS1's global bell was genuine world state broadcast to everyone. DS2 has no
// such channel, so the absence of one here is not a missing feature either way.
func (s *Service) handleNotifyRingBell(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	// Not parsed: the message is a TODO in our schema, so parsing it would only
	// assert a shape we have no evidence for. The bytes are the useful artefact.
	log.Info("BELL RUNG - unmapped message, capture this",
		"player_id", cs.playerID, "payload_bytes", len(payload),
		"payload_hex", fmt.Sprintf("%x", payload))
	return nil, nil
}

// handleNotifyKillEnemy adds to the world enemy-kill counter.
//
// The message is a repeated group of (enemy_id, count) pairs, so one message can
// carry many kills — the client batches them rather than sending one per death.
func (s *Service) handleNotifyKillEnemy(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyKillEnemy
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyKillEnemy: %w", err)
	}

	var total int64
	for _, e := range req.GetEnemyCount() {
		n := e.GetEnemyCount()
		// Client-supplied and unvalidated, same as the offline death batch.
		if n <= 0 || n > maxOfflineDeathBatch {
			log.Warn("ignoring implausible enemy kill count",
				"player_id", cs.playerID, "enemy_id", e.GetEnemyId(), "count", n)
			continue
		}
		total += n
	}
	if total == 0 {
		return nil, nil
	}

	world, err := s.store.AddCounter(context.Background(), counterEnemiesKilled, total)
	if err != nil {
		return nil, err
	}
	log.Info("enemies killed", "player_id", cs.playerID,
		"kinds", len(req.GetEnemyCount()), "count", total, "world_total", world)
	return nil, nil
}

// handleNotifyBuyItem records a merchant purchase.
func (s *Service) handleNotifyBuyItem(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyBuyItem
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyBuyItem: %w", err)
	}

	qty := int64(req.GetQuantity())
	if qty > 0 && qty <= maxOfflineDeathBatch {
		if _, err := s.store.AddCounter(context.Background(), counterItemsBought, qty); err != nil {
			return nil, err
		}
	}
	if souls := int64(req.GetSoulsSpent()); souls > 0 {
		if _, err := s.store.AddCounter(context.Background(), counterSoulsSpent, souls); err != nil {
			return nil, err
		}
	}

	log.Info("item bought", "player_id", cs.playerID,
		"merchant_id", req.GetMerchantId(), "item_id", req.GetItemId(),
		"quantity", req.GetQuantity(), "souls_spent", req.GetSoulsSpent())
	return nil, nil
}

// handleNotifyDisconnectSession records the client announcing a session teardown.
//
// This is the client telling us it is going away, which is a chance to clean up
// immediately rather than waiting out the 60s idle timeout. It is not acted on
// yet: the message carries a session identifier we do not track, and dropping the
// wrong session would be worse than reaping it late.
func (s *Service) handleNotifyDisconnectSession(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyDisconnectSession
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyDisconnectSession: %w", err)
	}
	log.Info("session disconnect announced",
		"player_id", cs.playerID, "field_1", req.GetField_1())
	return nil, nil
}
