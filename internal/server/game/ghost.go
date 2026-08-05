package game

import (
	"fmt"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Ghost opcodes. Both are request/response: the client retries them until
// answered, and will not open other online UI (writing a message, for one) while
// it has an outstanding request. Confirmed live 2026-08-05 by a client retrying
// an identical 1367-byte RequestCreateGhostData twice.
const (
	opRequestCreateGhostData  uint32 = 0x03B1
	opRequestGetGhostDataList uint32 = 0x03B2
)

// ghost is one recorded replay. data is the game's own opaque encoding of the
// movement recording; the server stores and echoes it without interpreting it.
type ghost struct {
	id     uint32
	areaID uint32
	cellID uint32
	data   []byte
}

// ghostStore holds ghosts in memory.
//
// The reference keeps these memory-only by default rather than persisting them,
// and they are inherently transient, so that is what we do too.
type ghostStore struct {
	mu     sync.Mutex
	nextID uint32
	byID   map[uint32]*ghost
}

func newGhostStore() *ghostStore {
	return &ghostStore{byID: make(map[uint32]*ghost)}
}

func (s *ghostStore) add(g *ghost) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	g.id = s.nextID
	s.byID[g.id] = g
	return g.id
}

// inCells returns up to limit ghosts in the area, restricted to the requested
// cells. An empty cells map means any cell in the area.
func (s *ghostStore) inCells(areaID uint32, cells map[uint32]bool, limit int) []*ghost {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*ghost
	for _, g := range s.byID {
		if g.areaID != areaID {
			continue
		}
		if len(cells) > 0 && !cells[g.cellID] {
			continue
		}
		out = append(out, g)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Service) handleCreateGhostData(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestCreateGhostData
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestCreateGhostData: %w", err)
	}

	g := &ghost{
		areaID: req.GetOnlineAreaId(),
		cellID: req.GetCellId(),
		data:   append([]byte(nil), req.GetData()...),
	}
	id := s.ghosts.add(g)

	log.Info("ghost recorded",
		"player_id", cs.playerID, "ghost_id", id,
		"area_id", g.areaID, "cell_id", g.cellID, "data_bytes", len(g.data))

	// The response is empty, but it must still be SENT — this is a
	// request/response message and the client retries until it gets a reply.
	return proto.Marshal(&ds2pb.RequestCreateGhostDataResponse{})
}

func (s *Service) handleGetGhostDataList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetGhostDataList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetGhostDataList: %w", err)
	}

	cells := make(map[uint32]bool, len(req.GetSearchAreas()))
	for _, a := range req.GetSearchAreas() {
		cells[a.GetCellId()] = true
	}
	found := s.ghosts.inCells(req.GetOnlineAreaId(), cells, int(req.GetMaxGhosts()))

	items := make([]*ds2pb.GhostData, 0, len(found))
	for _, g := range found {
		items = append(items, &ds2pb.GhostData{
			CellId:  proto.Uint32(g.cellID),
			GhostId: proto.Uint32(g.id),
			Data:    g.data,
		})
	}

	log.Info("ghost list",
		"player_id", cs.playerID, "area_id", req.GetOnlineAreaId(),
		"cells_requested", len(cells), "max", req.GetMaxGhosts(), "returned", len(items))

	// Note the wire tag: `ghosts` is field 3 and field 2 is skipped. The
	// generated code has this right; hand-rolling it as field 2 would produce a
	// message the client silently ignores.
	resp := &ds2pb.RequestGetGhostDataListResponse{
		OnlineAreaId: proto.Uint32(req.GetOnlineAreaId()),
		Ghosts:       items,
	}
	return proto.Marshal(resp)
}
