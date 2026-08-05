package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2datapb"
	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// cov builds the pointer a matchProfile literal needs. covenant is a pointer so
// that "in no covenant" (0) cannot be confused with "not told yet".
func cov(v uint32) *uint32 { return &v }

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

	var p matchProfile
	p.applyStatus(blob)
	if !p.received {
		t.Fatal("profile not marked received")
	}
	if p.effectiveCovenant() != covenantRatKing || p.soulMemory != 1_234_567 || p.soulLevel != 120 {
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
	var p matchProfile
	p.applyStatus([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	if p.received {
		t.Error("garbage blob produced a profile we would match on")
	}
	if got := visitorPoolFor(p); got != ds2pb.VisitorType_VisitorType_None {
		t.Errorf("pool = %v, want None", got)
	}
}

// TestPartialStatusDoesNotWipeProfile is a regression test for the bug that made
// matchmaking look far too strict on its first live run.
//
// The client sends an occasional full status blob (~1336 bytes observed) and a
// steady stream of small deltas (28-52 bytes). Rebuilding the profile from each
// message meant every delta blanked whatever it did not mention, so a player
// flickered in and out of their visitor pool several times a minute and was
// essentially never offered to anyone.
func TestPartialStatusDoesNotWipeProfile(t *testing.T) {
	u := func(v uint32) *uint32 { return &v }

	full, err := proto.Marshal(&ds2datapb.AllStatus{
		PlayerStatus: &ds2datapb.PlayerStatus{
			Covenant:   u(covenantBellKeepers),
			SoulMemory: u(5_634_765),
		},
		PlayerLocation: &ds2datapb.PlayerLocation{OnlineActivityAreaId: u(103410)},
		ItemUsingInfo:  &ds2datapb.ItemUsingInfo{BellKeepersSeal: u(1)},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A delta carrying nothing but a bonfire flag — the shape that caused the bug.
	delta, err := proto.Marshal(&ds2datapb.AllStatus{
		PlayerStatus: &ds2datapb.PlayerStatus{SittingAtBonfire: u(0)},
	})
	if err != nil {
		t.Fatal(err)
	}

	var p matchProfile
	p.applyStatus(full)
	if got := visitorPoolFor(p); got != ds2pb.VisitorType_VisitorType_Rat {
		t.Fatalf("after full status, pool = %v, want Rat", got)
	}

	p.applyStatus(delta)
	if p.onlineActivityArea != 103410 {
		t.Errorf("delta wiped activity area: %d, want 103410", p.onlineActivityArea)
	}
	if p.soulMemory != 5_634_765 {
		t.Errorf("delta wiped soul memory: %d", p.soulMemory)
	}
	if p.effectiveCovenant() != covenantBellKeepers {
		t.Errorf("delta wiped covenant: %d", p.effectiveCovenant())
	}
	if got := visitorPoolFor(p); got != ds2pb.VisitorType_VisitorType_Rat {
		t.Errorf("delta knocked the player out of their pool: %v", got)
	}
}

// TestMatchingParameterFillsSoulMemoryGap — a player whose status blob has not
// yet carried soul memory would otherwise sit in tier 0 and match nobody.
func TestMatchingParameterFillsSoulMemoryGap(t *testing.T) {
	var p matchProfile
	p.applyMatchingParameter(testMatchingParameter())
	if got := p.effectiveSoulMemory(); got != 100_000 {
		t.Errorf("effective soul memory = %d, want 100000 from MatchingParameter", got)
	}

	// Once the status blob supplies one, it wins.
	u := func(v uint32) *uint32 { return &v }
	blob, err := proto.Marshal(&ds2datapb.AllStatus{
		PlayerStatus: &ds2datapb.PlayerStatus{SoulMemory: u(2_000_000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	p.applyStatus(blob)
	if got := p.effectiveSoulMemory(); got != 2_000_000 {
		t.Errorf("effective soul memory = %d, want the status blob's 2000000", got)
	}
}

// TestRatPoolIsInverted is the rule that is easiest to get backwards, and did
// mislead this project: you are rat-summonable because you are NOT a rat.
func TestRatPoolIsInverted(t *testing.T) {
	prey := matchProfile{received: true, covenant: cov(covenantBellKeepers), onlineActivityArea: 103410}
	if got := visitorPoolFor(prey); got != ds2pb.VisitorType_VisitorType_Rat {
		t.Errorf("non-rat in a rat area: pool = %v, want Rat", got)
	}

	rat := matchProfile{received: true, covenant: cov(covenantRatKing), onlineActivityArea: 103410}
	if got := visitorPoolFor(rat); got == ds2pb.VisitorType_VisitorType_Rat {
		t.Error("a rat was placed in its own prey pool; the covenant text says " +
			"members are never pulled into other worlds")
	}
}

// TestBonfireLeavesEveryPool pins the behaviour that explains the live failure:
// activity area 0 means unavailable regardless of covenant or equipment.
func TestBonfireLeavesEveryPool(t *testing.T) {
	atFire := matchProfile{
		received: true, covenant: cov(covenantBellKeepers), bellKeepersSeal: true,
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
		received: true, covenant: cov(covenantBellKeepers), onlineActivityArea: 101640,
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
		{100_000, 7},
		// The 359,999,999 boundary is the medium-confidence one; pinning both
		// sides of it makes an accidental removal a test failure rather than a
		// silent widening of matching for very high soul memory.
		{44_999_999, 42}, {45_000_000, 43}, {359_999_999, 43},
		{360_000_000, 44}, {999_999_999, 44}, {4_000_000_000, 44},
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
		received: true, covenant: cov(covenantHeirsOfTheSun), onlineActivityArea: 103410,
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
