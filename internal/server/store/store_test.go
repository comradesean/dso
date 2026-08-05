package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBloodMessageCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	body := []byte{0xde, 0xad, 0xbe, 0xef}
	id, err := s.AddBloodMessage(ctx, &BloodMessage{
		PlayerID: 1, CharacterID: 2, AccountID: "comradesean",
		AreaID: 10040000, CellID: 4290772992, Data: body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("assigned id is 0")
	}

	got, ok, err := s.GetBloodMessage(ctx, id)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got.Data, body) {
		t.Errorf("data: got %x, want %x (must round-trip verbatim)", got.Data, body)
	}
	if got.AccountID != "comradesean" {
		t.Errorf("account_id: got %q", got.AccountID)
	}

	if err := s.RemoveBloodMessage(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetBloodMessage(ctx, id); ok {
		t.Error("message still present after remove")
	}
}

func TestBloodMessageEvaluate(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	id, err := s.AddBloodMessage(ctx, &BloodMessage{
		PlayerID: 1, AreaID: 100, CellID: 1, Data: []byte{1}, AccountID: "a",
	})
	if err != nil {
		t.Fatal(err)
	}

	for want := int64(1); want <= 3; want++ {
		rating, ok, err := s.EvaluateBloodMessage(ctx, id)
		if err != nil || !ok {
			t.Fatalf("evaluate: ok=%v err=%v", ok, err)
		}
		if rating != want {
			t.Errorf("rating: got %d, want %d", rating, want)
		}
	}

	// An unknown id must report not-found rather than inventing a row.
	if _, ok, err := s.EvaluateBloodMessage(ctx, 99999); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("evaluating an unknown id reported success")
	}
}

func TestBloodMessageAreaAndCellFiltering(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	add := func(area, cell uint32) {
		if _, err := s.AddBloodMessage(ctx, &BloodMessage{
			PlayerID: 1, AreaID: area, CellID: cell, Data: []byte{1}, AccountID: "a",
		}); err != nil {
			t.Fatal(err)
		}
	}
	add(100, 1)
	add(100, 2)
	add(200, 1)

	cases := []struct {
		area  uint32
		cells []uint32
		want  int
	}{
		{100, []uint32{1}, 1},
		{100, []uint32{1, 2}, 2},
		{100, nil, 2},         // no cell filter = any cell in the area
		{100, []uint32{9}, 0}, // cell the client did not ask about
		{300, []uint32{1}, 0}, // empty area
	}
	for _, c := range cases {
		got, err := s.BloodMessagesInCells(ctx, c.area, c.cells, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.want {
			t.Errorf("area %d cells %v: got %d, want %d", c.area, c.cells, len(got), c.want)
		}
	}
}

func TestBloodMessageLimit(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		if _, err := s.AddBloodMessage(ctx, &BloodMessage{
			PlayerID: 1, AreaID: 100, CellID: 1, Data: []byte{byte(i)}, AccountID: "a",
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.BloodMessagesInCells(ctx, 100, []uint32{1}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("limit not applied: got %d, want 3", len(got))
	}
	// Newest first, so a busy area shows recent content.
	if got[0].ID < got[1].ID {
		t.Errorf("expected newest first, got ids %d then %d", got[0].ID, got[1].ID)
	}
}

// TestPersistsAcrossReopen is the point of the whole package: content must
// survive a server restart. Every restart during development wiped every placed
// message before this existed.
func TestPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "dso.db")

	s1, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte{0x01, 0x02, 0x03}
	id, err := s1.AddBloodMessage(ctx, &BloodMessage{
		PlayerID: 7, CharacterID: 1, AccountID: "comradesean",
		AreaID: 10040000, CellID: 42, Data: body,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s1.EvaluateBloodMessage(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen, as a restarted server would.
	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	got, ok, err := s2.GetBloodMessage(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("message did not survive reopen")
	}
	if !bytes.Equal(got.Data, body) {
		t.Errorf("data after reopen: got %x, want %x", got.Data, body)
	}
	if got.Rating != 1 {
		t.Errorf("rating after reopen: got %d, want 1", got.Rating)
	}
	if got.AccountID != "comradesean" {
		t.Errorf("account_id after reopen: got %q", got.AccountID)
	}

	// And it is still findable through the listing path the client uses.
	found, err := s2.BloodMessagesInCells(ctx, 10040000, []uint32{42}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("listing after reopen returned %d messages, want 1", len(found))
	}
}
