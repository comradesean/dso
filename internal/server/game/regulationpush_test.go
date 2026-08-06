package game

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// TestRegulationPushWireFormat pins the actual bytes on the wire.
//
// Round-tripping through our own generated Go code proves nothing about field
// numbers: if we assign diff_data_list the wrong number, marshal and unmarshal
// agree with each other perfectly and the client discards the push in silence.
// That is exactly how the ghost list was broken for the whole project's life —
// field 3 from the PC schema where PS3 wants field 2 — so the field numbers the
// decompilation recovered get checked against raw bytes here.
//
// Expected framing, from tasks/regulation-push-038b.md:
//
//	push:  field 1 varint  = push id 0x038B (907)
//	       field 2 bytes   = RegulationFileUpdateMessage
//	inner: field 1 bytes   = RegulationFileDiffData (repeated)
//	diff:  field 1 varint  = version_new
//	       field 2 varint  = version_required
//	       field 3 bytes   = path
//	       field 4 bytes   = diff_data
func TestRegulationPushWireFormat(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	body, err := proto.Marshal(&ds2pb.RegulationFileUpdatePushMessage{
		PushMessageId: ds2pb.PushMessageId(opPushRegulationFileUpdate).Enum(),
		UpdateMsg: &ds2pb.RegulationFileUpdateMessage{
			DiffDataList: []*ds2pb.RegulationFileDiffData{{
				VersionNew:      proto.Uint32(1),
				VersionRequired: proto.Uint32(0),
				Path:            proto.String("OnlineEventParam.param"),
				DiffData:        payload,
			}},
		},
	})
	if err != nil {
		t.Fatalf("the push we send does not marshal: %v", err)
	}

	// Field 1, varint: 0x08 then 907 as a varint = 0x8B 0x07.
	if want := []byte{0x08, 0x8B, 0x07}; !bytes.HasPrefix(body, want) {
		t.Errorf("push id framing = %s..., want prefix %s",
			hex.EncodeToString(body[:min(4, len(body))]), hex.EncodeToString(want))
	}

	// Field 2, length-delimited: tag 0x12.
	rest := body[3:]
	if len(rest) == 0 || rest[0] != 0x12 {
		t.Fatalf("update_msg tag = %#02x, want 0x12 (field 2, length-delimited)", rest[0])
	}

	// Inside RegulationFileUpdateMessage, diff_data_list is field 1,
	// length-delimited: tag 0x0A.
	inner := rest[2:]
	if len(inner) == 0 || inner[0] != 0x0A {
		t.Fatalf("diff_data_list tag = %#02x, want 0x0a (field 1, length-delimited)", inner[0])
	}

	// The path must appear as a bare string with a field-3 tag (0x1A) in front,
	// and the payload bytes with a field-4 tag (0x22). Searching for the tag plus
	// length plus content catches a field renumbering that a parse would not.
	pathBytes := []byte("OnlineEventParam.param")
	wantPath := append([]byte{0x1A, byte(len(pathBytes))}, pathBytes...)
	if !bytes.Contains(body, wantPath) {
		t.Errorf("path not framed as field 3: looked for %s in %s",
			hex.EncodeToString(wantPath), hex.EncodeToString(body))
	}
	wantData := append([]byte{0x22, byte(len(payload))}, payload...)
	if !bytes.Contains(body, wantData) {
		t.Errorf("diff_data not framed as field 4: looked for %s in %s",
			hex.EncodeToString(wantData), hex.EncodeToString(body))
	}

	var got ds2pb.RegulationFileUpdatePushMessage
	if err := proto.Unmarshal(body, &got); err != nil {
		t.Fatalf("client-side parse would reject our push: %v", err)
	}
	if int(got.GetPushMessageId()) != opPushRegulationFileUpdate {
		t.Errorf("push id = %#04x, want %#04x", int(got.GetPushMessageId()), opPushRegulationFileUpdate)
	}
	if n := len(got.GetUpdateMsg().GetDiffDataList()); n != 1 {
		t.Fatalf("diff_data_list length = %d, want 1", n)
	}
	if d := got.GetUpdateMsg().GetDiffDataList()[0]; !bytes.Equal(d.GetDiffData(), payload) {
		t.Errorf("diff_data = %x, want %x", d.GetDiffData(), payload)
	}
}

// TestRegulationPushPayloadSizeUnchanged guards the one constraint that fails
// silently on the console: the param route compares the pushed size against the
// loaded resource's size and skips the entry unless they are equal (0x770DE4).
//
// The armed payload must therefore be byte-for-byte the same length as the file
// it was derived from, and differ only in the claim threshold.
func TestRegulationPushPayloadSizeUnchanged(t *testing.T) {
	stock := mustReadFile(t, "../../../data/regpush/OnlineEventParam.param")
	armed := mustReadFile(t, "../../../data/regpush/OnlineEventParam.armed.param")

	if len(stock) != len(armed) {
		t.Fatalf("armed payload is %d bytes, stock is %d — the client would discard it",
			len(armed), len(stock))
	}

	// Row 0 sits at file offset 0x4C; the threshold is the u16 at +2.
	const thresholdOff = 0x4C + 2
	if got := uint16(stock[thresholdOff])<<8 | uint16(stock[thresholdOff+1]); got != 0 {
		t.Errorf("stock threshold = %d, want 0 (the whole reason the chest never arms)", got)
	}
	if got := uint16(armed[thresholdOff])<<8 | uint16(armed[thresholdOff+1]); got != 1 {
		t.Errorf("armed threshold = %d, want 1", got)
	}

	// Nothing else may differ, or we are testing more than one thing at once.
	for i := range stock {
		if i == thresholdOff || i == thresholdOff+1 {
			continue
		}
		if stock[i] != armed[i] {
			t.Fatalf("payloads differ at %#x (%#02x vs %#02x); only the threshold should change",
				i, stock[i], armed[i])
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
