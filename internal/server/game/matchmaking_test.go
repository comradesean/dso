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

	// Covenant deliberately NOT Bell Keepers with a seal here: since defenders
	// are no longer gated on position, a sealed Bell Keeper standing in a rat
	// area matches the Bell Keeper pool first and this would stop exercising the
	// rat path it exists to test.
	full, err := proto.Marshal(&ds2datapb.AllStatus{
		PlayerStatus: &ds2datapb.PlayerStatus{
			Covenant:   u(covenantHeirsOfTheSun),
			SoulMemory: u(5_634_765),
		},
		PlayerLocation: &ds2datapb.PlayerLocation{OnlineActivityAreaId: u(103410)},
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
	if p.effectiveCovenant() != covenantHeirsOfTheSun {
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

// TestChangedFieldsReportsOnlyMovement — this drives a diagnostic log used to
// hunt gates we do not model, so a diff that reports noise (or misses a change)
// costs a live test cycle to discover.
func TestChangedFieldsReportsOnlyMovement(t *testing.T) {
	before := map[string]uint32{"a": 1, "b": 2, "c": 3}
	after := map[string]uint32{"a": 1, "b": 99, "d": 7}

	got := changedFields(before, after)
	want := []string{"b=2->99", "d=+7"}
	if len(got) != len(want) {
		t.Fatalf("changedFields = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("changedFields = %v, want %v", got, want)
			break
		}
	}

	if n := len(changedFields(before, before)); n != 0 {
		t.Errorf("identical maps produced %d changes, want none — a quiet "+
			"client must stay quiet in the log", n)
	}
}

// TestStatusFieldsSurviveDeltas — the diagnostic mirror has to merge like
// everything else, or every partial update would look like a mass change.
func TestStatusFieldsSurviveDeltas(t *testing.T) {
	u := func(v uint32) *uint32 { return &v }

	full, err := proto.Marshal(&ds2datapb.AllStatus{
		PlayerStatus:  &ds2datapb.PlayerStatus{Unknown_4: u(1), Unknown_5: u(2)},
		ItemUsingInfo: &ds2datapb.ItemUsingInfo{UsingDriedFingers: u(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	delta, err := proto.Marshal(&ds2datapb.AllStatus{
		PlayerStatus: &ds2datapb.PlayerStatus{Unknown_5: u(9)},
	})
	if err != nil {
		t.Fatal(err)
	}

	var p matchProfile
	p.applyStatus(full)
	snapshot := make(map[string]uint32, len(p.statusFields))
	for k, v := range p.statusFields {
		snapshot[k] = v
	}

	p.applyStatus(delta)
	if got := p.statusFields["ps.unknown_4"]; got != 1 {
		t.Errorf("delta wiped ps.unknown_4: got %d, want 1", got)
	}
	// Presence, not value: dried_fingers is legitimately 0 here, so a plain
	// lookup could not tell "recorded as 0" from "dropped by the delta".
	if got, ok := p.statusFields["item.dried_fingers"]; !ok || got != 0 {
		t.Errorf("delta lost item.dried_fingers (present=%v value=%d)", ok, got)
	}

	changed := changedFields(snapshot, p.statusFields)
	if len(changed) != 1 || changed[0] != "ps.unknown_5=2->9" {
		t.Errorf("changed = %v, want exactly [ps.unknown_5=2->9]", changed)
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

// TestBellKeeperDefenderNeedsNoBelfry pins the half of the rule that was
// backwards, and which made the covenant unsummonable.
//
// The defender is summoned TO the trespasser, so they may be anywhere. Gating
// them on standing in a belfry — which is what the reference server does, and
// what this code did — meant a defender in any ordinary area was never in the
// pool, so every poll reported skipped_wrong_pool and nothing ever matched.
// Confirmed in game: the covenant icon glows far from either belfry.
func TestBellKeeperDefenderNeedsNoBelfry(t *testing.T) {
	// Somewhere entirely unrelated to a belfry.
	away := matchProfile{
		received: true, covenant: cov(covenantBellKeepers), bellKeepersSeal: true,
		onlineActivityArea: 102310,
	}
	if got := visitorPoolFor(away); got != ds2pb.VisitorType_VisitorType_BellKeepers {
		t.Errorf("defender away from a belfry: pool = %v, want BellKeepers", got)
	}

	// Still nothing while not in an online activity area at all.
	away.onlineActivityArea = 0
	if got := visitorPoolFor(away); got != ds2pb.VisitorType_VisitorType_None {
		t.Errorf("defender with no activity area: pool = %v, want None", got)
	}
}

// TestBellKeeperTrespasserMustBeInABelfry is the other half: the requester is
// the one being invaded, and Bell Keepers defend Luna and Sol specifically.
func TestBellKeeperTrespasserMustBeInABelfry(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	inBellKeeperPool(host)

	// Requester is nowhere near a belfry — expect no targets even though a
	// perfectly good defender is available.
	summoner.profile = matchProfile{received: true, onlineActivityArea: 102310}
	replyRaw, err := svc.handleGetVisitorList(log, summoner, visitorListRequest(t, 5))
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetVisitorListResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if n := len(resp.GetTargetData()); n != 0 {
		t.Errorf("summoned %d defenders from outside a belfry, want 0", n)
	}

	// Same request from inside Belfry Luna finds the defender.
	summoner.profile.onlineActivityArea = 101640
	replyRaw, err = svc.handleGetVisitorList(log, summoner, visitorListRequest(t, 5))
	if err != nil {
		t.Fatal(err)
	}
	resp.Reset()
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if n := len(resp.GetTargetData()); n != 1 {
		t.Errorf("from inside the belfry got %d defenders, want 1", n)
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
