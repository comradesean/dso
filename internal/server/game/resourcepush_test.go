package game

import (
	"testing"
	"time"
)

// The login push schedule is derived from whatever is switched on, not fixed.
// That matters because the client discards all but one entry per applier pass,
// so the run has to stay correctly spaced as features come and go — and the
// order has to hold, since the chest's prize must precede its arm.
func TestLoginResourcePushesFollowConfig(t *testing.T) {
	const (
		lotFile    = "../../../data/regpush/stock/ItemLotParam2_SvrEvent.param"
		onlineFile = "../../../data/regpush/stock/OnlineEventParam.param"
	)

	tests := []struct {
		name    string
		obelisk string
		chest   []uint64
		want    []string // "feature:path", in order
	}{
		{
			name: "nothing enabled sends nothing",
			want: nil,
		},
		{
			name:    "obelisk alone",
			obelisk: "hello",
			want:    []string{"obelisk:regulation.fmg"},
		},
		{
			name:  "chest alone, prize before arm",
			chest: []uint64{5600000},
			want: []string{
				"event_chest:ItemLotParam2_SvrEvent.param",
				"event_chest:OnlineEventParam.param",
			},
		},
		{
			name:    "both, obelisk first",
			obelisk: "hello",
			chest:   []uint64{5600000},
			want: []string{
				"obelisk:regulation.fmg",
				"event_chest:ItemLotParam2_SvrEvent.param",
				"event_chest:OnlineEventParam.param",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, log, _ := testService(t)
			svc.srv.Config.ObeliskText = tc.obelisk
			svc.srv.Config.EventChestRotation = tc.chest
			svc.srv.Config.EventChestLotParamFile = lotFile
			svc.srv.Config.EventChestOnlineEventFile = onlineFile

			got := svc.loginResourcePushes(log)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d pushes, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, want := range tc.want {
				if key := got[i].feature + ":" + got[i].path; key != want {
					t.Errorf("push %d = %q, want %q", i, key, want)
				}
				if len(got[i].data) == 0 {
					t.Errorf("push %d (%s) carries no payload", i, want)
				}
			}
		})
	}
}

// A player who quits during the run must not leave a goroutine pushing into a
// session nobody is on the other end of — and a fast relog must not overlap two
// runs, which would collapse the spacing the client depends on.
func TestResourcePushRunStopsWhenSessionEnds(t *testing.T) {
	svc, log, cs := testService(t)
	// Long enough that the test would hang, not merely flake, if the abandon
	// path were broken. cs.conn is nil here, so a send would panic — which is
	// the assertion: nothing may be sent after the session ends.
	svc.srv.Config.RegulationPushDelaySeconds = 3600
	cs.done = make(chan struct{})
	close(cs.done)

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.sendResourcePushes(log, cs, []resourcePush{
			{feature: "obelisk", path: "regulation.fmg", data: []byte{1}},
			{feature: "obelisk", path: "regulation.fmg", data: []byte{2}},
		})
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("sendResourcePushes did not return after the session ended")
	}
}
