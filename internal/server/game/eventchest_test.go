package game

import (
	"encoding/binary"
	"os"
	"testing"
)

const (
	stockLotParam    = "../../../data/regpush/stock/ItemLotParam2_SvrEvent.param"
	stockOnlineParam = "../../../data/regpush/stock/OnlineEventParam.param"
)

// TestEventChestPayloadsKeepTheirSize guards the constraint that fails silently
// on the console: the client compares a pushed param's size against the loaded
// resource's and discards any mismatch without a word (0x770DE4).
//
// Editing in place cannot change a length, which is the point — but nothing
// stops a future change from rebuilding a param instead, and the console would
// not tell us. So the invariant is asserted here where it is loud.
func TestEventChestPayloadsKeepTheirSize(t *testing.T) {
	lot := mustReadOrSkip(t, stockLotParam)
	before := len(lot)

	if err := setParamRowU32(lot, eventChestLotRow, eventChestItemOffset, 5610000); err != nil {
		t.Fatalf("set item: %v", err)
	}
	if len(lot) != before {
		t.Fatalf("lot param changed length: %d -> %d", before, len(lot))
	}

	online := mustReadOrSkip(t, stockOnlineParam)
	before = len(online)
	if err := setParamRowU16(online, 0, eventChestThresholdOffset, 7); err != nil {
		t.Fatalf("set threshold: %v", err)
	}
	if len(online) != before {
		t.Fatalf("online event param changed length: %d -> %d", before, len(online))
	}
}

// TestEventChestFieldOffsets pins the two offsets against the real files.
//
// The item offset is not a guess: the stock row holds 0x0399EFA0 = 60420000, and
// the chest handed over a Torch on 2026-08-06. If a future edit moves this
// constant, that live evidence is what it would be contradicting.
func TestEventChestFieldOffsets(t *testing.T) {
	lot := mustReadOrSkip(t, stockLotParam)
	row, err := paramRowOffset(lot, eventChestLotRow)
	if err != nil {
		t.Fatalf("row %d not found: %v", eventChestLotRow, err)
	}
	if got := binary.BigEndian.Uint32(lot[row+eventChestItemOffset:]); got != 60420000 {
		t.Errorf("stock item id = %d, want 60420000 (Torch, confirmed by live claim)", got)
	}

	online := mustReadOrSkip(t, stockOnlineParam)
	row, err = paramRowOffset(online, 0)
	if err != nil {
		t.Fatalf("OnlineEventParam row 0 not found: %v", err)
	}
	if got := binary.BigEndian.Uint16(online[row+eventChestThresholdOffset:]); got != 0 {
		t.Errorf("stock threshold = %d, want 0 — the reason the chest never armed offline", got)
	}
}

// TestParamRowOffsetRejectsMissingRows — a silent zero here would write the item
// id over a param header and push a corrupt file.
func TestParamRowOffsetRejectsMissingRows(t *testing.T) {
	lot := mustReadOrSkip(t, stockLotParam)
	if _, err := paramRowOffset(lot, 999999999); err == nil {
		t.Error("paramRowOffset accepted a row id that does not exist")
	}
	if err := setParamRowU32(lot, 999999999, eventChestItemOffset, 1); err == nil {
		t.Error("setParamRowU32 accepted a row id that does not exist")
	}
}

func mustReadOrSkip(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		// /data/ is not in the repo; these are extracted from game data.
		t.Skipf("stock param not built: %v", err)
	}
	return b
}
