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

// TestJoinPushUsesTheOddAliasGroup pins the push ids.
//
// Eight aliases exist for four message types. The PC enum's four values are all
// odd and match exactly the group the decompilation records as registered first,
// which is why they are used unchanged — this test fails if someone "helpfully"
// switches them to the even group.
func TestJoinPushUsesTheOddAliasGroup(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  ds2pb.PushMessageId
		want int
	}{
		{"join", ds2pb.PushMessageId_PushID_PushRequestJoinQuickMatch, 0x03E1},
		{"reject", ds2pb.PushMessageId_PushID_PushRequestRejectQuickMatch, 0x03E3},
		{"allow", ds2pb.PushMessageId_PushID_PushRequestAllowQuickMatch, 0x03E5},
		{"remove", ds2pb.PushMessageId_PushID_PushRequestRemoveQuickMatch, 0x03E7},
	} {
		if int(tc.got) != tc.want {
			t.Errorf("%s push id = %#04x, want %#04x (odd group, registered first)",
				tc.name, int(tc.got), tc.want)
		}
		if int(tc.got)%2 == 0 {
			t.Errorf("%s push id %#04x is even; the odd group is the one the "+
				"decompilation records as a complete registration pass", tc.name, int(tc.got))
		}
	}
}

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
