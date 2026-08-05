package game

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Bloodstain opcodes.
//
// 0x0391 is the one message in this cluster that registers NO response callback
// — the only such message the PC reference also agrees on. The other two are
// request/response and the client retries them until answered.
const (
	opRequestCreateBloodstain  uint32 = 0x0391
	opRequestGetBloodstainList uint32 = 0x0392
	opRequestGetDeadingGhost   uint32 = 0x0393
)

// bloodstain is one death marker. Both blobs are the game's own opaque encodings:
// data is what the stain shows, ghostData is the replay of the death that another
// player watches when they touch it.
type bloodstain struct {
	id        uint32
	areaID    uint32
	cellID    uint32
	data      []byte
	ghostData []byte
}

// bloodstainStore holds death markers in memory. Memory-only, matching the
// reference's default for this feature.
type bloodstainStore struct {
	mu     sync.Mutex
	nextID uint32
	byID   map[uint32]*bloodstain
}

func newBloodstainStore() *bloodstainStore {
	return &bloodstainStore{byID: make(map[uint32]*bloodstain)}
}

func (s *bloodstainStore) add(b *bloodstain) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	b.id = s.nextID
	s.byID[b.id] = b
	return b.id
}

func (s *bloodstainStore) get(id uint32) (*bloodstain, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.byID[id]
	return b, ok
}

// inCells returns up to limit stains in the area, restricted to the requested
// cells. An empty cells map means any cell in the area.
func (s *bloodstainStore) inCells(areaID uint32, cells map[uint32]bool, limit int) []*bloodstain {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*bloodstain
	for _, b := range s.byID {
		if b.areaID != areaID {
			continue
		}
		if len(cells) > 0 && !cells[b.cellID] {
			continue
		}
		out = append(out, b)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// handleCreateBloodstain records a death marker. Returns no reply: this opcode
// registers no response callback in the client.
func (s *Service) handleCreateBloodstain(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestCreateBloodstain
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestCreateBloodstain: %w", err)
	}

	b := &bloodstain{
		areaID:    req.GetOnlineAreaId(),
		cellID:    req.GetCellId(),
		data:      append([]byte(nil), req.GetData()...),
		ghostData: append([]byte(nil), req.GetGhostData()...),
	}
	id := s.bloodstains.add(b)

	log.Info("bloodstain recorded",
		"player_id", cs.playerID, "bloodstain_id", id,
		"area_id", b.areaID, "cell_id", b.cellID,
		"data_bytes", len(b.data), "ghost_bytes", len(b.ghostData))
	return nil, nil
}

func (s *Service) handleGetBloodstainList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetBloodstainList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetBloodstainList: %w", err)
	}

	cells := make(map[uint32]bool, len(req.GetSearchAreas()))
	for _, a := range req.GetSearchAreas() {
		cells[a.GetCellId()] = true
	}
	found := s.bloodstains.inCells(req.GetOnlineAreaId(), cells, int(req.GetMaxStains()))

	items := make([]*ds2pb.BloodstainInfo, 0, len(found))
	for _, b := range found {
		items = append(items, &ds2pb.BloodstainInfo{
			OnlineAreaId: proto.Uint32(b.areaID),
			CellId:       proto.Uint32(b.cellID),
			BloodstainId: proto.Uint32(b.id),
			Data:         b.data,
		})
	}

	log.Info("bloodstain list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cells_requested", len(cells), "max", req.GetMaxStains(), "returned", len(items))

	resp := &ds2pb.RequestGetBloodstainListResponse{Bloodstains: items}
	return proto.Marshal(resp)
}

// handleGetDeadingGhost returns the death replay attached to a bloodstain.
//
// An unknown id still gets a reply, with empty data — the reference does the same.
// Returning nothing would leave the client retrying forever over a stain that has
// simply expired from memory.
func (s *Service) handleGetDeadingGhost(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetDeadingGhost
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetDeadingGhost: %w", err)
	}

	id := req.GetBloodstainId()
	var data []byte
	if b, ok := s.bloodstains.get(id); ok {
		data = b.ghostData
	}
	log.Info("deading ghost requested",
		"player_id", cs.playerID, "bloodstain_id", id, "found", data != nil,
		"data_bytes", len(data))

	resp := &ds2pb.RequestGetDeadingGhostResponse{
		OnlineAreaId: proto.Uint32(req.GetOnlineAreaId()),
		CellId:       proto.Uint32(req.GetCellId()),
		BloodstainId: proto.Uint32(id),
		Data:         data,
	}
	return proto.Marshal(resp)
}
