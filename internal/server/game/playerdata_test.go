package game

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/proto/ds2pb"
	"github.com/sstreight/dso/internal/server/core"
	"github.com/sstreight/dso/internal/server/store"
)

// TestCharacterDataSurvivesRestart is the point of persisting any of this. A
// file-backed store is used rather than :memory: precisely so it can be closed
// and reopened, which an in-memory database cannot demonstrate.
func TestCharacterDataSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dso.db")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	blob := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	open := func() (*Service, func()) {
		st, err := store.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		return &Service{
			srv:   &core.Server{Config: config.Default(), Logger: log},
			store: st,
		}, func() { _ = st.Close() }
	}

	// First run: log in, claim a slot, upload a character.
	svc, closeA := open()
	cs := &clientSession{accountID: "comradesean"}
	pid, err := svc.store.PlayerID(context.Background(), cs.accountID)
	if err != nil {
		t.Fatal(err)
	}
	cs.playerID = pid
	cs.characterID = 1

	raw, err := proto.Marshal(&ds2pb.RequestUpdatePlayerCharacter{
		CharacterId:   proto.Uint32(1),
		CharacterData: blob,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleUpdatePlayerCharacter(log, cs, raw); err != nil {
		t.Fatal(err)
	}
	closeA()

	// Second run: same account must get the same player id and see its blob.
	svc, closeB := open()
	defer closeB()

	pid2, err := svc.store.PlayerID(context.Background(), "comradesean")
	if err != nil {
		t.Fatal(err)
	}
	if pid2 != pid {
		t.Fatalf("player id changed across restart: %d then %d; every cached "+
			"reference to this player would now point at the wrong person", pid, pid2)
	}

	getRaw, err := proto.Marshal(&ds2pb.RequestGetPlayerCharacter{
		PlayerId:    proto.Uint32(pid2),
		CharacterId: proto.Uint32(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleGetPlayerCharacter(log, &clientSession{playerID: pid2}, getRaw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetPlayerCharacterResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.GetCharacterData()) != string(blob) {
		t.Errorf("character data = %x, want %x", resp.GetCharacterData(), blob)
	}
}

// TestGetPlayerCharacterAnswersForUnknownCharacter — all three response fields
// are required, so an unknown character still needs a complete reply. Silence
// stalls whatever UI is waiting, and the client will not open other online UI
// while a request is outstanding.
func TestGetPlayerCharacterAnswersForUnknownCharacter(t *testing.T) {
	svc, log, cs := testService(t)

	raw, err := proto.Marshal(&ds2pb.RequestGetPlayerCharacter{
		PlayerId:    proto.Uint32(999),
		CharacterId: proto.Uint32(7),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleGetPlayerCharacter(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(replyRaw) == 0 {
		t.Fatal("empty reply; the required fields must be present even when the " +
			"character is unknown")
	}
	var resp ds2pb.RequestGetPlayerCharacterResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatalf("client-side parse would reject our reply: %v", err)
	}
	if resp.GetPlayerId() != 999 || resp.GetCharacterId() != 7 {
		t.Errorf("ids not echoed: got %d/%d, want 999/7",
			resp.GetPlayerId(), resp.GetCharacterId())
	}
}

// TestCharacterSlotAllocationAvoidsStoredIDs covers the reason allocation
// consults the store at all.
//
// The client only volunteers the slots it knows about. If it connects from
// another console, or with a fresh local cache, allocating from that list alone
// would hand back an id already recorded for this player — silently merging two
// characters into one row.
func TestCharacterSlotAllocationAvoidsStoredIDs(t *testing.T) {
	svc, log, cs := testService(t)

	// Character 1 already exists server-side.
	if err := svc.store.SaveCharacterData(context.Background(),
		cs.playerID, 1, []byte{0x01}); err != nil {
		t.Fatal(err)
	}

	// The client claims to know about nothing, and asks for an allocation.
	raw, err := proto.Marshal(&ds2pb.RequestUpdateLoginPlayerCharacter{
		CharacterId: proto.Uint32(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleUpdateLoginPlayerCharacter(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestUpdateLoginPlayerCharacterResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetCharacterId() == 1 {
		t.Error("allocated slot 1, which the store already holds; the new character " +
			"would overwrite the existing one")
	}
}

// TestPowerStoneBoardIsKeyedByPlayerAndCharacter — character_id is the client's
// local slot number, so every player has a character 1. Keying the board on it
// alone merges them into a single entry.
func TestPowerStoneBoardIsKeyedByPlayerAndCharacter(t *testing.T) {
	svc, log, one := testService(t)
	two := &clientSession{accountID: "mgnomad2", playerID: one.playerID + 1, characterID: 1}

	submitScore(t, svc, log, one, 1, 100)
	submitScore(t, svc, log, two, 1, 500)

	page := rankingPage(t, svc, log, one, 1, 10)
	if len(page) != 2 {
		t.Fatalf("board has %d entries, want 2 — two players' character 1 must not "+
			"share a row", len(page))
	}
	if page[0].GetScore() != 500 || page[0].GetPlayerId() != two.playerID {
		t.Errorf("top entry is player %d score %d, want player %d score 500",
			page[0].GetPlayerId(), page[0].GetScore(), two.playerID)
	}
}
