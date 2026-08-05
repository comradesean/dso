package game

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
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

// bloodMessage is one placed message.
//
// data is the game's own opaque encoding of the message text (template ids,
// conjunction, gesture). The server never interprets it — it is stored and echoed
// back verbatim, exactly as the reference does.
type bloodMessage struct {
	id          uint32
	playerID    uint32
	characterID uint32
	accountID   string
	areaID      uint32
	cellID      uint32
	data        []byte
	rating      int64 // net score; each evaluation is +1
}

// bloodMessageStore holds placed messages in memory.
//
// Memory-only for now, which mirrors the reference's default for this feature.
// Persistence lands with the store work; the interface here is deliberately
// small so swapping in a database is a local change.
type bloodMessageStore struct {
	mu     sync.Mutex
	nextID uint32
	byID   map[uint32]*bloodMessage
}

func newBloodMessageStore() *bloodMessageStore {
	return &bloodMessageStore{byID: make(map[uint32]*bloodMessage)}
}

// add stores a message and assigns it an id.
func (s *bloodMessageStore) add(m *bloodMessage) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	m.id = s.nextID
	s.byID[m.id] = m
	return m.id
}

// reentry re-registers a message the client already holds locally, so it stays
// visible after a reconnect. Returns false if the id is unknown to us — which is
// the normal case for a fresh server, since the client remembers messages across
// sessions but our store does not (yet).
func (s *bloodMessageStore) reentry(id uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.byID[id]
	return ok
}

func (s *bloodMessageStore) remove(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
}

func (s *bloodMessageStore) get(id uint32) (*bloodMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[id]
	return m, ok
}

// evaluate records a rating and returns the new total.
func (s *bloodMessageStore) evaluate(id uint32) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.byID[id]
	if !ok {
		return 0, false
	}
	m.rating++
	return m.rating, true
}

// inCells returns up to limit messages placed in the given area, restricted to
// the requested cells. A nil or empty cells slice means "any cell in the area".
func (s *bloodMessageStore) inCells(areaID uint32, cells map[uint32]bool, limit int) []*bloodMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*bloodMessage
	for _, m := range s.byID {
		if m.areaID != areaID {
			continue
		}
		if len(cells) > 0 && !cells[m.cellID] {
			continue
		}
		out = append(out, m)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// toProto converts a stored message to its wire form.
func (m *bloodMessage) toProto() *ds2pb.BloodMessageData {
	return &ds2pb.BloodMessageData{
		PlayerId:      proto.Uint32(m.playerID),
		CharacterId:   proto.Uint32(m.characterID),
		MessageId:     proto.Uint32(m.id),
		Good:          proto.Uint32(uint32(max64(m.rating, 0))),
		MessageData:   m.data,
		PlayerSteamId: proto.String(m.accountID),
		CellId:        proto.Uint32(m.cellID),
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

	m := &bloodMessage{
		playerID:    cs.playerID,
		characterID: req.GetCharacterId(),
		accountID:   cs.accountID,
		areaID:      req.GetOnlineAreaId(),
		cellID:      req.GetCellId(),
		data:        append([]byte(nil), req.GetMessageData()...),
	}
	id := s.messages.add(m)

	log.Info("blood message created",
		"player_id", cs.playerID, "message_id", id,
		"area_id", m.areaID, "cell_id", m.cellID, "data_bytes", len(m.data))

	resp := &ds2pb.RequestCreateBloodMessageResponse{MessageId: proto.Uint32(id)}
	return proto.Marshal(resp)
}

func (s *Service) handleGetBloodMessageList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetBloodMessageList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetBloodMessageList: %w", err)
	}

	cells := make(map[uint32]bool, len(req.GetSearchAreas()))
	for _, a := range req.GetSearchAreas() {
		cells[a.GetCellId()] = true
	}
	found := s.messages.inCells(req.GetOnlineAreaId(), cells, int(req.GetMaxMessages()))

	items := make([]*ds2pb.BloodMessageData, 0, len(found))
	for _, m := range found {
		items = append(items, m.toProto())
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
	known := s.messages.reentry(req.GetMessageId())
	log.Info("blood message reentry",
		"player_id", cs.playerID, "message_id", req.GetMessageId(), "known", known)
	return proto.Marshal(&ds2pb.RequestReentryBloodMessageResponse{})
}

func (s *Service) handleRemoveBloodMessage(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestRemoveBloodMessage
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestRemoveBloodMessage: %w", err)
	}
	s.messages.remove(req.GetMessageId())
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
	if m, ok := s.messages.get(id); ok && m.playerID == cs.playerID {
		log.Info("ignoring self-evaluation", "player_id", cs.playerID, "message_id", id)
		return proto.Marshal(&ds2pb.RequestEvaluateBloodMessageResponse{})
	}

	rating, ok := s.messages.evaluate(id)
	log.Info("blood message evaluated",
		"player_id", cs.playerID, "message_id", id, "known", ok, "rating", rating)

	// The reference pushes PushRequestEvaluateBloodMessage to the author here so
	// they see the praise live. Not wired up yet — pushes are unverified on PS3
	// (see docs/protocol-map-ps3.md).
	return proto.Marshal(&ds2pb.RequestEvaluateBloodMessageResponse{})
}

func (s *Service) handleGetBloodMessageEvaluation(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetBloodMessageEvaluation
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetBloodMessageEvaluation: %w", err)
	}
	id := req.GetMessageId()
	var rating int64
	if m, ok := s.messages.get(id); ok {
		rating = m.rating
	}
	log.Info("blood message evaluation queried",
		"player_id", cs.playerID, "message_id", id, "rating", rating)

	resp := &ds2pb.RequestGetBloodMessageEvaluationResponse{
		MessageId: proto.Int64(int64(id)),
		Rating:    proto.Int64(rating),
	}
	return proto.Marshal(resp)
}
