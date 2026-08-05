package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2datapb"
	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// TestProfileFromStatusRoundTrip proves we read the AllStatus blob the way the
// client writes it. Every field here drives a filter decision, so a silently
// wrong field number would show up as "matchmaking mysteriously offers nobody".
func TestProfileFromStatusRoundTrip(t *testing.T) {
	u := func(v uint32) *uint32 { return &v }
	blob, err := proto.Marshal(&ds2datapb.AllStatus{
		PlayerStatus: &ds2datapb.PlayerStatus{
			Covenant:         u(covenantRatKing),
			SoulMemory:       u(1_234_567),
			SoulLevel:        u(120),
			SittingAtBonfire: u(1),
		},
		PlayerLocation: &ds2datapb.PlayerLocation{
			OnlineActivityAreaId: u(103410),
		},
		ItemUsingInfo: &ds2datapb.ItemUsingInfo{
			CrestOfTheRat: u(1),
			NamedRingGod:  u(7),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	p := profileFromStatus(blob)
	if !p.received {
		t.Fatal("profile not marked received")
	}
	if p.covenant != covenantRatKing || p.soulMemory != 1_234_567 || p.soulLevel != 120 {
		t.Errorf("player status fields wrong: %+v", p)
	}
	if p.onlineActivityArea != 103410 {
		t.Errorf("activity area = %d, want 103410", p.onlineActivityArea)
	}
	if !p.sittingAtBonfire || !p.crestOfTheRat || p.nameEngravedRing != 7 {
		t.Errorf("flags wrong: %+v", p)
	}
}

// TestUnparseableStatusIsUnmatchableNotFatal — the blob is client-supplied, so a
// malformed one must degrade to "we know nothing", never take the session down.
func TestUnparseableStatusIsUnmatchableNotFatal(t *testing.T) {
	p := profileFromStatus([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	if p.received {
		t.Error("garbage blob produced a profile we would match on")
	}
	if got := visitorPoolFor(p); got != ds2pb.VisitorType_VisitorType_None {
		t.Errorf("pool = %v, want None", got)
	}
}

// TestRatPoolIsInverted is the rule that is easiest to get backwards, and did
// mislead this project: you are rat-summonable because you are NOT a rat.
func TestRatPoolIsInverted(t *testing.T) {
	prey := matchProfile{received: true, covenant: covenantBellKeepers, onlineActivityArea: 103410}
	if got := visitorPoolFor(prey); got != ds2pb.VisitorType_VisitorType_Rat {
		t.Errorf("non-rat in a rat area: pool = %v, want Rat", got)
	}

	rat := matchProfile{received: true, covenant: covenantRatKing, onlineActivityArea: 103410}
	if got := visitorPoolFor(rat); got == ds2pb.VisitorType_VisitorType_Rat {
		t.Error("a rat was placed in its own prey pool; the covenant text says " +
			"members are never pulled into other worlds")
	}
}

// TestBonfireLeavesEveryPool pins the behaviour that explains the live failure:
// activity area 0 means unavailable regardless of covenant or equipment.
func TestBonfireLeavesEveryPool(t *testing.T) {
	atFire := matchProfile{
		received: true, covenant: covenantBellKeepers, bellKeepersSeal: true,
		onlineActivityArea: 0, sittingAtBonfire: true,
	}
	if got := visitorPoolFor(atFire); got != ds2pb.VisitorType_VisitorType_None {
		t.Errorf("pool at bonfire = %v, want None", got)
	}

	// The same player, having walked into the belfry, is available.
	walked := atFire
	walked.onlineActivityArea = 101640
	walked.sittingAtBonfire = false
	if got := visitorPoolFor(walked); got != ds2pb.VisitorType_VisitorType_BellKeepers {
		t.Errorf("pool in belfry = %v, want BellKeepers", got)
	}
}

// TestBellKeeperNeedsTheSeal — covenant membership alone is not enough.
func TestBellKeeperNeedsTheSeal(t *testing.T) {
	noSeal := matchProfile{
		received: true, covenant: covenantBellKeepers, onlineActivityArea: 101640,
	}
	if got := visitorPoolFor(noSeal); got == ds2pb.VisitorType_VisitorType_BellKeepers {
		t.Error("bell keeper without the seal equipped was offered")
	}
}

func TestSoulMemoryTierBoundaries(t *testing.T) {
	cases := []struct {
		sm   uint32
		tier int
	}{
		{0, 0}, {9_999, 0}, {10_000, 1}, {19_999, 1}, {20_000, 2},
		{100_000, 7}, {999_999_999, 43}, {4_000_000_000, 43},
	}
	for _, c := range cases {
		if got := soulMemoryTier(c.sm); got != c.tier {
			t.Errorf("soulMemoryTier(%d) = %d, want %d", c.sm, got, c.tier)
		}
	}
}

// TestRatTierWindow encodes the in-game covenant text verbatim: "up to 1 Soul
// Memory tier lower or 3 tiers higher than your own". This window is the one
// piece of matchmaking policy we have authoritative wording for, so it is worth
// pinning against a future refactor that "tidies" the windows to symmetric.
func TestRatTierWindow(t *testing.T) {
	w := visitorTierWindows[ds2pb.VisitorType_VisitorType_Rat]
	if w.below != 1 || w.above != 3 {
		t.Fatalf("rat window = %+v, want {below:1 above:3}", w)
	}

	// Host at tier 7 (soul memory 100000).
	const host = uint32(100_000)

	if !soulMemoryMatches(host, 85_000, w) {
		t.Error("one tier below should match")
	}
	if !soulMemoryMatches(host, 100_000, w) {
		t.Error("same tier should match")
	}
	// Three tiers above tier 7 is tier 10, whose band ends at 179_999.
	if !soulMemoryMatches(host, 179_999, w) {
		t.Error("three tiers above should match")
	}
	// Four above (tier 11) must not.
	if soulMemoryMatches(host, 209_999, w) {
		t.Error("four tiers above should not match")
	}
	// Two below (tier 5) must not.
	if soulMemoryMatches(host, 69_999, w) {
		t.Error("two tiers below should not match")
	}
}

// TestVisitorListFiltersByPool is the end-to-end version: an ineligible target
// must not be returned, because returning one produces the 20-second refusal
// loop that motivated this whole file.
func TestVisitorListFiltersByPool(t *testing.T) {
	svc, log, host, summoner := signTestService(t)

	// Host is in the rat area but the request asks for Bell Keepers.
	host.profile = matchProfile{
		received: true, covenant: covenantHeirsOfTheSun, onlineActivityArea: 103410,
		soulMemory: 100_000,
	}

	replyRaw, err := svc.handleGetVisitorList(log, summoner, visitorListRequest(t, 5))
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetVisitorListResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.GetTargetData()) != 0 {
		t.Errorf("offered %d targets from the wrong pool, want 0",
			len(resp.GetTargetData()))
	}
}

// TestVisitorListFiltersBySoulMemory — right pool, wrong band.
func TestVisitorListFiltersBySoulMemory(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	inBellKeeperPool(host)
	host.profile.soulMemory = 30_000_000 // far outside the requester's tier

	replyRaw, err := svc.handleGetVisitorList(log, summoner, visitorListRequest(t, 5))
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetVisitorListResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.GetTargetData()) != 0 {
		t.Errorf("offered %d out-of-band targets, want 0", len(resp.GetTargetData()))
	}
}
