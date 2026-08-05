package game

import (
	"encoding/hex"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

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

	svc := &Service{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
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
func TestPlayerIDsAreDistinct(t *testing.T) {
	payload, err := hex.DecodeString(capturedWaitForUserLogin)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	seen := map[uint32]bool{}
	for i := 0; i < 5; i++ {
		cs := &clientSession{}
		if _, err := svc.handleWaitForUserLogin(log, cs, payload); err != nil {
			t.Fatal(err)
		}
		if seen[cs.playerID] {
			t.Fatalf("player id %d handed out twice", cs.playerID)
		}
		seen[cs.playerID] = true
	}
}

func TestHandleWaitForUserLoginRejectsGarbage(t *testing.T) {
	svc := &Service{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

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
