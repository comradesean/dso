package game

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/config"
	"github.com/sstreight/dso/internal/proto/ds2pb"
	"github.com/sstreight/dso/internal/server/core"
	"github.com/sstreight/dso/internal/server/store"
)

func testService(t *testing.T) (*Service, logger, *clientSession) {
	t.Helper()
	st, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// A real Server with real config rather than a zero value: handlers read
	// config, and a nil srv would make them panic in tests while working in
	// production, which is the wrong way round.
	svc := &Service{
		srv:      &core.Server{Config: config.Default(), Logger: log},
		store:    st,
		sessions: make(map[string]*clientSession),
	}
	cs := &clientSession{accountID: "comradesean", playerID: 1, characterID: 1}
	return svc, log, cs
}

// TestBloodMessageRoundTrip is the single-client path: place a message, then read
// it back from the area listing. This is the first feature that can be verified
// end to end without a second player.
func TestBloodMessageRoundTrip(t *testing.T) {
	svc, log, cs := testService(t)
	body := []byte{0x01, 0x02, 0x03, 0x04}

	createRaw, err := proto.Marshal(&ds2pb.RequestCreateBloodMessage{
		OnlineAreaId: proto.Uint32(100),
		CellId:       proto.Uint32(7),
		CharacterId:  proto.Uint32(1),
		MessageData:  body,
	})
	if err != nil {
		t.Fatal(err)
	}
	replyRaw, err := svc.handleCreateBloodMessage(log, cs, createRaw)
	if err != nil {
		t.Fatal(err)
	}
	var created ds2pb.RequestCreateBloodMessageResponse
	if err := proto.Unmarshal(replyRaw, &created); err != nil {
		t.Fatal(err)
	}
	if created.GetMessageId() == 0 {
		t.Fatal("message_id is 0; the client keys the message by this")
	}

	listRaw, err := proto.Marshal(&ds2pb.RequestGetBloodMessageList{
		OnlineAreaId: proto.Uint32(100),
		MaxMessages:  proto.Uint32(10),
		SearchAreas: []*ds2pb.BloodMessageCellLimitData{
			{CellId: proto.Uint32(7), MaxType_1: proto.Uint32(10), MaxType_2: proto.Uint32(10)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	listReplyRaw, err := svc.handleGetBloodMessageList(log, cs, listRaw)
	if err != nil {
		t.Fatal(err)
	}
	var list ds2pb.RequestGetBloodMessageListResponse
	if err := proto.Unmarshal(listReplyRaw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.GetMessages()) != 1 {
		t.Fatalf("got %d messages back, want 1", len(list.GetMessages()))
	}

	got := list.GetMessages()[0]
	if got.GetMessageId() != created.GetMessageId() {
		t.Errorf("message_id: got %d, want %d", got.GetMessageId(), created.GetMessageId())
	}
	if !bytes.Equal(got.GetMessageData(), body) {
		t.Errorf("message_data: got %x, want %x (must be echoed verbatim)", got.GetMessageData(), body)
	}
	if got.GetPlayerPsnId() != "comradesean" {
		t.Errorf("player_psn_id: got %q, want %q", got.GetPlayerPsnId(), "comradesean")
	}
	if got.GetCellId() != 7 {
		t.Errorf("cell_id: got %d, want 7", got.GetCellId())
	}
}

// TestBloodMessageAreaAndCellFiltering guards the listing filter: a message must
// not leak into a different area, or into a cell the client did not ask about.
func TestBloodMessageAreaAndCellFiltering(t *testing.T) {
	svc, log, cs := testService(t)

	place := func(area, cell uint32) {
		raw, _ := proto.Marshal(&ds2pb.RequestCreateBloodMessage{
			OnlineAreaId: proto.Uint32(area),
			CellId:       proto.Uint32(cell),
			CharacterId:  proto.Uint32(1),
			MessageData:  []byte{0xAA},
		})
		if _, err := svc.handleCreateBloodMessage(log, cs, raw); err != nil {
			t.Fatal(err)
		}
	}
	place(100, 1)
	place(100, 2)
	place(200, 1)

	list := func(area, cell uint32) int {
		raw, _ := proto.Marshal(&ds2pb.RequestGetBloodMessageList{
			OnlineAreaId: proto.Uint32(area),
			MaxMessages:  proto.Uint32(10),
			SearchAreas: []*ds2pb.BloodMessageCellLimitData{
				{CellId: proto.Uint32(cell), MaxType_1: proto.Uint32(10), MaxType_2: proto.Uint32(10)},
			},
		})
		replyRaw, err := svc.handleGetBloodMessageList(log, cs, raw)
		if err != nil {
			t.Fatal(err)
		}
		var resp ds2pb.RequestGetBloodMessageListResponse
		if err := proto.Unmarshal(replyRaw, &resp); err != nil {
			t.Fatal(err)
		}
		return len(resp.GetMessages())
	}

	if n := list(100, 1); n != 1 {
		t.Errorf("area 100 cell 1: got %d, want 1", n)
	}
	if n := list(100, 2); n != 1 {
		t.Errorf("area 100 cell 2: got %d, want 1", n)
	}
	if n := list(200, 1); n != 1 {
		t.Errorf("area 200 cell 1: got %d, want 1", n)
	}
	if n := list(300, 1); n != 0 {
		t.Errorf("empty area 300: got %d, want 0", n)
	}
	if n := list(100, 9); n != 0 {
		t.Errorf("area 100 unrequested cell 9: got %d, want 0", n)
	}
}

// TestBloodMessageSelfEvaluationIgnored covers the one place we deliberately
// differ from the reference: it disconnects a client that rates its own message,
// we simply decline to count it.
func TestBloodMessageSelfEvaluationIgnored(t *testing.T) {
	svc, log, cs := testService(t)

	createRaw, _ := proto.Marshal(&ds2pb.RequestCreateBloodMessage{
		OnlineAreaId: proto.Uint32(100),
		CellId:       proto.Uint32(1),
		CharacterId:  proto.Uint32(1),
		MessageData:  []byte{0x01},
	})
	replyRaw, err := svc.handleCreateBloodMessage(log, cs, createRaw)
	if err != nil {
		t.Fatal(err)
	}
	var created ds2pb.RequestCreateBloodMessageResponse
	if err := proto.Unmarshal(replyRaw, &created); err != nil {
		t.Fatal(err)
	}
	id := created.GetMessageId()

	evalRaw, _ := proto.Marshal(&ds2pb.RequestEvaluateBloodMessage{
		OnlineAreaId: proto.Uint32(100),
		CellId:       proto.Uint32(1),
		MessageId:    proto.Uint32(id),
	})
	if _, err := svc.handleEvaluateBloodMessage(log, cs, evalRaw); err != nil {
		t.Fatal(err)
	}
	if m, _, _ := svc.store.GetBloodMessage(context.Background(), id); m.Rating != 0 {
		t.Errorf("self-evaluation counted: rating %d, want 0", m.Rating)
	}

	// Another player rating it does count.
	other := &clientSession{accountID: "someone-else", playerID: 2}
	if _, err := svc.handleEvaluateBloodMessage(log, other, evalRaw); err != nil {
		t.Fatal(err)
	}
	if m, _, _ := svc.store.GetBloodMessage(context.Background(), id); m.Rating != 1 {
		t.Errorf("rating after another player evaluated: got %d, want 1", m.Rating)
	}

	// And the query reflects it.
	qRaw, _ := proto.Marshal(&ds2pb.RequestGetBloodMessageEvaluation{
		OnlineAreaId: proto.Uint32(100),
		CellId:       proto.Uint32(1),
		MessageId:    proto.Uint32(id),
	})
	qReplyRaw, err := svc.handleGetBloodMessageEvaluation(log, cs, qRaw)
	if err != nil {
		t.Fatal(err)
	}
	var q ds2pb.RequestGetBloodMessageEvaluationResponse
	if err := proto.Unmarshal(qReplyRaw, &q); err != nil {
		t.Fatal(err)
	}
	if q.GetRating() != 1 {
		t.Errorf("queried rating: got %d, want 1", q.GetRating())
	}
}

// TestBloodMessageRemove checks a removed message stops appearing in listings.
func TestBloodMessageRemove(t *testing.T) {
	svc, log, cs := testService(t)

	createRaw, _ := proto.Marshal(&ds2pb.RequestCreateBloodMessage{
		OnlineAreaId: proto.Uint32(100),
		CellId:       proto.Uint32(1),
		CharacterId:  proto.Uint32(1),
		MessageData:  []byte{0x01},
	})
	replyRaw, _ := svc.handleCreateBloodMessage(log, cs, createRaw)
	var created ds2pb.RequestCreateBloodMessageResponse
	if err := proto.Unmarshal(replyRaw, &created); err != nil {
		t.Fatal(err)
	}

	rmRaw, _ := proto.Marshal(&ds2pb.RequestRemoveBloodMessage{
		OnlineAreaId: proto.Uint32(100),
		CellId:       proto.Uint32(1),
		MessageId:    proto.Uint32(created.GetMessageId()),
	})
	if _, err := svc.handleRemoveBloodMessage(log, cs, rmRaw); err != nil {
		t.Fatal(err)
	}

	listRaw, _ := proto.Marshal(&ds2pb.RequestGetBloodMessageList{
		OnlineAreaId: proto.Uint32(100),
		MaxMessages:  proto.Uint32(10),
		SearchAreas: []*ds2pb.BloodMessageCellLimitData{
			{CellId: proto.Uint32(1), MaxType_1: proto.Uint32(10), MaxType_2: proto.Uint32(10)},
		},
	})
	listReplyRaw, err := svc.handleGetBloodMessageList(log, cs, listRaw)
	if err != nil {
		t.Fatal(err)
	}
	var list ds2pb.RequestGetBloodMessageListResponse
	if err := proto.Unmarshal(listReplyRaw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.GetMessages()) != 0 {
		t.Errorf("removed message still listed: got %d messages", len(list.GetMessages()))
	}
}
