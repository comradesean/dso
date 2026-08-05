package game

import (
	"context"
	"encoding/hex"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/proto/ds2pb"
	"github.com/sstreight/dso/internal/server/core"
	"github.com/sstreight/dso/internal/server/store"
)

// bootService builds a service with a real store. Login now allocates a
// persisted player id, so a storeless Service cannot get through it.
func bootService(t *testing.T) (*Service, logger) {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Service{
		srv:   &core.Server{Config: config.Default(), Logger: log},
		store: st,
	}, log
}

// capturedWaitForUserLogin is the real payload a Dark Souls 2 PS3 client
// (BLUS41045) sent as its first game-service message on 2026-08-05, immediately
// after the reliable-UDP session reached Established.
//
// Decoded:
//
//	field 1 = "comradesean"  (PSN online ID)
//	field 2 = 2, field 3 = 0, field 4 = 1, field 5 = 2
//
// Using the console's own bytes rather than a synthetic message is deliberate:
// it pins that PS3 really does use opcode 0x0386 with the PC message shape, which
// was an open question until this capture.
const capturedWaitForUserLogin = "0a0b636f6d726164657365616e1002180020012802"

func TestHandleWaitForUserLoginFromCapture(t *testing.T) {
	payload, err := hex.DecodeString(capturedWaitForUserLogin)
	if err != nil {
		t.Fatal(err)
	}

	svc, log := bootService(t)
	cs := &clientSession{}

	reply, err := svc.handleWaitForUserLogin(log, cs, payload)
	if err != nil {
		t.Fatalf("handler rejected a payload captured from a real client: %v", err)
	}

	if cs.accountID != "comradesean" {
		t.Errorf("accountID: got %q, want %q", cs.accountID, "comradesean")
	}
	if cs.playerID == 0 {
		t.Error("playerID is 0; the client keys everything downstream by this")
	}

	var resp ds2pb.RequestWaitForUserLoginResponse
	if err := proto.Unmarshal(reply, &resp); err != nil {
		t.Fatalf("reply is not a valid RequestWaitForUserLoginResponse: %v", err)
	}
	if resp.GetPsnId() != "comradesean" {
		t.Errorf("reply psn_id: got %q, want %q", resp.GetPsnId(), "comradesean")
	}
	if resp.GetPlayerId() != cs.playerID {
		t.Errorf("reply player_id: got %d, want %d", resp.GetPlayerId(), cs.playerID)
	}
}

// TestPlayerIDsAreDistinct guards against every client being handed the same id,
// which would silently break anything keyed by player.
// TestPlayerIDIsStablePerAccount replaces an earlier test that asserted repeated
// logins get *distinct* ids. That was right when ids were a per-run counter and
// is wrong now: player ids appear in blood messages, signs and the leaderboard,
// and other clients cache them, so the same account must keep the same id.
func TestPlayerIDIsStablePerAccount(t *testing.T) {
	payload, err := hex.DecodeString(capturedWaitForUserLogin)
	if err != nil {
		t.Fatal(err)
	}
	svc, log := bootService(t)

	var first uint32
	for i := 0; i < 5; i++ {
		cs := &clientSession{}
		if _, err := svc.handleWaitForUserLogin(log, cs, payload); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = cs.playerID
			if first < 100000 {
				t.Errorf("player id %d is below the reserved floor; clients cache "+
					"these across sessions and low ids collide with earlier runs", first)
			}
			continue
		}
		if cs.playerID != first {
			t.Fatalf("login %d gave player id %d, want the stable %d for the same account",
				i, cs.playerID, first)
		}
	}
}

// TestDifferentAccountsGetDifferentIDs is the other half: stability must not
// collapse two accounts onto one id.
func TestDifferentAccountsGetDifferentIDs(t *testing.T) {
	svc, log := bootService(t)

	id := func(account string) uint32 {
		raw, err := proto.Marshal(&ds2pb.RequestWaitForUserLogin{
			PsnId:     proto.String(account),
			Unknown_1: proto.Uint32(2),
			Unknown_2: proto.Uint32(0),
			Unknown_3: proto.Uint32(1),
			Unknown_4: proto.Uint32(2),
		})
		if err != nil {
			t.Fatal(err)
		}
		cs := &clientSession{}
		if _, err := svc.handleWaitForUserLogin(log, cs, raw); err != nil {
			t.Fatal(err)
		}
		return cs.playerID
	}

	a, b := id("comradesean"), id("mgnomad2")
	if a == b {
		t.Fatalf("two accounts share player id %d", a)
	}
	if again := id("comradesean"); again != a {
		t.Errorf("first account's id changed from %d to %d", a, again)
	}
}

func TestHandleWaitForUserLoginRejectsGarbage(t *testing.T) {
	svc, log := bootService(t)

	// Missing the required psn_id field.
	empty, err := proto.Marshal(&ds2pb.RequestWaitForUserLogin{})
	if err == nil {
		if _, err := svc.handleWaitForUserLogin(log, &clientSession{}, empty); err == nil {
			t.Error("handler accepted a message with no account id")
		}
	}

	if _, err := svc.handleWaitForUserLogin(log, &clientSession{}, []byte{0xff, 0xff, 0xff}); err == nil {
		t.Error("handler accepted a malformed protobuf")
	}
}
