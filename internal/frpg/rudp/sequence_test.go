package rudp

import "testing"

// TestSequenceComparisonAcrossWrap is a regression test for a bug that killed
// live sessions.
//
// The old comparison only accepted a wrapped ack when the current value sat in
// the top quarter of the sequence space. A single lost ack near the boundary left
// it stuck: the wrapped ack was discarded, every packet then looked
// unacknowledged, and the session died on max retransmits ~32s later while the
// client was still sending normally.
func TestSequenceComparisonAcrossWrap(t *testing.T) {
	t.Run("newer", func(t *testing.T) {
		for _, tc := range []struct {
			a, b uint32
			want bool
			why  string
		}{
			{5, 4, true, "ordinary increment"},
			{4, 5, false, "ordinary decrement"},
			{5, 5, false, "equal is not newer"},
			{5, 4095, true, "wrapped forward past the end"},
			{4095, 5, false, "the value we wrapped from is older"},
			// The case the old code got wrong: acked stuck mid-range because an
			// ack was lost, then a wrapped ack arrives. 3000 -> 5 is a forward
			// distance of 1101, well under half the space, so it must be accepted.
			// The old quarter-based test rejected it and doomed the session.
			{5, 3000, true, "a wrapped ack must be accepted over a stale mid-range one"},
			// Beyond half the space is a wrap backwards, i.e. older.
			{5, 2000, false, "2000 -> 5 is more than half the space; treat as older"},
			{3000, 2999, true, "mid-range increment"},
			{0, 4095, true, "exact wrap boundary"},
		} {
			if got := seqNewer(tc.a, tc.b); got != tc.want {
				t.Errorf("seqNewer(%d, %d) = %v, want %v (%s)", tc.a, tc.b, got, tc.want, tc.why)
			}
		}
	})

	t.Run("at or before", func(t *testing.T) {
		for _, tc := range []struct {
			a, b uint32
			want bool
			why  string
		}{
			{4, 5, true, "an ack for 5 covers 4"},
			{5, 5, true, "an ack covers itself"},
			{6, 5, false, "an ack for 5 does not cover 6"},
			{4095, 5, true, "an ack for 5 covers 4095 across the wrap"},
			{4090, 3, true, "covers a run that spans the wrap"},
			{5, 4095, false, "an ack for 4095 does not cover a post-wrap 5"},
		} {
			if got := seqAtOrBefore(tc.a, tc.b); got != tc.want {
				t.Errorf("seqAtOrBefore(%d, %d) = %v, want %v (%s)", tc.a, tc.b, got, tc.want, tc.why)
			}
		}
	})
}

// TestAckSurvivesALostAckAcrossTheWrap reproduces the exact failure: the ack
// index sits mid-range because an ack was lost, and the next one observed is past
// the wrap. Under the old logic this was dropped and the session was doomed.
func TestAckSurvivesALostAckAcrossTheWrap(t *testing.T) {
	acked := uint32(3000) // we missed the acks for 3001..4095

	// The peer, having wrapped, now acknowledges 5.
	if !seqNewer(5, acked) {
		// Deliberately not the assertion — see below. 3000 -> 5 is a forward
		// distance of 1101, under half the space, so it IS newer.
		t.Fatalf("a wrapped ack of 5 must be accepted over a stale 3000")
	}
	acked = 5

	// Everything the peer acknowledged before the wrap must now count as covered.
	for _, local := range []uint32{2999, 3000, 3500, 4095, 0, 5} {
		if !seqAtOrBefore(local, acked) {
			t.Errorf("packet %d still looks unacknowledged after an ack of %d; "+
				"this is what caused the endless retransmit", local, acked)
		}
	}
	// A packet genuinely still in flight must not be considered acked.
	if seqAtOrBefore(6, acked) {
		t.Error("packet 6 counted as acknowledged by an ack of 5")
	}
}

// TestFullWrapIsMonotonic walks the entire sequence space to make sure every
// step is seen as an advance, with no discontinuity at the boundary.
func TestFullWrapIsMonotonic(t *testing.T) {
	cur := uint32(0)
	for i := 0; i < maxAckValue*2; i++ {
		next := (cur + 1) % maxAckValue
		if !seqNewer(next, cur) {
			t.Fatalf("step %d -> %d not treated as an advance (iteration %d)", cur, next, i)
		}
		if !seqAtOrBefore(cur, next) {
			t.Fatalf("ack of %d does not cover %d (iteration %d)", next, cur, i)
		}
		cur = next
	}
}
