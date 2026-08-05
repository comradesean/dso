package game

import (
	"os"
	"strconv"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2datapb"
	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Matchmaking filters.
//
// Until now every list request returned every online player, because the only
// thing the server knew about anyone was their id. The data to do better has
// been arriving all along in RequestUpdatePlayerStatus (0x03B8): the AllStatus
// blob is ordinary protobuf, not an opaque payload, and it carries soul memory,
// covenant, current activity area and the covenant seals. We simply never parsed
// it. matchProfile is that blob reduced to the fields matching actually needs.
//
// WHY THIS IS WORTH DOING, from a live session on 2026-08-05: a Rat King summon
// was refused 15 times in a row and then succeeded, with no change on our side.
// The client polls on a fixed ~20.5s timer for as long as the crest is equipped,
// and because we returned the same ineligible target every poll, the player saw
// "summoning failed" every 20 seconds for five minutes. The client is the final
// authority on whether it can be summoned and correctly refused each time. Our
// job is to not offer targets that are going to refuse.

// matchProfile is what the server knows about a player for matching purposes.
// The zero value means "no status received yet", which every filter treats as
// unmatchable rather than as a wildcard — see profileKnown.
type matchProfile struct {
	// received distinguishes a genuinely empty profile from an absent one.
	received bool

	soulMemory uint32
	soulLevel  uint32

	// covenant is a pointer rather than a value plus a "known" flag because
	// covenant 0 is a real state — belonging to none — and is indistinguishable
	// from "not told yet" in a plain uint32. A struct literal that sets a
	// covenant cannot then silently read back as 0.
	covenant *uint32

	// onlineActivityArea is the cell the player is doing online activity in, and
	// is 0 when they are not eligible for any of it — resting at a bonfire, in a
	// loading screen, or simply somewhere the game does not host sessions. This
	// single field is what made a rat summon fail 15 times and then succeed: it
	// was 0 while the target sat at a bonfire and became the rat cell when they
	// walked away.
	onlineActivityArea uint32

	sittingAtBonfire bool

	// Covenant items. A covenant alone is not enough — the seal or crest has to
	// be equipped for the player to take part.
	guardiansSeal   bool
	bellKeepersSeal bool
	crestOfTheRat   bool

	nameEngravedRing   uint32
	disableCrossRegion bool

	// Mirrors of the same two values from MatchingParameter, used only until the
	// status blob supplies them. Kept separate rather than merged so the source
	// of a matching decision stays visible.
	mpSoulMemory uint32
	mpCovenant   uint32
	mpSeen       bool
}

// applyStatus MERGES an AllStatus blob into the profile. It must never replace
// it wholesale.
//
// THE CLIENT SENDS PARTIAL UPDATES. Measured live: an occasional full blob of
// ~1336 bytes, and a steady stream of 28-52 byte deltas carrying only what
// changed. An earlier version of this rebuilt the profile from each message, so
// every delta that omitted PlayerLocation reset the activity area to 0 and every
// one that omitted PlayerStatus reset covenant and soul memory. The visible
// effect was a player flickering in and out of their visitor pool several times
// a minute and never being offered to anyone — which looked exactly like the
// filter being too strict, rather than the filter being fed zeroes.
//
// Merging is per FIELD, not per sub-message, because the sub-messages are
// partial too: a PlayerStatus carrying only sitting_at_bonfire must not blank
// soul memory. proto2 optional fields carry explicit presence, so a nil pointer
// means "unchanged" and is distinguishable from a real zero. That distinction is
// the whole mechanism here, and is why these read the pointers directly instead
// of the Get accessors, which cannot tell the two apart.
//
// A failure to parse is not an error: the blob is client-supplied and a
// malformed one should leave the profile untouched, never drop the session.
func (p *matchProfile) applyStatus(blob []byte) {
	var all ds2datapb.AllStatus
	if err := proto.Unmarshal(blob, &all); err != nil {
		return
	}
	p.received = true

	if st := all.GetPlayerStatus(); st != nil {
		if st.SoulMemory != nil {
			p.soulMemory = *st.SoulMemory
		}
		if st.SoulLevel != nil {
			p.soulLevel = *st.SoulLevel
		}
		if st.Covenant != nil {
			cov := *st.Covenant
			p.covenant = &cov
		}
		if st.SittingAtBonfire != nil {
			p.sittingAtBonfire = *st.SittingAtBonfire != 0
		}
		if st.DisableCrossRegionPlay != nil {
			p.disableCrossRegion = *st.DisableCrossRegionPlay != 0
		}
	}
	if loc := all.GetPlayerLocation(); loc != nil {
		if loc.OnlineActivityAreaId != nil {
			p.onlineActivityArea = *loc.OnlineActivityAreaId
		}
	}
	if it := all.GetItemUsingInfo(); it != nil {
		if it.GuardiansSeal != nil {
			p.guardiansSeal = *it.GuardiansSeal != 0
		}
		if it.BellKeepersSeal != nil {
			p.bellKeepersSeal = *it.BellKeepersSeal != 0
		}
		if it.CrestOfTheRat != nil {
			p.crestOfTheRat = *it.CrestOfTheRat != 0
		}
		if it.NamedRingGod != nil {
			p.nameEngravedRing = *it.NamedRingGod
		}
	}
}

// applyMatchingParameter records the matching view the client sends with every
// list request. Each field is `required`, so this is complete every time —
// unlike the status blob, which arrives in fragments.
//
// This exists because a player who has just connected, or who has only sent
// deltas so far, would otherwise have a soul memory of 0 and sit in tier 0,
// matching nobody. It fills that gap immediately and is also a useful check on
// the status blob: the two should agree, and a disagreement is worth seeing.
func (p *matchProfile) applyMatchingParameter(mp *ds2pb.MatchingParameter) {
	if mp == nil {
		return
	}
	p.received = true
	p.mpSoulMemory = mp.GetSoulMemory()
	p.mpCovenant = mp.GetCovenant()
	p.mpSeen = true
}

// effectiveSoulMemory prefers the status blob and falls back to the client's own
// MatchingParameter.
func (p matchProfile) effectiveSoulMemory() uint32 {
	if p.soulMemory != 0 {
		return p.soulMemory
	}
	return p.mpSoulMemory
}

// effectiveCovenant does the same, using presence rather than non-zero: covenant
// 0 is the legitimate value for belonging to no covenant.
func (p matchProfile) effectiveCovenant() uint32 {
	if p.covenant != nil {
		return *p.covenant
	}
	return p.mpCovenant
}

// DS2 covenant ids as they appear in PlayerStatus.covenant.
//
// Rat King (5) and Bell Keepers (6) are CONFIRMED on the wire — both were read
// off live status blobs on 2026-08-05 for players known to be in those covenants.
// The rest of the list is unverified and would only show up as a covenant-gated
// pool never activating.
const (
	covenantNone               uint32 = 0
	covenantHeirsOfTheSun      uint32 = 1
	covenantBlueSentinels      uint32 = 2
	covenantBrotherhood        uint32 = 3
	covenantWayOfTheBlue       uint32 = 4
	covenantRatKing            uint32 = 5
	covenantBellKeepers        uint32 = 6
	covenantDragonRemnants     uint32 = 7
	covenantCompanyOfChampions uint32 = 8
	covenantPilgrimsOfDark     uint32 = 9
)

// Cells that host each covenant's auto-summon.
//
// These are online_activity_area_id values, NOT online_area_id — the activity
// area is a cell-level id. Belfry Luna and Doors of Pharros are corroborated by
// live captures (101640 appears in our own visitor traffic, and 103410 is the
// cell every rat poll carried). The others are reference-derived and unverified,
// so an unrecognised cell is logged rather than silently dropped.
var (
	bellKeeperCells = map[uint32]bool{
		101640: true, // Belfry Luna
		101950: true, // Belfry Sol
	}
	ratCells = map[uint32]bool{
		// Both CONFIRMED LIVE on 2026-08-05, each read straight off the wire.
		103410: true, // Grave of Saints    (m10_34) -- summons confirmed working
		103320: true, // Doors of Pharros   (m10_33) -- captured on entry

		// Having both settles a labelling conflict rather than just adding a
		// constant. The reference server carries only 103410 and calls it Doors
		// of Pharros; our capture puts Pharros at 103320 and pairs 103410 with
		// area id 10340000 = m10_34 = Grave of Saints. The reference's label is
		// wrong. Its VALUE was right, which is why nothing broke.
		//
		// An earlier version of this map also guessed 103130 for the second
		// zone. That was wrong -- the 1031 prefix is m10_31, Heide's Tower of
		// Flame -- and it is a good illustration of why these are only added
		// from captures now.
	}
)

// The 6-digit activity-area ids decompose as mmmmbb: the first four digits are
// the map (m10_16 -> 1016) and the last two a block within it. Our own captures
// fit that exactly -- activity area 103410 arrived alongside area id 10340000 --
// which is worth stating because no public documentation of this id space could
// be found, so the wire is the only source.

// visitorPoolFor returns which auto-summon pool a player is currently AVAILABLE
// TO BE PULLED INTO, which is not the same as the covenant they belong to.
//
// This distinction cost real debugging time, so it is worth stating plainly.
// The pool is a property of the *target*, and for the Rat King it is inverted:
// you are rat-summonable precisely because you are NOT a rat and are standing in
// a rat area. The in-game covenant text says so outright — "you will not be
// pulled into other people's worlds while you are in the covenant".
//
// Bell Keepers and Blue Sentinels go the other way: the covenant member is the
// one summoned away, so they are in the pool when they hold the seal and are in
// the right place. In every case the REQUESTER is the host and the target is the
// guest who travels.
func visitorPoolFor(p matchProfile) ds2pb.VisitorType {
	if !p.received {
		return ds2pb.VisitorType_VisitorType_None
	}
	// Not in an online activity area means not available to anyone, whatever is
	// equipped. This is the bonfire case.
	if p.onlineActivityArea == 0 {
		return ds2pb.VisitorType_VisitorType_None
	}

	cov := p.effectiveCovenant()
	if p.guardiansSeal && cov == covenantBlueSentinels {
		return ds2pb.VisitorType_VisitorType_BlueSentinels
	}
	if p.bellKeepersSeal && cov == covenantBellKeepers &&
		bellKeeperCells[p.onlineActivityArea] {
		return ds2pb.VisitorType_VisitorType_BellKeepers
	}
	// Inverted on purpose: a rat cannot be prey.
	if cov != covenantRatKing && ratCells[p.onlineActivityArea] {
		return ds2pb.VisitorType_VisitorType_Rat
	}
	return ds2pb.VisitorType_VisitorType_None
}

// Soul memory tiers — the end value of each band, ascending.
//
// Not verifiable against this binary: these are server-side matchmaking policy
// the client never sees, so they cannot be recovered from the executable, and
// two test accounts cannot derive them either.
//
// Bands 1-43 are solid — every community source agrees character for character,
// and they trace back to systematic black-box testing (save-edited mules with
// binary-searched boundaries) rather than assertion.
//
// The 359,999,999 band is MEDIUM confidence. It is the majority reading and the
// reason this list has 44 entries below it rather than 43, but no published test
// covers it and it only ever separates players above 45M soul memory. Including
// it is the low-risk choice: too many bands merely narrows matching for players
// nobody has, while too few would silently merge them.
//
// Treat the apparent unanimity of sources with care — they descend from roughly
// two lineages, not five independent ones, and at least one widely-copied table
// is a stale snapshot with every lower bound off by one. If matching behaves
// oddly at a band edge, suspect this table first.
var soulMemoryTiers = []uint32{
	9_999, 19_999, 29_999, 39_999, 49_999, 69_999, 89_999, 109_999,
	129_999, 149_999, 179_999, 209_999, 239_999, 269_999, 299_999,
	349_999, 399_999, 449_999, 499_999, 599_999, 699_999, 799_999,
	899_999, 999_999, 1_099_999, 1_199_999, 1_299_999, 1_399_999,
	1_499_999, 1_749_999, 1_999_999, 2_249_999, 2_499_999, 2_749_999,
	2_999_999, 4_999_999, 6_999_999, 8_999_999, 11_999_999, 14_999_999,
	19_999_999, 29_999_999, 44_999_999, 359_999_999, 999_999_999,
}

// soulMemoryTier returns the 0-based band a soul memory value falls in.
func soulMemoryTier(sm uint32) int {
	for i, end := range soulMemoryTiers {
		if sm <= end {
			return i
		}
	}
	return len(soulMemoryTiers) - 1
}

// tierWindow is how far either side of the host's band a guest may sit.
type tierWindow struct{ below, above int }

// Per-covenant windows, as tiers either side of the ITEM USER's own band.
//
// Direction matters and is easy to invert. The window is built around the player
// using the covenant item — the one holding the crest or seal, which is always
// our requester — and it selects who they may be connected to. The reference
// server builds its window around the other party instead, which for invasions
// yields "you invade people at or below you", the opposite of DS2's documented
// anti-twink rule. We take its constants but not its argument order.
//
// An earlier version of this comment called the Rat window authoritative on the
// grounds that the covenant item says so in game. It does not: the item text
// carries no numbers, and the sentence quoted was wiki boilerplate repeated
// across several unrelated items. The value is still well-supported by testing —
// just not by that.
//
// Bell Keepers was same-tier-only here, which was simply too strict and is the
// kind of over-tightening that reads in game as "the covenant is broken".
var visitorTierWindows = map[ds2pb.VisitorType]tierWindow{
	// Both well-supported, though the Rat figure is flagged by the most rigorous
	// tester as not re-verified after patch 1.10.
	ds2pb.VisitorType_VisitorType_Rat:         {below: 1, above: 3},
	ds2pb.VisitorType_VisitorType_BellKeepers: {below: 1, above: 3},
	// DISPUTED: sources split between 5/4 and 7/6, and the wider figure has no
	// published test behind it. The narrower one is the majority, and being a
	// little too strict here shows up as "found nobody", which is far easier to
	// recognise than being too loose. Blue Sentinels is still untested live.
	ds2pb.VisitorType_VisitorType_BlueSentinels: {below: 5, above: 4},
}

// soulMemoryMatches reports whether a candidate's soul memory is in range of the
// item user's, given a window. Both are absolute soul memory, not tiers.
func soulMemoryMatches(itemUserSM, candidateSM uint32, w tierWindow) bool {
	user := soulMemoryTier(itemUserSM)
	cand := soulMemoryTier(candidateSM)
	return cand >= user-w.below && cand <= user+w.above
}

// matchmakingEnabled gates the whole filter so a bad table or a wrong area
// constant can be turned off from dso.env without a rebuild, rather than
// leaving the server unable to match anyone.
//
// Defaults ON: an unfiltered list is not a safe fallback, it is the bug.
func matchmakingEnabled() bool {
	v := os.Getenv("DSO_MATCHMAKING_FILTERS")
	if v == "" {
		return true
	}
	on, err := strconv.ParseBool(v)
	return err != nil || on
}
