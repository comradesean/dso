package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

func createMirrorKnightSign(t *testing.T, svc *Service, log logger, cs *clientSession) uint32 {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestCreateMirrorKnightSign{
		MatchingParameter: testMatchingParameter(),
		Data:              []byte{0xDE, 0xAD, 0xBE, 0xEF},
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleCreateMirrorKnightSign(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestCreateMirrorKnightSignResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetSignId() == 0 {
		t.Fatal("sign_id is 0; the client keys the sign by this")
	}
	return uint32(resp.GetSignId())
}

// TestMirrorKnightSignIDsAreDisjointFromSigns is the reason the two stores exist
// separately rather than sharing one.
//
// Both stores are seeded independently, so without distinct ranges an arena sign
// and an ordinary summon sign would be handed the same id. The client keys cached
// state by sign id without regard for which system produced it — the same class
// of bug that once made a fresh blood message show as already-rated.
func TestMirrorKnightSignIDsAreDisjointFromSigns(t *testing.T) {
	svc, log, host, _ := signTestService(t)

	mkID := createMirrorKnightSign(t, svc, log, host)

	ordinaryRaw, err := proto.Marshal(&ds2pb.RequestCreateSign{
		OnlineAreaId:      proto.Uint32(100),
		MatchingParameter: testMatchingParameter(),
		PlayerStruct:      []byte{0x01},
		CellId:            proto.Uint32(7),
		SignType:          proto.Uint32(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleCreateSign(log, host, ordinaryRaw)
	if err != nil {
		t.Fatal(err)
	}
	var ordinary ds2pb.RequestCreateSignResponse
	if err := proto.Unmarshal(replyRaw, &ordinary); err != nil {
		t.Fatal(err)
	}

	if mkID == ordinary.GetSignId() {
		t.Fatalf("mirror knight sign and summon sign share id %d; the client caches "+
			"state by sign id and cannot tell the two systems apart", mkID)
	}
	if mkID < firstMirrorKnightSignID {
		t.Errorf("mirror knight sign id %d is below its range start %d",
			mkID, firstMirrorKnightSignID)
	}
}

// TestMirrorKnightListExcludesOwnSign — a host must not be shown their own arena
// sign, or they could summon themselves.
func TestMirrorKnightListExcludesOwnSign(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	createMirrorKnightSign(t, svc, log, host)

	list := func(cs *clientSession) []*ds2pb.SignData {
		raw, err := proto.Marshal(&ds2pb.RequestGetMirrorKnightSignList{
			MaxSigns:          proto.Int64(10),
			MatchingParameter: testMatchingParameter(),
		})
		if err != nil {
			t.Fatal(err)
		}
		replyRaw, err := svc.handleGetMirrorKnightSignList(log, cs, raw)
		if err != nil {
			t.Fatal(err)
		}
		var resp ds2pb.RequestGetMirrorKnightSignListResponse
		if err := proto.Unmarshal(replyRaw, &resp); err != nil {
			t.Fatal(err)
		}
		return resp.GetSignData()
	}

	if got := list(host); len(got) != 0 {
		t.Errorf("host sees %d of their own signs, want 0", len(got))
	}
	seen := list(summoner)
	if len(seen) != 1 {
		t.Fatalf("summoner sees %d signs, want 1", len(seen))
	}
	// SignData's area, cell and type are `required`; Mirror Knight has no
	// placement, so they must still be present (as zero) or the client's proto2
	// parser rejects the whole message.
	sd := seen[0]
	if sd.OnlineAreaId == nil || sd.CellId == nil || sd.SignType == nil {
		t.Error("required placement fields absent; the client would reject this message")
	}
}

// TestMirrorKnightSummonBrokersToHost covers the whole point of the feature: the
// summoner claims, the host is told, and a second summoner is refused rather than
// both being let through.
func TestMirrorKnightSummonBrokersToHost(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	signID := createMirrorKnightSign(t, svc, log, host)

	summonRaw, err := proto.Marshal(&ds2pb.RequestSummonMirrorKnightSign{
		SignInfo: &ds2pb.SignInfo{
			PlayerId: proto.Uint32(host.playerID),
			SignId:   proto.Uint32(signID),
		},
		PlayerStruct: []byte{0x11, 0x22},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleSummonMirrorKnightSign(log, summoner, summonRaw); err != nil {
		t.Fatal(err)
	}

	sg, ok := svc.mirrorKnight.get(signID)
	if !ok {
		t.Fatal("sign vanished after summon")
	}
	if sg.summonedBy != summoner.playerID {
		t.Errorf("sign claimed by %d, want summoner %d", sg.summonedBy, summoner.playerID)
	}

	// A second summoner must be refused, not silently allowed alongside the first.
	third := newTestSession("thirdwheel", 3)
	svc.sessions["third"] = third
	if _, err := svc.handleSummonMirrorKnightSign(log, third, summonRaw); err != nil {
		t.Fatal(err)
	}
	if sg, _ := svc.mirrorKnight.get(signID); sg.summonedBy != summoner.playerID {
		t.Errorf("second summoner stole the claim: now %d, want %d",
			sg.summonedBy, summoner.playerID)
	}
}

// TestMirrorKnightSignDroppedOnDisconnect — a departed host's arena sign must go
// with them, or summoning it fails in a way that looks like a server bug.
func TestMirrorKnightSignDroppedOnDisconnect(t *testing.T) {
	svc, log, host, _ := signTestService(t)
	signID := createMirrorKnightSign(t, svc, log, host)

	svc.dropMirrorKnightSignsForPlayer(log, host.playerID)

	if _, ok := svc.mirrorKnight.get(signID); ok {
		t.Error("arena sign survived its host disconnecting")
	}
}

// TestRejectMirrorKnightFreesTheSign — after a host declines, the sign must be
// claimable again rather than stuck.
func TestRejectMirrorKnightFreesTheSign(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	signID := createMirrorKnightSign(t, svc, log, host)

	summonRaw, err := proto.Marshal(&ds2pb.RequestSummonMirrorKnightSign{
		SignInfo:     &ds2pb.SignInfo{PlayerId: proto.Uint32(host.playerID), SignId: proto.Uint32(signID)},
		PlayerStruct: []byte{0x11},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleSummonMirrorKnightSign(log, summoner, summonRaw); err != nil {
		t.Fatal(err)
	}

	rejectRaw, err := proto.Marshal(&ds2pb.RequestRejectMirrorKnightSign{
		OnlineAreaId: proto.Int64(0),
		SignId:       proto.Int64(int64(signID)),
		Error:        ds2pb.SummonErrorId_SummonErrorId_SignAlreadyUsed.Enum(),
		Unknown_4:    proto.Int64(2),
		CellId:       proto.Int64(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleRejectMirrorKnightSign(log, host, rejectRaw); err != nil {
		t.Fatal(err)
	}

	sg, ok := svc.mirrorKnight.get(signID)
	if !ok {
		t.Fatal("sign removed by a rejection; it should stay available")
	}
	if sg.summonedBy != 0 {
		t.Errorf("sign still claimed by %d after rejection; nobody else can summon it",
			sg.summonedBy)
	}
}
