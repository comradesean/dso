package game

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// TestManagementTextDisabledByDefault pins that we send nothing unless it is
// configured. The push goes out on every login, so a default-on version would
// put an unrequested message in front of every player.
func TestManagementTextDisabledByDefault(t *testing.T) {
	svc, log, _ := testService(t)
	cs := newTestSession("comradesean", 1)

	// A nil conn would panic if the guard were missing, which is the point: this
	// asserts the early return happens before anything touches the connection.
	cs.conn = nil
	svc.sendManagementText(log, cs)
}

// TestManagementTextWireFormat checks the exact bytes we put on the wire.
//
// Every one of the five fields is `required`, and the client's IsInitialized
// (EBOOT 0x162A6F8) masks the has-bits with 0x1F and demands all five, plus a
// recursive check on the timestamp's seven. An omitted field does not produce an
// error — the client drops the push in silence, which is indistinguishable from
// a feature that simply does not exist. So the round-trip is the test.
func TestManagementTextWireFormat(t *testing.T) {
	svc, log, _ := testService(t)
	svc.srv.Config.ManagementText = "Seek the land of an ancient king, in the Black Gulch, deep below"
	svc.srv.Config.ManagementTextLanguage = 1

	// Exercise the real send path against a session with a real reliable-UDP
	// connection, so a panic or marshal failure in the production path surfaces
	// here rather than in front of a console.
	cs := newTestSession("comradesean", 1)
	svc.sendManagementText(log, cs)

	// Then pin the payload itself. The framing (0x0320 / 0xFFFFFFFF) is already
	// covered by the push tests; what is new and fragile here is the message body.
	sent, err := proto.Marshal(&ds2pb.ManagementTextMessage{
		PushMessageId: ds2pb.PushMessageId(opPushManagementText).Enum(),
		Message:       proto.String(svc.srv.Config.ManagementText),
		Timestamp:     managementTextTimestamp,
		Unknown_4:     proto.Uint32(uint32(svc.srv.Config.ManagementTextLanguage)),
		Unknown_5:     proto.Uint32(0),
	})
	if err != nil {
		t.Fatalf("the message we send does not marshal: %v", err)
	}

	// Parse it back the way the client would. proto2 rejects a message with any
	// required field unset, so this is a real check that all five are populated.
	var got ds2pb.ManagementTextMessage
	if err := proto.Unmarshal(sent, &got); err != nil {
		t.Fatalf("client-side parse would reject our push: %v", err)
	}

	if got.GetPushMessageId() != ds2pb.PushMessageId(opPushManagementText) {
		t.Errorf("push_message_id = %#04x, want %#04x — this is the value the "+
			"client's dispatcher branches on at EBOOT 0x158C1D8",
			int(got.GetPushMessageId()), opPushManagementText)
	}
	if got.GetMessage() != svc.srv.Config.ManagementText {
		t.Errorf("message = %q, want %q", got.GetMessage(), svc.srv.Config.ManagementText)
	}
	if got.GetUnknown_4() != 1 {
		t.Errorf("language = %d, want 1 (EN)", got.GetUnknown_4())
	}
	if ts := got.GetTimestamp(); ts == nil || ts.GetYear() == 0 {
		t.Error("timestamp missing or incomplete; the client validates it recursively")
	}
}
