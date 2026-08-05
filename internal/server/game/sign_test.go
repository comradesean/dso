package game

import (
	"io"
	"log/slog"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/frpg/rudp"
	"github.com/sstreight/dso/internal/proto/ds2pb"
	"github.com/sstreight/dso/internal/server/core"
)

// signTestService builds a service with two sessions registered, so pushes have
// somewhere to go. Signs are memory-only, so no store is needed.
func signTestService(t *testing.T) (*Service, logger, *clientSession, *clientSession) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := &Service{
		srv:      &core.Server{Config: config.Default(), Logger: log},
		signs:    newSignStore(),
		sessions: make(map[string]*clientSession),
	}

	host := newTestSession("comradesean", 1)
	summoner := newTestSession("mgnomad2", 2)
	svc.sessions["host"] = host
	svc.sessions["summoner"] = summoner
	return svc, log, host, summoner
}

// newTestSession builds a session with a real reliable-UDP connection whose
// datagrams go nowhere. Pushes therefore exercise the actual send path rather
// than being stubbed out, which is what a nil conn would have forced.
func newTestSession(accountID string, playerID uint32) *clientSession {
	sess := rudp.NewServerSession(func(_ []byte, _ bool) error { return nil })
	return &clientSession{
		accountID: accountID,
		playerID:  playerID,
		sess:      sess,
		conn:      rudp.NewMessageConn(sess),
	}
}

// testMatchingParameter builds a fully-populated MatchingParameter. Every field
// is `required` in proto2, so a partially-filled one fails to marshal — real
// clients always send a complete struct.
func testMatchingParameter() *ds2pb.MatchingParameter {
	u := func(v uint32) *uint32 { return &v }
	return &ds2pb.MatchingParameter{
		CalibrationVersion:     u(1),
		SoulLevel:              u(50),
		ClearCount:             u(0),
		Unknown_4:              u(0),
		Covenant:               u(0),
		Unknown_7:              u(0),
		DisableCrossRegionPlay: u(0),
		Unknown_9:              u(0),
		Unknown_10:             u(0),
		NameEngravedRing:       u(0),
		SoulMemory:             u(100000),
	}
}

func createSign(t *testing.T, svc *Service, log logger, cs *clientSession, area, cell uint32) uint32 {
	t.Helper()
	raw, err := proto.Marshal(&ds2pb.RequestCreateSign{
		OnlineAreaId:      proto.Uint32(area),
		CellId:            proto.Uint32(cell),
		SignType:          proto.Uint32(1),
		MatchingParameter: testMatchingParameter(),
		PlayerStruct:      []byte{0x01, 0x02},
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleCreateSign(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestCreateSignResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	return resp.GetSignId()
}

func listSigns(t *testing.T, svc *Service, log logger, cs *clientSession, area, cell uint32, known []uint32) *ds2pb.RequestGetSignListResponse {
	t.Helper()
	cellInfo := &ds2pb.SignCellInfo{CellId: proto.Uint32(cell)}
	for _, id := range known {
		cellInfo.LocalSigns = append(cellInfo.LocalSigns, &ds2pb.SignInfo{
			PlayerId: proto.Uint32(0), SignId: proto.Uint32(id),
		})
	}
	raw, err := proto.Marshal(&ds2pb.RequestGetSignList{
		OnlineAreaId:      proto.Uint32(area),
		SearchAreas:       []*ds2pb.SignCellInfo{cellInfo},
		MaxSigns:          proto.Uint32(10),
		MatchingParameter: testMatchingParameter(),
		Unknown_5:         proto.Uint32(0),
		Unknown_6:         proto.Uint32(0),
		Unknown_7:         proto.Uint32(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleGetSignList(log, cs, raw)
	if err != nil {
		t.Fatal(err)
	}
	var resp ds2pb.RequestGetSignListResponse
	if err := proto.Unmarshal(replyRaw, &resp); err != nil {
		t.Fatal(err)
	}
	return &resp
}

func TestSignCreateAndList(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	id := createSign(t, svc, log, host, 100, 7)
	if id < firstSignID {
		t.Errorf("sign id %d is below %d; low ids risk colliding with client-cached state", id, firstSignID)
	}

	resp := listSigns(t, svc, log, summoner, 100, 7, nil)
	if len(resp.GetSignData()) != 1 {
		t.Fatalf("summoner saw %d signs, want 1", len(resp.GetSignData()))
	}
	sd := resp.GetSignData()[0]
	if sd.GetSignInfo().GetSignId() != id {
		t.Errorf("sign_id: got %d, want %d", sd.GetSignInfo().GetSignId(), id)
	}
	if sd.GetPlayerPsnId() != "comradesean" {
		t.Errorf("player_psn_id: got %q", sd.GetPlayerPsnId())
	}
}

// TestSignListExcludesOwnSigns: a host must never be offered their own sign.
func TestSignListExcludesOwnSigns(t *testing.T) {
	svc, log, host, _ := signTestService(t)
	createSign(t, svc, log, host, 100, 7)

	resp := listSigns(t, svc, log, host, 100, 7, nil)
	if n := len(resp.GetSignData()) + len(resp.GetSignInfo()); n != 0 {
		t.Errorf("host was offered its own sign (%d entries)", n)
	}
}

// TestSignListSplitsKnownFromNew: signs the client already holds come back as
// bare SignInfo, not as full SignData. Sending the whole body again would be
// wasted bandwidth on every poll.
func TestSignListSplitsKnownFromNew(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	id := createSign(t, svc, log, host, 100, 7)

	resp := listSigns(t, svc, log, summoner, 100, 7, []uint32{id})
	if len(resp.GetSignData()) != 0 {
		t.Errorf("already-known sign resent as full data (%d)", len(resp.GetSignData()))
	}
	if len(resp.GetSignInfo()) != 1 {
		t.Fatalf("known sign not echoed as SignInfo (%d)", len(resp.GetSignInfo()))
	}
	if resp.GetSignInfo()[0].GetSignId() != id {
		t.Errorf("sign_id: got %d, want %d", resp.GetSignInfo()[0].GetSignId(), id)
	}
}

func TestSignAreaAndCellFiltering(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	createSign(t, svc, log, host, 100, 7)

	if n := len(listSigns(t, svc, log, summoner, 100, 7, nil).GetSignData()); n != 1 {
		t.Errorf("same area+cell: got %d, want 1", n)
	}
	if n := len(listSigns(t, svc, log, summoner, 100, 9, nil).GetSignData()); n != 0 {
		t.Errorf("different cell: got %d, want 0", n)
	}
	if n := len(listSigns(t, svc, log, summoner, 200, 7, nil).GetSignData()); n != 0 {
		t.Errorf("different area: got %d, want 0", n)
	}
}

// TestSummonClaimsSignOnce: a sign may only be claimed by one summoner at a time.
// Without this two players could both be told they summoned the same host.
func TestSummonClaimsSignOnce(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	id := createSign(t, svc, log, host, 100, 7)

	summonRaw, err := proto.Marshal(&ds2pb.RequestSummonSign{
		OnlineAreaId: proto.Int64(100),
		SignInfo:     &ds2pb.SignInfo{PlayerId: proto.Uint32(1), SignId: proto.Uint32(id)},
		PlayerStruct: []byte{0xAA},
		CellId:       proto.Int64(7),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.handleSummonSign(log, summoner, summonRaw); err != nil {
		t.Fatal(err)
	}
	if sg, _ := svc.signs.get(id); sg.summonedBy != summoner.playerID {
		t.Errorf("sign not claimed: summonedBy=%d, want %d", sg.summonedBy, summoner.playerID)
	}

	// A third player attempting the same sign must not displace the first.
	third := newTestSession("third", 3)
	svc.sessions["third"] = third
	if _, err := svc.handleSummonSign(log, third, summonRaw); err != nil {
		t.Fatal(err)
	}
	if sg, _ := svc.signs.get(id); sg.summonedBy != summoner.playerID {
		t.Errorf("second summoner stole the claim: summonedBy=%d, want %d",
			sg.summonedBy, summoner.playerID)
	}
}

// TestRejectReleasesSign: after the host declines, the sign must be claimable
// again rather than staying stuck.
func TestRejectReleasesSign(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	id := createSign(t, svc, log, host, 100, 7)

	summonRaw, _ := proto.Marshal(&ds2pb.RequestSummonSign{
		OnlineAreaId: proto.Int64(100),
		SignInfo:     &ds2pb.SignInfo{PlayerId: proto.Uint32(1), SignId: proto.Uint32(id)},
		PlayerStruct: []byte{0xAA},
		CellId:       proto.Int64(7),
	})
	if _, err := svc.handleSummonSign(log, summoner, summonRaw); err != nil {
		t.Fatal(err)
	}

	rejectRaw, _ := proto.Marshal(&ds2pb.RequestRejectSign{
		OnlineAreaId: proto.Int64(100),
		SignId:       proto.Int64(int64(id)),
		Error:        ds2pb.SummonErrorId_SummonErrorId_NoLongerBeSummonable.Enum(),
		Unknown_4:    proto.Int64(0),
		CellId:       proto.Int64(7),
	})
	if _, err := svc.handleRejectSign(log, host, rejectRaw); err != nil {
		t.Fatal(err)
	}
	if sg, _ := svc.signs.get(id); sg.summonedBy != 0 {
		t.Errorf("sign still claimed after rejection: summonedBy=%d", sg.summonedBy)
	}
}

// TestSignsDroppedWhenHostLeaves: a departed host's signs must not linger, or
// other players see a sign that can never be summoned.
func TestSignsDroppedWhenHostLeaves(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	id := createSign(t, svc, log, host, 100, 7)

	// The summoner sees it, so becomes aware of it.
	if n := len(listSigns(t, svc, log, summoner, 100, 7, nil).GetSignData()); n != 1 {
		t.Fatalf("setup: summoner should see 1 sign, got %d", n)
	}

	svc.dropSession("host", host)

	if _, ok := svc.signs.get(id); ok {
		t.Error("sign survived its host disconnecting")
	}
	if n := len(listSigns(t, svc, log, summoner, 100, 7, nil).GetSignData()); n != 0 {
		t.Errorf("departed host's sign still listed (%d)", n)
	}
}

func TestRemoveSignDeletesIt(t *testing.T) {
	svc, log, host, summoner := signTestService(t)
	id := createSign(t, svc, log, host, 100, 7)

	rmRaw, _ := proto.Marshal(&ds2pb.RequestRemoveSign{
		OnlineAreaId: proto.Uint32(100),
		SignId:       proto.Uint32(id),
		CellId:       proto.Uint32(7),
	})
	if _, err := svc.handleRemoveSign(log, host, rmRaw); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.signs.get(id); ok {
		t.Error("sign still present after removal")
	}
	if n := len(listSigns(t, svc, log, summoner, 100, 7, nil).GetSignData()); n != 0 {
		t.Errorf("removed sign still listed (%d)", n)
	}
}
