package game

import "testing"

// TestBellReachesRegion pins the thing that breaks visibly when it is wrong.
//
// The client cannot tell which bell rang — the push sets a latch and whichever
// belfry map is loaded plays its own bell. So delivering a Luna toll to someone
// in Belfry Sol makes them hear SOL.
//
// RE-CONFIRMED 2026-08-08 by direct experiment: an unfiltered synthetic toll
// claiming map 10160000 was sent to a client that walked from Belfry Luna into
// Belfry Sol (cell 101910), and it rang there too. The client does not mute on
// the map id, so this filter is load-bearing, not an optimisation.
func TestBellReachesRegion(t *testing.T) {
	const luna, sol, lostBastille = 10160000, 10190000, 10140000

	cases := []struct {
		name          string
		ringMap, area uint32
		want          bool
	}{
		{"luna reaches itself", luna, luna, true},
		// Lost Bastille (10140000) was in Luna's region until 2026-08-08 and is
		// NOT any more. DO NOT PUT IT BACK without new evidence.
		//
		// The old case cited "confirmed on the wire", but the only observation
		// behind it was a SYNTHETIC toll heard by a player in the Lost Bastille —
		// and the synthetic path sends to everyone regardless of location, so it
		// showed only that the map's script plays a bell, never that the toll
		// should have been targeted there. The PC captures do not support it
		// either: the listener who heard Luna tolls from "the Lost Bastille" was
		// at the Servants' Quarters bonfire, which reports 10160000 — the SAME
		// map as the bell. No client was ever in 10140000.
		{"luna must NOT reach 10140000 — nothing supports it", luna, lostBastille, false},
		{"luna must NOT reach belfry sol — would ring the wrong bell", luna, sol, false},
		{"sol reaches itself", sol, sol, true},
		{"sol must NOT reach belfry luna", sol, luna, false},
		{"unknown area is not sent to", luna, 0, false},
		{"unrelated area is not sent to", luna, 10320000, false},
	}
	for _, c := range cases {
		if got := bellReaches(c.ringMap, c.area); got != c.want {
			t.Errorf("%s: bellReaches(%d, %d) = %v, want %v", c.name, c.ringMap, c.area, got, c.want)
		}
	}
}

// TestBellRegionOverride — region membership is incomplete, so it must be
// fillable without a rebuild.
//
// (Iron Keep's map id IS established — it is 10190000, the same id that carries
// Belfry Sol, confirmed by a player at the Threshold Bridge bonfire reporting it.
// An earlier note here said otherwise. What remains incomplete is how far each
// belfry's region extends beyond its own map, which is untested in both
// directions.)
func TestBellRegionOverride(t *testing.T) {
	t.Setenv("DSO_BELL_REGION_10190000", "10190000, 10170000")
	if !bellReaches(10190000, 10170000) {
		t.Error("override did not extend Belfry Sol's region")
	}
	if bellReaches(10190000, 10160000) {
		t.Error("override must not widen beyond what it lists")
	}
}
