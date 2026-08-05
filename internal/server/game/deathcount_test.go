package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// notifyDeath builds a RequestNotifyDeath. Every field is `required`, so a
// partial message would not survive the handler's own proto2 parse.
func notifyDeath(t *testing.T) []byte {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestNotifyDeath{
		OnlineAreaId: proto.Uint32(100),
		CellId:       proto.Uint32(7),
		Field_3:      proto.Int64(0),
		Field_4:      proto.Int64(0),
		Field_5:      proto.Int64(0),
		Field_6:      proto.Int64(0),
		Field_7:      proto.Int64(0),
		Field_8:      []byte{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// totalDeaths runs the 0x03F0 handler and returns the count the client would see.
func totalDeaths(t *testing.T, svc *Service, log logger, cs *clientSession) uint32 {
	t.Helper()
	raw, err := svc.handleGetTotalDeathCount(log, cs, nil)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetTotalDeathCountResponse
	if err := proto.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	return resp.GetTotalDeathCount()
}

// TestTotalDeathCountEmpty pins the reply on a fresh server. total_death_count is
// `required`, so an empty message is not the same as zero — it is a message the
// client's proto2 parser rejects, and the counter times out rather than showing 0.
func TestTotalDeathCountEmpty(t *testing.T) {
	svc, log, cs := testService(t)

	raw, err := svc.handleGetTotalDeathCount(log, cs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("empty reply; total_death_count is required and must be present at zero")
	}
	var resp ds2pb.RequestGetTotalDeathCountResponse
	if err := proto.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("reply does not parse as the client would parse it: %v", err)
	}
	if got := resp.GetTotalDeathCount(); got != 0 {
		t.Fatalf("total_death_count = %d, want 0", got)
	}
}

// TestDeathCountAccumulates is the whole feature end to end: online deaths and an
// offline batch both land in the same world total.
func TestDeathCountAccumulates(t *testing.T) {
	svc, log, cs := testService(t)

	for i := 0; i < 3; i++ {
		if _, err := svc.handleNotifyDeath(log, cs, notifyDeath(t)); err != nil {
			t.Fatal(err)
		}
	}

	offline, err := proto.Marshal(&ds2pb.RequestNotifyOfflineDeathCount{
		Count: proto.Int64(12),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleNotifyOfflineDeathCount(log, cs, offline); err != nil {
		t.Fatal(err)
	}

	if got := totalDeaths(t, svc, log, cs); got != 15 {
		t.Fatalf("total_death_count = %d, want 15", got)
	}
}

// TestNotifyDeathSendsNoReply guards the three fire-and-forget opcodes. The client
// registers no response callback for them; replying puts a message on the wire it
// has nowhere to route.
func TestNotifyDeathSendsNoReply(t *testing.T) {
	svc, log, cs := testService(t)

	reply, err := svc.handleNotifyDeath(log, cs, notifyDeath(t))
	if err != nil {
		t.Fatal(err)
	}
	if reply != nil {
		t.Fatalf("handleNotifyDeath returned %d bytes; this opcode takes no reply", len(reply))
	}

	// ...and the dispatcher must still mark it handled, or it lands in the
	// "no handler" log alongside genuinely unimplemented opcodes.
	reply, handled, err := svc.handleMessage(log, cs, opRequestNotifyDeath, notifyDeath(t))
	if err != nil {
		t.Fatal(err)
	}
	if !handled || reply != nil {
		t.Fatalf("handleMessage(0x03F1) = (%d bytes, handled=%v), want (nil, true)", len(reply), handled)
	}
}

// TestNotifyKillPlayerDoesNotCount is the double-count guard.
//
// A single PvP death produces two messages from two different consoles: the
// victim's RequestNotifyDeath and the killer's RequestNotifyKillPlayer, ~55ms
// apart in our capture. Counting both would double every PvP death in the world
// total, permanently, since the counter is persisted.
func TestNotifyKillPlayerDoesNotCount(t *testing.T) {
	svc, log, cs := testService(t)

	if _, err := svc.handleNotifyDeath(log, cs, notifyDeath(t)); err != nil {
		t.Fatal(err)
	}

	kill, err := proto.Marshal(&ds2pb.RequestNotifyKillPlayer{
		Field_1: proto.Int64(14),
		Field_2: proto.Int64(290333),
		Field_3: proto.Int64(0),
		Field_4: proto.Int64(0),
		Field_5: proto.Int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := svc.handleNotifyKillPlayer(log, cs, kill)
	if err != nil {
		t.Fatal(err)
	}
	if reply != nil {
		t.Fatalf("handleNotifyKillPlayer returned %d bytes; this opcode takes no reply", len(reply))
	}

	if got := totalDeaths(t, svc, log, cs); got != 1 {
		t.Fatalf("total_death_count = %d, want 1 — the kill report must not count "+
			"the death the victim already reported", got)
	}
}

// TestOfflineDeathCountRejectsGarbage covers the one field a client fully
// controls. The count is unvalidated on the wire and the counter is persistent,
// so a single modified client could otherwise pin the world total forever.
func TestOfflineDeathCountRejectsGarbage(t *testing.T) {
	cases := []struct {
		name  string
		count int64
		want  uint32
	}{
		{"negative", -5000, 0},
		{"zero", 0, 0},
		{"absurd", 1 << 40, maxOfflineDeathBatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, log, cs := testService(t)
			raw, err := proto.Marshal(&ds2pb.RequestNotifyOfflineDeathCount{
				Count: proto.Int64(tc.count),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.handleNotifyOfflineDeathCount(log, cs, raw); err != nil {
				t.Fatal(err)
			}
			if got := totalDeaths(t, svc, log, cs); got != tc.want {
				t.Fatalf("total_death_count = %d, want %d", got, tc.want)
			}
		})
	}
}
