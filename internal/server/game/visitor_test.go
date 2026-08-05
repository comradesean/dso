package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

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
	svc, log, _, summoner := signTestService(t)
	for i := uint32(10); i < 15; i++ {
		svc.sessions[string(rune('a'+i))] = newTestSession("filler", i)
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
