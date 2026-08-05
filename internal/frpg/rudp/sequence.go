package rudp

// Sequence numbers are 12 bits and wrap at maxAckValue, so they must be compared
// modularly. Plain `<` and `>` are wrong across a wrap, and the quarter-based
// special case they replaced had a hole that killed live sessions:
//
//	if acked > topQuarter && incoming < bottomQuarter { accept the wrap }
//	else if incoming > acked { accept }
//
// If a single ack was lost near the boundary — say acked sat at 3000 and the next
// ack observed was 5 — neither arm fired. The ack was discarded, every subsequent
// packet looked unacknowledged, and the session died on max retransmits about 32
// seconds later while the client was still happily sending. With only 4096
// sequence numbers and fragmented 1.5 KB uploads, wraps are frequent.
//
// The standard fix is to treat a forward distance of less than half the sequence
// space as "newer" and anything else as a wrap.

// seqDelta is the forward distance from a to b: how many increments take a to b.
func seqDelta(a, b uint32) uint32 {
	return (b - a) % maxAckValue
}

// seqNewer reports whether a is strictly ahead of b.
func seqNewer(a, b uint32) bool {
	d := seqDelta(b, a)
	return d != 0 && d < maxAckValue/2
}

// seqAtOrBefore reports whether a is at or behind b — i.e. whether an ack for b
// also covers a.
func seqAtOrBefore(a, b uint32) bool {
	return seqDelta(a, b) < maxAckValue/2
}
