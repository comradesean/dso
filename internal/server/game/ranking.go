package game

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
	"github.com/sstreight/dso/internal/server/store"
)

// Power-stone ranking opcodes — the Champion's Tablet.
//
// No item called a "Power Stone" exists in DS2; the protocol name is
// FromSoftware's. In game this is the Victor's Stone in the Undead Purgatory
// area, whose tablet lists the TOP 100 players by Awestones offered, with names.
// That top-100 cap is why the client pages (offset/count) and asks for a record
// count separately, and why submissions carry an increment rather than a total.
//
// The displayed name almost certainly rides in the opaque `data` blob, which we
// store and echo without interpreting.
//
// All four are request/response and must be answered. The board is persisted:
// a leaderboard that resets whenever the server restarts is not a leaderboard.
const (
	opRequestRegisterPowerStoneData         uint32 = 0x03F3
	opRequestGetPowerStoneRanking           uint32 = 0x03F4
	opRequestGetPowerStoneMyRanking         uint32 = 0x03F5
	opRequestGetPowerStoneRankingRecordCoun uint32 = 0x03F8
)

// maxScoreIncrement bounds a single submission.
//
// The client sends an increment, not a total, and nothing validates it. Without
// a bound one modified client could pin the top of a persistent board forever,
// and the damage would outlive the session. Same reasoning as the offline death
// batch.
const maxScoreIncrement = 1000000

// maxRankingPage bounds how many rows one request can pull back. It matches the
// tablet's own top-100 display, and guards against an absurd count building an
// enormous reply.
const maxRankingPage = 100

// rankingToProto renders a stored board row for the wire.
func rankingToProto(r *store.PowerStoneRanking) *ds2pb.PowerStoneRankingData {
	return &ds2pb.PowerStoneRankingData{
		PlayerId:    proto.Uint32(r.PlayerID),
		CharacterId: proto.Uint32(r.CharacterID),
		SerialRank:  proto.Uint32(r.SerialRank),
		Rank:        proto.Uint32(r.Rank),
		Score:       proto.Uint32(clampScore(r.Score)),
		Data:        r.Data,
	}
}

// clampScore narrows the stored int64 to the wire's uint32 rather than wrapping.
func clampScore(v int64) uint32 {
	if v < 0 {
		return 0
	}
	if v > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(v)
}

// handleRegisterPowerStoneData records a score submission.
//
// The board is keyed by (player_id, character_id). RequestGetPowerStoneMyRanking
// names only a character, but character_id is the client's local slot number —
// every player has a character 1 — so the caller's player id supplies the rest of
// the key, which is what the reference does too. Keying on character_id alone
// would merge every player's first character into one board entry.
func (s *Service) handleRegisterPowerStoneData(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRegisterPowerStoneData
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRegisterPowerStoneData: %w", err)
	}

	inc := int64(req.GetIncrement())
	if inc < 0 || inc > maxScoreIncrement {
		log.Warn("ignoring implausible power stone increment",
			"player_id", cs.playerID, "character_id", req.GetCharacterId(), "increment", inc)
		return proto.Marshal(&ds2pb.RequestRegisterPowerStoneDataResponse{})
	}

	total, err := s.store.AddPowerStoneScore(context.Background(),
		cs.playerID, req.GetCharacterId(), inc, req.GetData())
	if err != nil {
		return nil, err
	}

	log.Info("power stone score registered",
		"player_id", cs.playerID, "character_id", req.GetCharacterId(),
		"increment", inc, "total", total, "data_bytes", len(req.GetData()))

	return proto.Marshal(&ds2pb.RequestRegisterPowerStoneDataResponse{})
}

// handleGetPowerStoneRanking answers a page of the leaderboard.
//
// The offset is 1-based, matching the reference's handling of the same field.
func (s *Service) handleGetPowerStoneRanking(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetPowerStoneRanking
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetPowerStoneRanking: %w", err)
	}

	count := req.GetCount()
	if count == 0 || count > maxRankingPage {
		count = maxRankingPage
	}

	rows, err := s.store.PowerStoneRankings(context.Background(), req.GetOffset(), count)
	if err != nil {
		return nil, err
	}

	datas := make([]*ds2pb.PowerStoneRankingData, 0, len(rows))
	for _, r := range rows {
		datas = append(datas, rankingToProto(r))
	}

	log.Info("power stone ranking page",
		"player_id", cs.playerID, "offset", req.GetOffset(),
		"requested", req.GetCount(), "returned", len(datas))

	return proto.Marshal(&ds2pb.RequestGetPowerStoneRankingResponse{Data: datas})
}

// handleGetPowerStoneMyRanking answers one character's own placement.
//
// `data` is `required` in the response, so a character with no entry still needs
// a fully-populated zero row rather than an omitted one — an unset required field
// produces a message the client's proto2 parser rejects outright, which surfaces
// as a hang rather than an empty board.
func (s *Service) handleGetPowerStoneMyRanking(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetPowerStoneMyRanking
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetPowerStoneMyRanking: %w", err)
	}

	// Keyed by the caller's player id as well as the character: character_id is
	// the client's local slot number, so every player has a character 1.
	r, found, err := s.store.PowerStoneRankingFor(context.Background(),
		cs.playerID, req.GetCharacterId())
	if err != nil {
		return nil, err
	}

	var data *ds2pb.PowerStoneRankingData
	if found {
		data = rankingToProto(r)
	} else {
		data = &ds2pb.PowerStoneRankingData{
			PlayerId:    proto.Uint32(cs.playerID),
			CharacterId: proto.Uint32(req.GetCharacterId()),
			SerialRank:  proto.Uint32(0),
			Rank:        proto.Uint32(0),
			Score:       proto.Uint32(0),
			Data:        []byte{},
		}
	}

	log.Info("power stone my ranking",
		"player_id", cs.playerID, "character_id", req.GetCharacterId(),
		"found", found, "rank", data.GetRank(), "score", data.GetScore())

	return proto.Marshal(&ds2pb.RequestGetPowerStoneMyRankingResponse{Data: data})
}

// handleGetPowerStoneRankingRecordCount answers the board size, for paging.
func (s *Service) handleGetPowerStoneRankingRecordCount(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetPowerStoneRankingRecordCount
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetPowerStoneRankingRecordCount: %w", err)
	}

	n, err := s.store.PowerStoneRankingCount(context.Background())
	if err != nil {
		return nil, err
	}
	log.Info("power stone record count", "player_id", cs.playerID, "count", n)

	return proto.Marshal(&ds2pb.RequestGetPowerStoneRankingRecordCountResponse{
		Count: proto.Uint32(n),
	})
}
