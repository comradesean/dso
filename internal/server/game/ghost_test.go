package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// TestGhostListResponseWireTags pins the field numbers of the ghost list reply
// at the WIRE level, not through the generated accessors.
//
// This exists because the field number was wrong for the entire life of the
// feature and nothing caught it. `ghosts` was written as field 3, taken from the
// PC reference proto; on PS3 it is field 2. The client's parser tests only
// fields 1 and 2 and sends anything else to SkipField, so every list was
// discarded whole — eighty ghosts recorded, lists returning up to eight, and not
// one ever seen in game. A code comment in this package asserted, confidently
// and incorrectly, that field 3 was right and field 2 would be ignored.
//
// A test through the Go API cannot catch that: the generated struct would
// happily round-trip whatever number the proto declares. Only the bytes tell the
// truth, so this asserts on the bytes.
//
// Recovered from BLUS41045: parser v1.10 0x1685110, serialiser 0x1634668,
// identical in v1.00. The working RequestGetBloodMessageListResponse has the
// same {1 uint32, 2 repeated} shape, which is why messages rendered and ghosts
// did not.
func TestGhostListResponseWireTags(t *testing.T) {
	raw, err := proto.Marshal(&ds2pb.RequestGetGhostDataListResponse{
		OnlineAreaId: proto.Uint32(1),
		Ghosts: []*ds2pb.GhostData{{
			CellId:  proto.Uint32(2),
			GhostId: proto.Uint32(3),
			Data:    []byte{0xAA},
		}},
	})
	if err != nil {
		t.Fatalf("the reply we send does not marshal: %v", err)
	}

	// Field 1, varint: tag = 1<<3 | 0 = 0x08, then the value.
	if len(raw) < 3 || raw[0] != 0x08 || raw[1] != 0x01 {
		t.Fatalf("online_area_id is not field 1 varint: % x", raw)
	}
	// Field 2, length-delimited: tag = 2<<3 | 2 = 0x12. Field 3 would be 0x1a,
	// which is the value that made the client discard the list.
	if raw[2] != 0x12 {
		t.Errorf("ghosts tag = %#02x, want 0x12 (field 2). %#02x means field 3, "+
			"which the PS3 client skips — every ghost is dropped. bytes: % x",
			raw[2], 0x1a, raw)
	}
}

// TestGhostListResponseRequiresArea — online_area_id is `required`, and the
// client stamps it into every ghost record it builds from the reply. Omitting it
// makes the whole message fail the client's IsInitialized check, dropping the
// list exactly as a wrong tag would.
func TestGhostListResponseRequiresArea(t *testing.T) {
	_, err := proto.Marshal(&ds2pb.RequestGetGhostDataListResponse{
		Ghosts: []*ds2pb.GhostData{{
			CellId:  proto.Uint32(2),
			GhostId: proto.Uint32(3),
			Data:    []byte{0xAA},
		}},
	})
	if err == nil {
		t.Error("marshalled a reply with no online_area_id; proto2 required " +
			"fields are genuinely enforced and the client would drop this")
	}
}

// TestGhostPerCellCaps — the client asks for a few ghosts PER CELL across ~27
// cells with a smaller overall maximum. Ignoring the per-cell figure let one
// busy cell consume the whole quota and starve every other one, which is
// visible in game as phantoms clustering in a single spot.
func TestGhostPerCellCaps(t *testing.T) {
	st := newGhostStore()
	// Six recordings in one cell, one in another.
	for i := 0; i < 6; i++ {
		st.add(&ghost{ownerID: 1, areaID: 10, cellID: 100, data: []byte{1}})
	}
	st.add(&ghost{ownerID: 1, areaID: 10, cellID: 200, data: []byte{1}})

	// Cap of 2 per cell, 10 overall: the crowded cell must not swamp the other.
	got := st.inCells(10, map[uint32]int{100: 2, 200: 2}, 0, 10)
	byCell := map[uint32]int{}
	for _, g := range got {
		byCell[g.cellID]++
	}
	if byCell[100] != 2 {
		t.Errorf("cell 100 returned %d, want 2 (its cap)", byCell[100])
	}
	if byCell[200] != 1 {
		t.Errorf("cell 200 returned %d, want 1 — the crowded cell starved it",
			byCell[200])
	}

	// A cap of 0 means no per-cell limit, not "none from here".
	got = st.inCells(10, map[uint32]int{100: 0, 200: 0}, 0, 10)
	if len(got) != 7 {
		t.Errorf("cap 0 returned %d, want all 7; 0 must not be read as a ban", len(got))
	}

	// The overall limit still applies on top.
	if got = st.inCells(10, map[uint32]int{100: 5, 200: 5}, 0, 3); len(got) != 3 {
		t.Errorf("overall limit ignored: got %d, want 3", len(got))
	}
}

// TestGhostDataWireTags pins the element shape too — all three fields are
// `required` on PS3, and the response's IsInitialized walks every element, so a
// single element missing a single field silently voids the entire list.
func TestGhostDataWireTags(t *testing.T) {
	raw, err := proto.Marshal(&ds2pb.GhostData{
		CellId:  proto.Uint32(1),
		GhostId: proto.Uint32(1),
		Data:    []byte{0xAA},
	})
	if err != nil {
		t.Fatal(err)
	}
	// 1 varint (0x08), 2 varint (0x10), 3 bytes (0x1a).
	want := []byte{0x08, 0x01, 0x10, 0x01, 0x1a, 0x01, 0xAA}
	if len(raw) != len(want) {
		t.Fatalf("GhostData = % x, want % x", raw, want)
	}
	for i := range want {
		if raw[i] != want[i] {
			t.Fatalf("GhostData = % x, want % x", raw, want)
		}
	}
}
