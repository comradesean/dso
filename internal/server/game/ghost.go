package game

import (
	"fmt"
	"sort"
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

// firstGhostID keeps ghost ids clear of low numbers, for the same reason message
// and sign ids do: the client caches by server-assigned id, and ghosts are
// memory-only so numbering would otherwise restart at 1 on every server restart
// and hand a fresh recording an id a client already believes it has.
const firstGhostID = 100000

// maxGhostsPerArea bounds how many recordings are kept for one area.
//
// Ghosts are transient by nature and the store was previously unbounded, so a
// long session accumulated thousands and a listing returned an arbitrary handful
// of mostly-stale ones. Keeping a recent window is both closer to the intent and
// what makes a listing useful.
const maxGhostsPerArea = 64

// ghost is one recorded replay. data is the game's own opaque encoding of the
// movement recording; the server stores and echoes it without interpreting it.
type ghost struct {
	id uint32
	// ownerID is who recorded it. Needed so a player is not shown their own
	// ghosts: they are the one thing guaranteed to tell them nothing, and with
	// few players online they crowd out everyone else's.
	ownerID uint32
	areaID  uint32
	cellID  uint32
	data    []byte
	// seq orders recordings so a listing can prefer recent ones rather than
	// whatever Go's randomised map iteration happens to yield.
	seq uint64
}

// ghostStore holds ghosts in memory.
//
// The reference keeps these memory-only by default rather than persisting them,
// and they are inherently transient, so that is what we do too.
type ghostStore struct {
	mu     sync.Mutex
	nextID uint32
	seq    uint64
	byID   map[uint32]*ghost
}

func newGhostStore() *ghostStore {
	return &ghostStore{nextID: firstGhostID - 1, byID: make(map[uint32]*ghost)}
}

func (s *ghostStore) add(g *ghost) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	s.seq++
	g.id = s.nextID
	g.seq = s.seq
	s.byID[g.id] = g
	s.evictLocked(g.areaID)
	return g.id
}

// evictLocked drops the oldest recordings once an area exceeds its window.
func (s *ghostStore) evictLocked(areaID uint32) {
	var inArea []*ghost
	for _, g := range s.byID {
		if g.areaID == areaID {
			inArea = append(inArea, g)
		}
	}
	if len(inArea) <= maxGhostsPerArea {
		return
	}
	sort.Slice(inArea, func(i, j int) bool { return inArea[i].seq < inArea[j].seq })
	for _, g := range inArea[:len(inArea)-maxGhostsPerArea] {
		delete(s.byID, g.id)
	}
}

// inCells returns ghosts in the area, excluding the viewer's own, honouring BOTH
// the per-cell caps the client sends and the overall limit. An empty cells map
// means any cell.
//
// Newest first. Go randomises map iteration, so taking the first `limit` hits
// previously returned an arbitrary sample of the store.
//
// It honours BOTH the per-cell caps the
// client sends and the overall limit.
//
// cells maps cell id to that cell's cap — CellLimitData.max_items, which was
// previously discarded by collapsing the request into a set. The client asks for
// a handful per cell across ~27 cells with a smaller overall maximum, so
// ignoring the per-cell figure let a single busy cell consume the entire quota
// and starve every other one. That matters for where phantoms actually appear:
// the game shows more detailed phantoms around bonfires, and a bonfire cell that
// loses its share to a neighbour simply shows nothing.
//
// A cap of 0 is treated as "no per-cell limit" rather than "none from this
// cell". Reading it literally would be the stricter interpretation, and every
// time this project has guessed toward strictness it has silently broken a
// working feature.
func (s *ghostStore) inCells(areaID uint32, cells map[uint32]int, excludePlayer uint32, limit int) []*ghost {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*ghost
	for _, g := range s.byID {
		if g.areaID != areaID || g.ownerID == excludePlayer {
			continue
		}
		if len(cells) > 0 {
			if _, ok := cells[g.cellID]; !ok {
				continue
			}
		}
		out = append(out, g)
	}
	// Newest first, so the per-cell caps keep the most recent recordings.
	sort.Slice(out, func(i, j int) bool { return out[i].seq > out[j].seq })

	perCell := make(map[uint32]int, len(cells))
	kept := out[:0]
	for _, g := range out {
		if cap, ok := cells[g.cellID]; ok && cap > 0 && perCell[g.cellID] >= cap {
			continue
		}
		perCell[g.cellID]++
		kept = append(kept, g)
		if limit > 0 && len(kept) >= limit {
			break
		}
	}
	return kept
}

func (s *Service) handleCreateGhostData(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestCreateGhostData
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestCreateGhostData: %w", err)
	}

	g := &ghost{
		ownerID: cs.playerID,
		areaID:  req.GetOnlineAreaId(),
		cellID:  req.GetCellId(),
		data:    append([]byte(nil), req.GetData()...),
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

	// cell id -> that cell's cap. max_items is the client's per-cell limit and
	// was previously thrown away by storing a bare set.
	cells := make(map[uint32]int, len(req.GetSearchAreas()))
	for _, a := range req.GetSearchAreas() {
		cells[a.GetCellId()] = int(a.GetMaxItems())
	}
	found := s.ghosts.inCells(req.GetOnlineAreaId(), cells, cs.playerID, int(req.GetMaxGhosts()))

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

	// `ghosts` is field 2. An earlier comment here asserted it was field 3 and
	// that field 2 "would produce a message the client silently ignores" —
	// precisely backwards, and it was the reason no ghost was ever seen in game.
	// The client's parser tests only fields 1 and 2 and skips everything else,
	// so every list we sent was discarded whole. See the proto for the addresses.
	//
	// online_area_id is `required` and is not decorative: the client stamps it
	// into every ghost record it builds from this reply.
	resp := &ds2pb.RequestGetGhostDataListResponse{
		OnlineAreaId: proto.Uint32(req.GetOnlineAreaId()),
		Ghosts:       items,
	}
	return proto.Marshal(resp)
}
