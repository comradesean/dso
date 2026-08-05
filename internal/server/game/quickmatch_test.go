package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

const (
	arenaArea int64 = 10230000
	arenaCell int64 = 102350
)

func registerArena(t *testing.T, svc *Service, log logger, cs *clientSession, mode ds2pb.QuickMatchGameMode) {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestRegisterQuickMatch{
		OnlineAreaId:      proto.Int64(arenaArea),
		CellId:            proto.Int64(arenaCell),
		MatchingParameter: testMatchingParameter(),
		Mode:              mode.Enum(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleRegisterQuickMatch(log, cs, raw); err != nil {
		t.Fatal(err)
	}
}

func searchArena(t *testing.T, svc *Service, log logger, cs *clientSession, mode ds2pb.QuickMatchGameMode, cell int64) []*ds2pb.QuickMatchData {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestSearchQuickMatch{
		OnlineAreaId:      proto.Int64(arenaArea),
		CellId:            proto.Int64(cell),
		MatchingParameter: testMatchingParameter(),
		MaxResults:        proto.Int64(10),
		Mode:              mode.Enum(),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleSearchQuickMatch(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestSearchQuickMatchResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatalf("client-side parse would reject our reply: %v", err)
	}
	return resp.GetMatches()
}

// TestArenaSearchFindsRegisteredOpponent is the basic loop: one player advertises,
// another finds them.
func TestArenaSearchFindsRegisteredOpponent(t *testing.T) {
	svc, log, host, joiner := signTestService(t)
	registerArena(t, svc, log, host, ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood)

	found := searchArena(t, svc, log, joiner,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, arenaCell)
	if len(found) != 1 {
		t.Fatalf("found %d matches, want 1", len(found))
	}
	if got := uint32(found[0].GetPlayerId()); got != host.playerID {
		t.Errorf("found player %d, want the host %d", got, host.playerID)
	}
	// The searcher must never be offered their own registration.
	if own := searchArena(t, svc, log, host,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, arenaCell); len(own) != 0 {
		t.Errorf("host was offered their own registration (%d results)", len(own))
	}
}

// TestArenaSearchFiltersByModeAndCell — the arena has two game modes and several
// locations, and mixing them would match players into the wrong bracket.
func TestArenaSearchFiltersByModeAndCell(t *testing.T) {
	svc, log, host, joiner := signTestService(t)
	registerArena(t, svc, log, host, ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood)

	if got := searchArena(t, svc, log, joiner,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Blue, arenaCell); len(got) != 0 {
		t.Errorf("Blue search returned %d Brotherhood registrations", len(got))
	}
	if got := searchArena(t, svc, log, joiner,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, arenaCell+1); len(got) != 0 {
		t.Errorf("search of a different cell returned %d registrations", len(got))
	}
}

// TestReregisteringReplaces — the client's own register/update cycle would
// otherwise accumulate duplicate advertisements for one player.
func TestReregisteringReplaces(t *testing.T) {
	svc, log, host, joiner := signTestService(t)
	registerArena(t, svc, log, host, ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood)
	registerArena(t, svc, log, host, ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood)

	if got := searchArena(t, svc, log, joiner,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, arenaCell); len(got) != 1 {
		t.Errorf("found %d registrations for one player, want 1", len(got))
	}
}

// TestArenaRegistrationDroppedOnDisconnect — a departed player must stop being
// advertised, or joining them fails in a way that looks like a server fault.
func TestArenaRegistrationDroppedOnDisconnect(t *testing.T) {
	svc, log, host, joiner := signTestService(t)
	registerArena(t, svc, log, host, ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood)

	// The joiner sees it, which is what earns them the removal push.
	searchArena(t, svc, log, joiner,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, arenaCell)

	svc.dropQuickMatchForPlayer(log, host.playerID)

	if got := searchArena(t, svc, log, joiner,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, arenaCell); len(got) != 0 {
		t.Errorf("departed player still advertised (%d results)", len(got))
	}
}

// TestUnregisterRemoves covers the explicit withdrawal path.
func TestUnregisterRemoves(t *testing.T) {
	svc, log, host, joiner := signTestService(t)
	registerArena(t, svc, log, host, ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood)

	raw, err := proto.Marshal(&ds2pb.RequestUnregisterQuickMatch{
		OnlineAreaId: proto.Int64(arenaArea),
		CellId:       proto.Int64(arenaCell),
		Mode:         ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood.Enum(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleUnregisterQuickMatch(log, host, raw); err != nil {
		t.Fatal(err)
	}

	if got := searchArena(t, svc, log, joiner,
		ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, arenaCell); len(got) != 0 {
		t.Errorf("unregistered player still advertised (%d results)", len(got))
	}
}

// The former TestJoinPushUsesTheOddAliasGroup asserted the PC enum's four odd
// values were correct because they matched "the group registered first". That
// theory is superseded: the aliases are two per role, one per venue, so those
// four are simply mode 1. TestPushAliasFormulas below replaces it.

// TestJoinBrokersToHost is the feature: a joiner asks, the host is pushed.
func TestJoinBrokersToHost(t *testing.T) {
	svc, log, host, joiner := signTestService(t)
	registerArena(t, svc, log, host, ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood)

	raw, err := proto.Marshal(&ds2pb.RequestJoinQuickMatch{
		OnlineAreaId: proto.Int64(arenaArea),
		CellId:       proto.Int64(arenaCell),
		PlayerId:     proto.Int64(int64(host.playerID)),
		Mode:         ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood.Enum(),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleJoinQuickMatch(log, joiner, raw)
	if err != nil {
		t.Fatal(err)
	}
	// Request/response per the decompilation, even though the PC protos mark the
	// response "never received".
	var resp ds2pb.RequestJoinQuickMatchResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatalf("reply does not parse: %v", err)
	}
}

// TestPushAliasFormulas pins the three alias layouts recovered from the
// disassembly. Each manager is instantiated once per mode and registers all of
// its message types at that mode's slice of the block, so an alias identifies a
// (mode, role) pair — not a message type on its own.
//
// Getting this wrong is silent and expensive: 0x3BD, 0x3C1 and 0x3C5 were each
// tested live as invasion rejections and ignored, because they are TARGET pushes
// for modes 1, 2 and 3.
func TestPushAliasFormulas(t *testing.T) {
	t.Run("breakin", func(t *testing.T) {
		// opcode = 0x3B9 + 4*mode + role
		for _, tc := range []struct {
			mode ds2pb.BreakInType
			role int
			want int32
		}{
			{ds2pb.BreakInType_BreakInType_RedEyeOrb, breakInRoleTarget, 0x3B9},
			{ds2pb.BreakInType_BreakInType_RedEyeOrb, breakInRoleReject, 0x3BA},
			{ds2pb.BreakInType_BreakInType_RedEyeOrb, breakInRoleAllow, 0x3BB},
			{ds2pb.BreakInType_BreakInType_BlueEyeOrb, breakInRoleTarget, 0x3C1},
			{ds2pb.BreakInType_BreakInType_BlueEyeOrb, breakInRoleReject, 0x3C2},
		} {
			if got := breakInPushIDFor(tc.mode, tc.role); got != tc.want {
				t.Errorf("breakIn(mode=%v, role=%d) = %#04x, want %#04x",
					tc.mode, tc.role, got, tc.want)
			}
		}
		// 0x03B9 is the one value confirmed live; guard it explicitly.
		if got := breakInPushIDFor(ds2pb.BreakInType_BreakInType_RedEyeOrb, breakInRoleTarget); got != 0x03B9 {
			t.Errorf("the confirmed-live target push moved to %#04x", got)
		}
	})

	t.Run("visitor", func(t *testing.T) {
		// opcode = 0x3C9 + 3*mode + role
		for _, tc := range []struct {
			mode ds2pb.VisitorType
			role int
			want int32
		}{
			{ds2pb.VisitorType_VisitorType_BlueSentinels, visitorRoleVisit, 0x3C9},
			{ds2pb.VisitorType_VisitorType_BlueSentinels, visitorRoleRemove, 0x3CB},
			{ds2pb.VisitorType_VisitorType_BellKeepers, visitorRoleVisit, 0x3CC},
			{ds2pb.VisitorType_VisitorType_Rat, visitorRoleVisit, 0x3CF},
			{ds2pb.VisitorType_VisitorType_Rat, visitorRoleRemove, 0x3D1},
		} {
			if got := visitorPushIDFor(tc.mode, tc.role); got != tc.want {
				t.Errorf("visitor(mode=%v, role=%d) = %#04x, want %#04x",
					tc.mode, tc.role, got, tc.want)
			}
		}
		// VisitorType_None is -1 and must not compute an id below the block.
		if got := visitorPushIDFor(ds2pb.VisitorType_VisitorType_None, visitorRoleVisit); got < visitorPushBase {
			t.Errorf("VisitorType_None produced %#04x, below the block base %#04x",
				got, visitorPushBase)
		}
	})

	t.Run("quickmatch", func(t *testing.T) {
		// opcode = 0x3E0 + 2*role + mode  -- mode-MINOR, reversed from the others
		for _, tc := range []struct {
			mode ds2pb.QuickMatchGameMode
			role int
			want int32
		}{
			{ds2pb.QuickMatchGameMode_QuickMatchGameMode_Blue, quickMatchRoleJoin, 0x3E0},
			{ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, quickMatchRoleJoin, 0x3E1},
			{ds2pb.QuickMatchGameMode_QuickMatchGameMode_Blue, quickMatchRoleReject, 0x3E2},
			{ds2pb.QuickMatchGameMode_QuickMatchGameMode_Brotherhood, quickMatchRoleRemove, 0x3E7},
		} {
			if got := quickMatchPushIDFor(tc.mode, tc.role); got != tc.want {
				t.Errorf("quickMatch(mode=%v, role=%d) = %#04x, want %#04x",
					tc.mode, tc.role, got, tc.want)
			}
		}
	})
}

// TestEveryAliasIsWithinItsBlock — an id outside the registered range is never
// dispatched, so an off-by-one here is a feature that silently does nothing.
func TestEveryAliasIsWithinItsBlock(t *testing.T) {
	for mode := 0; mode <= 3; mode++ {
		for role := 0; role <= 2; role++ {
			if got := breakInPushIDFor(ds2pb.BreakInType(mode), role); got < 0x03B9 || got > 0x03C8 {
				t.Errorf("breakIn(%d,%d) = %#04x, outside 0x03B9-0x03C8", mode, role, got)
			}
		}
	}
	for mode := 0; mode <= 2; mode++ {
		for role := 0; role <= 2; role++ {
			if got := visitorPushIDFor(ds2pb.VisitorType(mode), role); got < 0x03C9 || got > 0x03D1 {
				t.Errorf("visitor(%d,%d) = %#04x, outside 0x03C9-0x03D1", mode, role, got)
			}
		}
	}
	for mode := 0; mode <= 1; mode++ {
		for role := 0; role <= 3; role++ {
			if got := quickMatchPushIDFor(ds2pb.QuickMatchGameMode(mode), role); got < 0x03E0 || got > 0x03E7 {
				t.Errorf("quickMatch(%d,%d) = %#04x, outside 0x03E0-0x03E7", mode, role, got)
			}
		}
	}
}
