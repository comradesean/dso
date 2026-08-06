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
//	         seen once alongside it and not yet attributed), 15 = Rat King prey,
//	         10 = Dragon Remnants duel via a Dragon Eye sign (sign_type 6),
//	         7 = break-in invasion (confirmed in the Dark Chasm of Old).
//
// The Rat King case is the one that inverts: the covenant member is the HOST and
// the victim is the guest pulled into their world, the opposite of Bell Keepers
// where the covenant member travels. Confirmed live 2026-08-05 — the rat sent
// RequestVisit and the log recorded host=rat, guest=victim.
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
// The name is NOT inherited from the PC protos — it is in this binary. The
// message's GetTypeName (v1.10 vtable 0x1CE1B60 slot 2) returns the literal
// "Frpg2RequestMessage.RequestNotifyRingBell".
//
// CONFIRMED BY OBSERVATION 2026-08-05: ringing a belfry bell does NOT send this.
// A player rang Belfry Sol, and later Belfry Luna five times consecutively, with
// full packet logging on, and no 0x03EE ever arrived.
//
// The decompilation explains why. The send at 0x15D0178 is reached only from a
// data-driven script command interpreter, gated on command id 130631 (0x1FE47).
// That constant occurs EXACTLY ONCE in the whole executable — at the dispatcher
// comparison — so there is no second producer and no table. Both v1.00 and v1.10
// are instruction-for-instruction identical here, so this is not a version
// difference.
//
// THE SCRIPT SIDE IS NOW CONFIRMED TOO. The PS3 GameData archive is a plain
// unencrypted BXF4 pair and can be read directly (tools/gamedata/bhf4.py).
// Scanning all 475 EzState scripts for command 130631 finds it in exactly two:
//
//	ezstate\event_m10_16_00_00.esd   Belfry Luna   x2
//	ezstate\event_m10_19_00_00.esd   Belfry Sol    x2
//
// and nowhere else in the game. So the opcode IS reachable in retail, it IS the
// bell, and it is issued only in the two belfries. Both call sites in a file are
// byte-identical, so which one runs cannot be told apart without an EzState
// decompiler.
//
// An earlier note here said a scan of the archive "would not settle it either
// way, since the entries are compressed". That was wrong and nothing had checked
// the header — the index is plaintext and carries every filename.
//
// OBSERVED LIVE 2026-08-06, the first and so far only time in this project:
//
//	payload_hex = 08 80 8f ec 04 12 00
//
// which is field 1 = 10160000 (Belfry Luna's map id) and field 2 empty. That is
// EXACTLY the shape reconstructed from the serialiser before any frame had ever
// been seen — `08 <varint mapid> 12 00` — so the decompiled payload layout is
// confirmed byte for byte, including that field 2 is always sent empty.
//
// The sequence it arrived in:
//
//	02:48:46  Bell Keeper session forms in Belfry Luna (kind 14)
//	02:50:10  the HOST dies; the Bell Keeper reports the kill
//	02:50:17  the HOST's client sends 0x03EE, 6.3s later
//
// HOST DEATH IS NECESSARY BUT NOT SUFFICIENT. An identical host death eight
// minutes earlier, same pair, same session kind, same map, produced no bell at
// all. So something else gates the final step, and one capture cannot say what.
// Note also that the sender is the player who DIED, not the victor.
//
// THE CONDITION INVOLVES HOST DEATH, NOT MERELY RINGING THE BELL.
//
// The send branch is gated behind 「ホスト死亡判定」 — host death determination —
// and the bell object's action prompt is disabled from map load until that same
// condition holds. A living player, alone or in a session, has no path to it.
// The name is confirmed from FromSoftware's own .edd command dictionaries and
// the executable's RTTI string, so the message really is the bell; only our
// assumption about what rings it was wrong.
//
// That is the actual DS2 mechanic: the bell TOLLS WHEN SOMEONE DIES in a belfry
// duel. It also finally explains the long-standing player reports of hearing
// bells with no invasion of their own in progress — they are hearing a Bell
// Keeper fight end somewhere else, which is why it sounded mysterious and why it
// never correlated with anything the listener was doing.
//
// This retires two theories that fit the evidence and were both wrong: that the
// script fires once on a first ring, and that the guard merely required an
// active multiplayer session. A player rang Belfry Luna five times solo, and
// later once more with a Bell Keeper session live in their world, and neither
// produced a packet.
//
// CAUTION on the earlier claim here that command 130631 appeared in five files
// across the DLC archive, including two AI scripts and Brightstone Cove
// Tseldora. Those were COINCIDENTAL BYTE MATCHES. A full structural parse of
// those scripts visited every command entry (760/760 and 768/768) and found no
// real uses. Raw byte scanning with tools/gamedata/bhf4.py grep32 over-reports;
// only a structural parse settles whether a hit is a command id.
//
// Payload, recovered from the serialiser rather than guessed:
//
//	field 1  uint32, the current map id in DS2's decimal convention
//	         (m10_02_00_00 -> 10020000), from the same helper that feeds
//	         RequestNotifyJoinGuestPlayer
//	field 2  bytes, and the only caller always constructs it EMPTY
//
// So a real frame would be just: 08 <varint mapid> 12 00.
//
// On the "I can hear other players ringing bells" reports, which are real and go
// back to 2014: there IS a server-to-client broadcast for this. Opcode 0x03EF is
// PushRequestNotifyRingBell — confirmed by GetTypeName, and long mis-documented
// here as a session-disconnect push. The client already has a handler for it.
//
// That supersedes the ghost-replay theory previously written here, which was
// only ever an inference made on the assumption that no bell channel existed. A
// dedicated push is the far simpler explanation, and unlike ghost replay it is
// directly testable: send a 0x03EF and listen. Its four fields (uint32, uint32,
// uint32, bytes) are of unknown meaning, so that test starts as a guess.
//
// This opcode remains definitively NOT the event-chest trigger.
func (s *Service) handleNotifyRingBell(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	// Still logged raw. The shape above is known, but nothing has ever sent one,
	// so the first real payload is worth having verbatim to check it against.
	log.Info("BELL RUNG - never yet observed, capture this",
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
