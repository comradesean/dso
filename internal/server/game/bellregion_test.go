package game

import "testing"

// TestBellReachesRegion pins the thing that breaks visibly when it is wrong.
//
// The client cannot tell which bell rang — the push sets a latch and whichever
// belfry map is loaded plays its own bell. So delivering a Luna toll to someone
// in Belfry Sol makes them hear SOL. Confirmed live on PS3 when the filter was
// off. These cases are the guard against reintroducing that.
func TestBellReachesRegion(t *testing.T) {
	const luna, sol, lostBastille = 10160000, 10190000, 10140000

	cases := []struct {
		name          string
		ringMap, area uint32
		want          bool
	}{
		{"luna reaches itself", luna, luna, true},
		{"luna reaches lost bastille (confirmed on the wire)", luna, lostBastille, true},
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

// TestBellRegionOverride — region membership is incomplete (Iron Keep's map id is
// not established), so it must be fillable without a rebuild.
func TestBellRegionOverride(t *testing.T) {
	t.Setenv("DSO_BELL_REGION_10190000", "10190000, 10170000")
	if !bellReaches(10190000, 10170000) {
		t.Error("override did not extend Belfry Sol's region")
	}
	if bellReaches(10190000, 10160000) {
		t.Error("override must not widen beyond what it lists")
	}
}
