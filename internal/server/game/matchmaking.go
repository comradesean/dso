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
	covenant   uint32

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
}

// profileFromStatus reduces an AllStatus blob to the fields matching needs.
//
// A failure to parse is not an error: the blob is client-supplied and a partial
// or unexpected one should degrade to "unknown player", never drop the session.
func profileFromStatus(blob []byte) matchProfile {
	var all ds2datapb.AllStatus
	if err := proto.Unmarshal(blob, &all); err != nil {
		return matchProfile{}
	}
	p := matchProfile{received: true}

	if st := all.GetPlayerStatus(); st != nil {
		p.soulMemory = st.GetSoulMemory()
		p.soulLevel = st.GetSoulLevel()
		p.covenant = st.GetCovenant()
		p.sittingAtBonfire = st.GetSittingAtBonfire() != 0
		p.disableCrossRegion = st.GetDisableCrossRegionPlay() != 0
	}
	if loc := all.GetPlayerLocation(); loc != nil {
		p.onlineActivityArea = loc.GetOnlineActivityAreaId()
	}
	if it := all.GetItemUsingInfo(); it != nil {
		p.guardiansSeal = it.GetGuardiansSeal() != 0
		p.bellKeepersSeal = it.GetBellKeepersSeal() != 0
		p.crestOfTheRat = it.GetCrestOfTheRat() != 0
		p.nameEngravedRing = it.GetNamedRingGod()
	}
	return p
}

// DS2 covenant ids as they appear in PlayerStatus.covenant.
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
		103410: true, // Doors of Pharros
		103130: true, // Grave of Saints
	}
)

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

	if p.guardiansSeal && p.covenant == covenantBlueSentinels {
		return ds2pb.VisitorType_VisitorType_BlueSentinels
	}
	if p.bellKeepersSeal && p.covenant == covenantBellKeepers &&
		bellKeeperCells[p.onlineActivityArea] {
		return ds2pb.VisitorType_VisitorType_BellKeepers
	}
	// Inverted on purpose: a rat cannot be prey.
	if p.covenant != covenantRatKing && ratCells[p.onlineActivityArea] {
		return ds2pb.VisitorType_VisitorType_Rat
	}
	return ds2pb.VisitorType_VisitorType_None
}

// Soul memory tiers — the end value of each band, ascending.
//
// REFERENCE-DERIVED and not verified against this binary. There is no plausible
// way to recover these from the PS3 executable (they are matchmaking policy, and
// the client never sees them), and no way to derive them from two test accounts,
// so this is one of the few places the reference is the only available source.
// If matching ever behaves oddly at a band edge, suspect this table first.
var soulMemoryTiers = []uint32{
	9_999, 19_999, 29_999, 39_999, 49_999, 69_999, 89_999, 109_999,
	129_999, 149_999, 179_999, 209_999, 239_999, 269_999, 299_999,
	349_999, 399_999, 449_999, 499_999, 599_999, 699_999, 799_999,
	899_999, 999_999, 1_099_999, 1_199_999, 1_299_999, 1_399_999,
	1_499_999, 1_749_999, 1_999_999, 2_249_999, 2_499_999, 2_749_999,
	2_999_999, 4_999_999, 6_999_999, 8_999_999, 11_999_999, 14_999_999,
	19_999_999, 29_999_999, 44_999_999, 999_999_999,
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

// Per-covenant windows.
//
// The Rat King window is AUTHORITATIVE — it is stated in the game's own covenant
// description: "you may pull players into your world that are up to 1 Soul
// Memory tier lower or 3 tiers higher than your own". The others have no such
// text and default to same-tier-only, which is the reference's default. Being
// too strict here shows up as "no targets found", which is a much easier
// symptom to recognise than being too loose.
var visitorTierWindows = map[ds2pb.VisitorType]tierWindow{
	ds2pb.VisitorType_VisitorType_Rat:           {below: 1, above: 3},
	ds2pb.VisitorType_VisitorType_BellKeepers:   {below: 0, above: 0},
	ds2pb.VisitorType_VisitorType_BlueSentinels: {below: 0, above: 0},
}

// soulMemoryMatches reports whether a guest's soul memory is in range of a
// host's, given a window. Both are absolute soul memory, not tiers.
func soulMemoryMatches(hostSM, guestSM uint32, w tierWindow) bool {
	host := soulMemoryTier(hostSM)
	guest := soulMemoryTier(guestSM)
	return guest >= host-w.below && guest <= host+w.above
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
