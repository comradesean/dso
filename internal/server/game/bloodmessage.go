package game

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
	"github.com/sstreight/dso/internal/server/store"
)

// Blood message opcodes. All decomp-confirmed for PS3; see docs/protocol-map-ps3.md.
const (
	opRequestReentryBloodMessage       uint32 = 0x03AD
	opRequestCreateBloodMessage        uint32 = 0x03AB
	opRequestRemoveBloodMessage        uint32 = 0x03AC
	opRequestGetBloodMessageList       uint32 = 0x03AE
	opRequestEvaluateBloodMessage      uint32 = 0x03AF
	opRequestGetBloodMessageEvaluation uint32 = 0x03B0
)

// toProto converts a stored message to its wire form.
func bloodMessageToProto(m *store.BloodMessage) *ds2pb.BloodMessageData {
	return &ds2pb.BloodMessageData{
		PlayerId:    proto.Uint32(m.PlayerID),
		CharacterId: proto.Uint32(m.CharacterID),
		MessageId:   proto.Uint32(m.ID),
		Good:        proto.Uint32(uint32(max64(m.Rating, 0))),
		MessageData: m.Data,
		PlayerPsnId: proto.String(m.AccountID),
		CellId:      proto.Uint32(m.CellID),
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (s *Service) handleCreateBloodMessage(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestCreateBloodMessage
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestCreateBloodMessage: %w", err)
	}

	m := &store.BloodMessage{
		PlayerID:    cs.playerID,
		CharacterID: req.GetCharacterId(),
		AccountID:   cs.accountID,
		AreaID:      req.GetOnlineAreaId(),
		CellID:      req.GetCellId(),
		Data:        append([]byte(nil), req.GetMessageData()...),
	}
	id, err := s.store.AddBloodMessage(context.Background(), m)
	if err != nil {
		return nil, err
	}

	log.Info("blood message created",
		"player_id", cs.playerID, "message_id", id,
		"area_id", m.AreaID, "cell_id", m.CellID, "data_bytes", len(m.Data))

	resp := &ds2pb.RequestCreateBloodMessageResponse{MessageId: proto.Uint32(id)}
	return proto.Marshal(resp)
}

func (s *Service) handleGetBloodMessageList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetBloodMessageList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetBloodMessageList: %w", err)
	}

	cells := make([]uint32, 0, len(req.GetSearchAreas()))
	for _, a := range req.GetSearchAreas() {
		cells = append(cells, a.GetCellId())
	}
	found, err := s.store.BloodMessagesInCells(
		context.Background(), req.GetOnlineAreaId(), cells, int(req.GetMaxMessages()))
	if err != nil {
		return nil, err
	}

	items := make([]*ds2pb.BloodMessageData, 0, len(found))
	for _, m := range found {
		items = append(items, bloodMessageToProto(m))
	}

	log.Info("blood message list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cells_requested", len(cells), "max", req.GetMaxMessages(), "returned", len(items))

	resp := &ds2pb.RequestGetBloodMessageListResponse{
		OnlineAreaId: proto.Uint32(req.GetOnlineAreaId()),
		Messages:     items,
	}
	return proto.Marshal(resp)
}

// handleReentryBloodMessage answers the client re-registering messages it still
// holds locally. We reply with an empty body either way: the client remembers its
// own messages across sessions but our store does not yet, so an unknown id is
// expected rather than an error.
func (s *Service) handleReentryBloodMessage(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestReentryBloodMessage
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestReentryBloodMessage: %w", err)
	}
	_, known, err := s.store.GetBloodMessage(context.Background(), req.GetMessageId())
	if err != nil {
		return nil, err
	}
	log.Info("blood message reentry",
		"player_id", cs.playerID, "message_id", req.GetMessageId(), "known", known)
	return proto.Marshal(&ds2pb.RequestReentryBloodMessageResponse{})
}

func (s *Service) handleRemoveBloodMessage(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRemoveBloodMessage
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRemoveBloodMessage: %w", err)
	}
	if err := s.store.RemoveBloodMessage(context.Background(), req.GetMessageId()); err != nil {
		return nil, err
	}
	log.Info("blood message removed", "player_id", cs.playerID, "message_id", req.GetMessageId())
	return proto.Marshal(&ds2pb.RequestRemoveBloodMessageResponse{})
}

// handleEvaluateBloodMessage records a "praise" on someone else's message.
//
// The reference disconnects a client that rates its own message. We only decline
// to count it: a self-rate is more likely a quirk of our single-client test setup
// than an attack, and dropping the session would be a hostile response to
// something harmless.
func (s *Service) handleEvaluateBloodMessage(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestEvaluateBloodMessage
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestEvaluateBloodMessage: %w", err)
	}

	id := req.GetMessageId()
	m, found, err := s.store.GetBloodMessage(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if found && m.PlayerID == cs.playerID {
		log.Info("ignoring self-evaluation", "player_id", cs.playerID, "message_id", id)
		return proto.Marshal(&ds2pb.RequestEvaluateBloodMessageResponse{})
	}

	rating, ok, err := s.store.EvaluateBloodMessage(context.Background(), id)
	if err != nil {
		return nil, err
	}
	log.Info("blood message evaluated",
		"player_id", cs.playerID, "message_id", id, "known", ok, "rating", rating)

	if ok {
		s.pushEvaluationToAuthor(log, cs, id)
	}
	return proto.Marshal(&ds2pb.RequestEvaluateBloodMessageResponse{})
}

// pushEvaluationToAuthor tells the message's author that someone praised it, so
// their client updates without having to re-query.
//
// FIRST USE OF THE PUSH MECHANISM ON PS3. The transport is the PC model — msg_type
// 0x0320 with msg_index 0xFFFFFFFF, the actual identity carried in the protobuf's
// first field — which decompilation could NOT confirm for PS3: the client's
// dispatcher (vaddr 0x158C138) keys on a u32 that could come from either the
// transport header or a parsed field, and both models fit the evidence.
//
// PushMessageId 938 (0x3AA) is confirmed though: it matches the push handler the
// client registers at 0x158ECEC. So if the model is right this should work, and if
// nothing reaches the author's client then the transport is the thing to question,
// not the id. Either outcome is informative, which is why this is a good first
// push to try — a wrong guess here is harmless, unlike in the summon path.
//
// Caller must hold s.mu (drain does).
func (s *Service) pushEvaluationToAuthor(log logger, rater *clientSession, messageID uint32) {
	m, ok, err := s.store.GetBloodMessage(context.Background(), messageID)
	if err != nil || !ok {
		return
	}
	author, live := s.sessionForPlayerLocked(m.PlayerID)
	if !live {
		log.Debug("evaluation not pushed: author is offline",
			"author_player_id", m.PlayerID, "message_id", messageID)
		return
	}
	if author == rater {
		return
	}

	push := &ds2pb.PushRequestEvaluateBloodMessage{
		PushMessageId: ds2pb.PushMessageId_PushID_PushRequestEvaluateBloodMessage.Enum(),
		PlayerId:      proto.Uint32(rater.playerID),
		MessageId:     proto.Uint32(messageID),
		PlayerPsnId:   proto.String(rater.accountID),
	}
	body, err := proto.Marshal(push)
	if err != nil {
		log.Warn("failed to build evaluation push", "err", err)
		return
	}
	author.conn.SendPush(body)
	log.Info("pushed evaluation to author",
		"author_player_id", m.PlayerID, "rater_player_id", rater.playerID,
		"message_id", messageID, "payload_bytes", len(body))
}

// sessionForPlayerLocked finds a live session by player id. Caller must hold s.mu.
func (s *Service) sessionForPlayerLocked(playerID uint32) (*clientSession, bool) {
	for _, cs := range s.sessions {
		if cs.playerID == playerID {
			return cs, true
		}
	}
	return nil, false
}

func (s *Service) handleGetBloodMessageEvaluation(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetBloodMessageEvaluation
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetBloodMessageEvaluation: %w", err)
	}
	id := req.GetMessageId()
	var rating int64
	if m, ok, err := s.store.GetBloodMessage(context.Background(), id); err == nil && ok {
		rating = m.Rating
	}
	log.Info("blood message evaluation queried",
		"player_id", cs.playerID, "message_id", id, "rating", rating)

	resp := &ds2pb.RequestGetBloodMessageEvaluationResponse{
		MessageId: proto.Int64(int64(id)),
		Rating:    proto.Int64(rating),
	}
	return proto.Marshal(resp)
}
