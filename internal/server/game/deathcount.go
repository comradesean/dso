package game

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// World death-counter opcodes. All four confirmed at the instruction level in
// the retail EBOOT and seen live from real clients.
//
// Only 0x03F1 and 0x03F2 feed the total. 0x03ED is the KILLER's report of the
// same death the victim already reported with 0x03F1 — counting it would double
// every PvP death. Our own capture shows both arriving 55ms apart from two
// different consoles for a single kill.
//
// Only 0x03F0 gets a reply. The other three go through the client's
// no-response-callback send path (EBOOT 0x1587DE8) and are never retransmitted
// when ignored. 0x03F0 *is* retried — twice, 5.3s apart — and that unanswered
// retry is exactly what made the in-game counter time out.
const (
	opRequestNotifyKillPlayer        uint32 = 0x03ED
	opRequestGetTotalDeathCount      uint32 = 0x03F0
	opRequestNotifyDeath             uint32 = 0x03F1
	opRequestNotifyOfflineDeathCount uint32 = 0x03F2
)

// counterTotalDeaths is the store key for the world death total.
const counterTotalDeaths = "player.total_deaths"

// maxOfflineDeathBatch bounds a single RequestNotifyOfflineDeathCount.
//
// The count is a client-supplied int64 that nothing validates. Without a bound
// one modified client could pin the world counter at its maximum permanently,
// and the counter is persistent, so the damage would outlive the session.
const maxOfflineDeathBatch = 10000

// handleNotifyDeath records one player death. No reply: this opcode registers no
// response callback in the client.
func (s *Service) handleNotifyDeath(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyDeath
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyDeath: %w", err)
	}
	total, err := s.store.AddCounter(context.Background(), counterTotalDeaths, 1)
	if err != nil {
		return nil, err
	}
	log.Info("player died",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cell_id", req.GetCellId(), "world_total", total)
	return nil, nil
}

// handleNotifyOfflineDeathCount adds a batch of deaths the client accumulated
// while offline. No reply.
func (s *Service) handleNotifyOfflineDeathCount(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyOfflineDeathCount
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyOfflineDeathCount: %w", err)
	}

	count := req.GetCount()
	if count <= 0 {
		log.Warn("ignoring non-positive offline death batch",
			"player_id", cs.playerID, "count", count)
		return nil, nil
	}
	if count > maxOfflineDeathBatch {
		log.Warn("clamping implausible offline death batch",
			"player_id", cs.playerID, "count", count, "clamped_to", maxOfflineDeathBatch)
		count = maxOfflineDeathBatch
	}

	total, err := s.store.AddCounter(context.Background(), counterTotalDeaths, count)
	if err != nil {
		return nil, err
	}
	log.Info("offline deaths reported",
		"player_id", cs.playerID, "count", count, "world_total", total)
	return nil, nil
}

// handleNotifyKillPlayer records a PvP kill.
//
// Deliberately does NOT touch the death counter: the victim's client already
// reported this same death with RequestNotifyDeath, so counting both would double
// every PvP death. It is logged because it is the only record we get of who
// killed whom, which is useful for the invasion work.
func (s *Service) handleNotifyKillPlayer(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestNotifyKillPlayer
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestNotifyKillPlayer: %w", err)
	}
	log.Info("player killed another player",
		"killer_player_id", cs.playerID, "victim_player_id", req.GetField_2())
	return nil, nil
}

// handleGetTotalDeathCount answers the in-game world death counter.
//
// The request is zero bytes — the client supplies no area, character or scope of
// any kind — so the only thing it can be asking for is a single world-wide
// number. This one DOES need a reply.
func (s *Service) handleGetTotalDeathCount(log logger, cs *clientSession, _ []byte) ([]byte, error) {
	total, err := s.store.Counter(context.Background(), counterTotalDeaths)
	if err != nil {
		return nil, err
	}
	// The wire field is uint32 while the client stores it in 64 bits, so clamp
	// rather than silently wrapping.
	if total < 0 {
		total = 0
	}
	if total > math.MaxUint32 {
		total = math.MaxUint32
	}

	log.Info("world death count requested", "player_id", cs.playerID, "total", total)

	// total_death_count is `required`, so it must be set even at zero — an unset
	// required field produces a message the client's proto2 parser rejects.
	return proto.Marshal(&ds2pb.RequestGetTotalDeathCountResponse{
		TotalDeathCount: proto.Uint32(uint32(total)),
	})
}
