package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// inBellKeeperPool makes a session a valid Bell Keeper target: the covenant, the
// equipped seal and a belfry activity area are all required, and soul memory is
// put in the same tier as testMatchingParameter's 100000 because the Bell Keeper
// window is same-tier-only.
//
// Tests have to do this explicitly now. A session with no status blob is
// deliberately unmatchable — we should never offer a player we know nothing
// about — so before this existed every visitor test was passing against a list
// that had simply not been filtered at all.
func inBellKeeperPool(cs *clientSession) {
	cs.profile = matchProfile{
		received:           true,
		covenant:           cov(covenantBellKeepers),
		bellKeepersSeal:    true,
		onlineActivityArea: 101640, // Belfry Luna
		soulMemory:         100000,
	}
}

func visitorListRequest(t *testing.T, max int64) []byte {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestGetVisitorList{
		OnlineAreaId:      proto.Int64(10160000),
		CellId:            proto.Int64(101650),
		MaxTargets:        proto.Int64(max),
		MatchingParameter: testMatchingParameter(),
		Type:              ds2pb.VisitorType_VisitorType_BellKeepers.Enum(),
		Field_6:           proto.Int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestVisitorListExcludesSelfAndEchoesArea covers both things the response has to
// get right: never offering the requester their own world, and echoing the area
// and cell, which are `required` and are how the client matches the reply to the
// area it asked about.
func TestVisitorListExcludesSelfAndEchoesArea(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	inBellKeeperPool(host)
	inBellKeeperPool(summoner) // eligible too, to prove self-exclusion still wins

	replyRaw, err := svc.handleGetVisitorList(log, summoner, visitorListRequest(t, 5))
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetVisitorListResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatalf("client-side parse would reject our reply: %v", err)
	}

	if resp.GetOnlineAreaId() != 10160000 || resp.GetCellId() != 101650 {
		t.Errorf("area/cell = %d/%d, want them echoed as 10160000/101650",
			resp.GetOnlineAreaId(), resp.GetCellId())
	}
	if len(resp.GetTargetData()) != 1 {
		t.Fatalf("returned %d targets, want 1 (the other player)", len(resp.GetTargetData()))
	}
	if got := uint32(resp.GetTargetData()[0].GetPlayerId()); got != host.playerID {
		t.Errorf("offered player %d, want the other session %d", got, host.playerID)
	}
	for _, td := range resp.GetTargetData() {
		if uint32(td.GetPlayerId()) == summoner.playerID {
			t.Error("requester was offered their own world")
		}
	}
}

// TestVisitorListRespectsMaxTargets — the client asks for a bounded list and a
// server that ignores the bound can overflow it.
func TestVisitorListRespectsMaxTargets(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	inBellKeeperPool(host)
	for i := uint32(10); i < 15; i++ {
		filler := newTestSession("filler", i)
		inBellKeeperPool(filler)
		svc.sessions[string(rune('a'+i))] = filler
	}

	replyRaw, err := svc.handleGetVisitorList(log, summoner, visitorListRequest(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetVisitorListResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.GetTargetData()) > 2 {
		t.Errorf("returned %d targets for max_targets=2", len(resp.GetTargetData()))
	}
}

// TestVisitBrokersToHost is the feature: the visitor asks, the host is pushed.
func TestVisitBrokersToHost(t *testing.T) {
	svc, log, host, visitor := signTestService(t)

	raw, err := proto.Marshal(&ds2pb.RequestVisit{
		OnlineAreaId: proto.Int64(10160000),
		CellId:       proto.Int64(101640),
		Type:         ds2pb.VisitorType_VisitorType_BellKeepers.Enum(),
		PlayerId:     proto.Int64(int64(host.playerID)),
		PlayerStruct: []byte{0xAB, 0xCD},
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleVisit(log, visitor, raw)
	if err != nil {
		t.Fatal(err)
	}
	// The opcode is request/response in the decompilation, so a reply is required
	// even though the PC captures never recorded one.
	var resp ds2pb.RequestVisitResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatalf("reply does not parse: %v", err)
	}
}

// TestVisitPushWireFormat pins the push body. Every field is `required`, and an
// unset one means the client drops the push in silence — indistinguishable from
// the wrong push id, which is the thing actually in doubt here.
func TestVisitPushWireFormat(t *testing.T) {
	body, err := proto.Marshal(&ds2pb.PushRequestVisit{
		PushMessageId: ds2pb.PushMessageId(visitorPushIDFor(
			ds2pb.VisitorType_VisitorType_BellKeepers, visitorRoleVisit)).Enum(),
		PlayerId:     proto.Int64(7),
		PlayerPsnId:  proto.String("comradesean"),
		PlayerStruct: []byte{0x01},
		Type:         ds2pb.VisitorType_VisitorType_BellKeepers.Enum(),
		OnlineAreaId: proto.Int64(10160000),
		CellId:       proto.Int64(101640),
	})
	if err != nil {
		t.Fatalf("the push we send does not marshal: %v", err)
	}
	var got ds2pb.PushRequestVisit
	if err := proto.Unmarshal(body, &got); err != nil {
		t.Fatalf("client-side parse would reject our push: %v", err)
	}
	want := visitorPushIDFor(ds2pb.VisitorType_VisitorType_BellKeepers, visitorRoleVisit)
	if int32(got.GetPushMessageId()) != want {
		t.Errorf("push id = %#04x, want %#04x", int(got.GetPushMessageId()), want)
	}
}

// TestVisitToOfflineTargetRejects — a visitor must be told, not left waiting.
func TestVisitToOfflineTargetRejects(t *testing.T) {
	svc, log, _, visitor := signTestService(t)

	raw, err := proto.Marshal(&ds2pb.RequestVisit{
		OnlineAreaId: proto.Int64(10160000),
		CellId:       proto.Int64(101640),
		Type:         ds2pb.VisitorType_VisitorType_Rat.Enum(),
		PlayerId:     proto.Int64(9999), // nobody
		PlayerStruct: []byte{0x01},
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleVisit(log, visitor, raw)
	if err != nil {
		t.Fatal(err)
	}
	if replyRaw == nil {
		t.Error("no reply to RequestVisit; the client would retry and block other UI")
	}
}

// TestVisitorAliasesStayInsideTheBlock — VisitorType_3 exists in the schema and
// is one past the block, which is only nine aliases wide.
//
// This is not a harmless dead id: 0x03D2 is RequestGetBreakInTargetList's opcode,
// so an unclamped push would collide with an unrelated message rather than being
// dropped.
func TestVisitorAliasesStayInsideTheBlock(t *testing.T) {
	types := []ds2pb.VisitorType{
		ds2pb.VisitorType_VisitorType_None,
		ds2pb.VisitorType_VisitorType_BlueSentinels,
		ds2pb.VisitorType_VisitorType_BellKeepers,
		ds2pb.VisitorType_VisitorType_Rat,
		ds2pb.VisitorType_VisitorType_3,
	}
	for _, vt := range types {
		for role := 0; role <= 2; role++ {
			got := visitorPushIDFor(vt, role)
			if got < 0x03C9 || got > 0x03D1 {
				t.Errorf("visitorPushIDFor(%v, %d) = %#04x, outside 0x03C9-0x03D1",
					vt, role, got)
			}
		}
	}
	// The three real covenants must still map to distinct triples.
	seen := map[int32]ds2pb.VisitorType{}
	for _, vt := range []ds2pb.VisitorType{
		ds2pb.VisitorType_VisitorType_BlueSentinels,
		ds2pb.VisitorType_VisitorType_BellKeepers,
		ds2pb.VisitorType_VisitorType_Rat,
	} {
		id := visitorPushIDFor(vt, visitorRoleVisit)
		if prev, dup := seen[id]; dup {
			t.Errorf("%v and %v share visit push %#04x", prev, vt, id)
		}
		seen[id] = vt
	}
	// And the one confirmed live must not move.
	if got := visitorPushIDFor(ds2pb.VisitorType_VisitorType_BellKeepers, visitorRoleVisit); got != 0x03CC {
		t.Errorf("Bell Keeper visit push = %#04x, want 0x03CC (confirmed live)", got)
	}
}
