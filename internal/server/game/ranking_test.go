package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

func submitScore(t *testing.T, svc *Service, log logger, cs *clientSession, characterID uint32, inc uint32) {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestRegisterPowerStoneData{
		CharacterId: proto.Uint32(characterID),
		Increment:   proto.Uint32(inc),
		Data:        []byte{0x01, 0x02},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleRegisterPowerStoneData(log, cs, raw); err != nil {
		t.Fatal(err)
	}
}

func rankingPage(t *testing.T, svc *Service, log logger, cs *clientSession, offset, count uint32) []*ds2pb.PowerStoneRankingData {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestGetPowerStoneRanking{
		Offset: proto.Uint32(offset),
		Count:  proto.Uint32(count),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleGetPowerStoneRanking(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetPowerStoneRankingResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	return resp.GetData()
}

// TestScoresAccumulate — the client submits increments, not totals.
func TestScoresAccumulate(t *testing.T) {
	svc, log, cs := testService(t)

	submitScore(t, svc, log, cs, 42, 100)
	submitScore(t, svc, log, cs, 42, 250)

	page := rankingPage(t, svc, log, cs, 1, 10)
	if len(page) != 1 {
		t.Fatalf("board has %d entries, want 1", len(page))
	}
	if got := page[0].GetScore(); got != 350 {
		t.Errorf("score = %d, want 350 (increments must accumulate)", got)
	}
}

// TestRankVsSerialRank pins the distinction between the two rank fields, which is
// the whole reason the message carries both.
//
// serial_rank is a unique position; rank is a competition rank where ties share a
// value. With scores 500, 300, 300, 100 the ranks are 1, 2, 2, 4 while the serial
// ranks are 1, 2, 3, 4.
func TestRankVsSerialRank(t *testing.T) {
	svc, log, cs := testService(t)

	submitScore(t, svc, log, cs, 1, 500)
	submitScore(t, svc, log, cs, 2, 300)
	submitScore(t, svc, log, cs, 3, 300)
	submitScore(t, svc, log, cs, 4, 100)

	page := rankingPage(t, svc, log, cs, 1, 10)
	if len(page) != 4 {
		t.Fatalf("board has %d entries, want 4", len(page))
	}

	wantSerial := []uint32{1, 2, 3, 4}
	wantRank := []uint32{1, 2, 2, 4}
	for i, e := range page {
		if e.GetSerialRank() != wantSerial[i] {
			t.Errorf("row %d: serial_rank = %d, want %d (must be a unique position)",
				i, e.GetSerialRank(), wantSerial[i])
		}
		if e.GetRank() != wantRank[i] {
			t.Errorf("row %d (score %d): rank = %d, want %d (ties must share a rank)",
				i, e.GetScore(), e.GetRank(), wantRank[i])
		}
	}

	// Descending by score is the point of a leaderboard.
	for i := 1; i < len(page); i++ {
		if page[i-1].GetScore() < page[i].GetScore() {
			t.Errorf("board is not sorted descending: %d before %d",
				page[i-1].GetScore(), page[i].GetScore())
		}
	}
}

// TestRankingOffsetIsOneBased — the client's offset starts at 1, matching the
// reference. Treating it as 0-based silently drops the top entry.
func TestRankingOffsetIsOneBased(t *testing.T) {
	svc, log, cs := testService(t)
	submitScore(t, svc, log, cs, 1, 500)
	submitScore(t, svc, log, cs, 2, 300)
	submitScore(t, svc, log, cs, 3, 100)

	first := rankingPage(t, svc, log, cs, 1, 1)
	if len(first) != 1 || first[0].GetScore() != 500 {
		t.Fatalf("offset=1 should return the top entry, got %v", first)
	}
	second := rankingPage(t, svc, log, cs, 2, 1)
	if len(second) != 1 || second[0].GetScore() != 300 {
		t.Fatalf("offset=2 should return the second entry, got %v", second)
	}
	// Offset 0 must not underflow into an error or a wrapped page.
	zero := rankingPage(t, svc, log, cs, 0, 1)
	if len(zero) != 1 || zero[0].GetScore() != 500 {
		t.Errorf("offset=0 should be treated as the start, got %v", zero)
	}
}

// TestMyRankingForUnrankedCharacter is the one that would hang a client rather
// than error. `data` is a required field, so an unranked character still needs a
// complete zero row — an omitted one is rejected by the proto2 parser.
func TestMyRankingForUnrankedCharacter(t *testing.T) {
	svc, log, cs := testService(t)

	raw, err := proto.Marshal(&ds2pb.RequestGetPowerStoneMyRanking{
		CharacterId: proto.Uint32(777),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleGetPowerStoneMyRanking(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetPowerStoneMyRankingResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatalf("client-side parse would reject our reply: %v", err)
	}
	if resp.GetData() == nil {
		t.Fatal("data absent; it is required and the client would reject the message")
	}
	if resp.GetData().GetScore() != 0 || resp.GetData().GetRank() != 0 {
		t.Errorf("unranked character reported rank %d score %d, want zeros",
			resp.GetData().GetRank(), resp.GetData().GetScore())
	}
}

// TestMyRankingRanksAgainstWholeBoard — the character's rank must come from its
// position among everyone, not from a board filtered down to itself.
func TestMyRankingRanksAgainstWholeBoard(t *testing.T) {
	svc, log, cs := testService(t)
	submitScore(t, svc, log, cs, 1, 500)
	submitScore(t, svc, log, cs, 2, 300)
	submitScore(t, svc, log, cs, 3, 100)

	raw, err := proto.Marshal(&ds2pb.RequestGetPowerStoneMyRanking{
		CharacterId: proto.Uint32(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleGetPowerStoneMyRanking(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetPowerStoneMyRankingResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if got := resp.GetData().GetRank(); got != 3 {
		t.Errorf("last-placed character reports rank %d, want 3 — filtering before "+
			"ranking would always yield 1", got)
	}
}

// TestRecordCount backs the client's paging.
func TestRecordCount(t *testing.T) {
	svc, log, cs := testService(t)

	count := func() uint32 {
		raw, err := proto.Marshal(&ds2pb.RequestGetPowerStoneRankingRecordCount{})
		if err != nil {
			t.Fatal(err)
		}
		replyRaw, err := svc.handleGetPowerStoneRankingRecordCount(log, cs, raw)
		if err != nil {
			t.Fatal(err)
		}
		var resp ds2pb.RequestGetPowerStoneRankingRecordCountResponse
		if err := proto.Unmarshal(replyRaw, &resp); err != nil {
			t.Fatal(err)
		}
		return resp.GetCount()
	}

	if got := count(); got != 0 {
		t.Errorf("fresh board count = %d, want 0", got)
	}
	submitScore(t, svc, log, cs, 1, 10)
	submitScore(t, svc, log, cs, 2, 20)
	// Same character twice must not add a second row.
	submitScore(t, svc, log, cs, 1, 10)
	if got := count(); got != 2 {
		t.Errorf("count = %d, want 2 (one row per character)", got)
	}
}

// TestScoreIncrementIsBounded — the increment is client-supplied and the board is
// persistent, so an unbounded value would pin the top of it permanently.
func TestScoreIncrementIsBounded(t *testing.T) {
	svc, log, cs := testService(t)

	submitScore(t, svc, log, cs, 1, 10)
	submitScore(t, svc, log, cs, 2, maxScoreIncrement+1)

	page := rankingPage(t, svc, log, cs, 1, 10)
	if len(page) != 1 {
		t.Fatalf("board has %d entries, want 1 — the absurd submission must be refused",
			len(page))
	}
	if page[0].GetCharacterId() != 1 {
		t.Errorf("surviving entry is character %d, want 1", page[0].GetCharacterId())
	}
}
