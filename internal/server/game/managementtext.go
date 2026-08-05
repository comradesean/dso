package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2datapb"
	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// opPushManagementText is the PushMessageId for ManagementTextMessage.
//
// CONFIRMED ON PS3, and unusually firmly: the push dispatcher at EBOOT 0x158C138
// special-cases exactly three ids *by value* before falling through to its
// red-black-tree lookup — 0x0389 -> 0x1587F60, 0x038B -> 0x158B150, 0x038C ->
// 0x1588218. Handler 0x1587F60 builds an object whose GetTypeName returns
// "Frpg2RequestMessage.ManagementTextMessage". So unlike the BreakIn push ids,
// which sit in alias blocks we had to guess at, there is no ambiguity here.
const opPushManagementText = 0x0389

// managementTextTimestamp is the timestamp sent with every management text.
//
// The client parses the field and requires it to be present and complete, but
// the handler at 0x1587F60 only forwards it on; nothing observed compares it
// against anything. A fixed value therefore costs nothing and keeps the push
// byte-identical between sessions, which makes it far easier to spot in a log.
var managementTextTimestamp = &ds2datapb.DateTime{
	Year:    proto.Uint32(2021),
	Month:   proto.Uint32(1),
	Day:     proto.Uint32(1),
	Hours:   proto.Uint32(0),
	Minutes: proto.Uint32(0),
	Seconds: proto.Uint32(0),
	Tzdiff:  proto.Uint32(0),
}

// sendManagementText pushes the configured server text to one client.
//
// CONFIRMED LIVE (2026-08-05): this renders as the banner in the UPPER LEFT of
// the screen, immediately after login. It is NOT the Majula obelisk. The binary
// could not settle this — the listener is stored at manager+72 but the code
// installing it was never found — and only a real console answered it.
//
// The obelisk turned out to be a different mechanism entirely: its text is string
// id 100 in regulation<Language>.fmg inside the regulation archive, changed by
// shipping a new regulation rather than by any push. See docs/STATUS.md.
//
// This is still the only free-text server->client channel in the DS2 protocol:
// across the whole schema the sole free-text strings are AnnounceMessageData's
// header/message and this one. Everything else is a PSN id, and blood messages
// are word-ID templates.
//
// All five fields are genuinely required: IsInitialized at 0x162A6F8 masks the
// has-bits with 0x1F and demands all five, and recursively validates the
// timestamp. An omitted field means the client drops the push in silence.
func (s *Service) sendManagementText(log logger, cs *clientSession) {
	text := s.srv.Config.ManagementText
	if text == "" {
		return
	}

	body, err := proto.Marshal(&ds2pb.ManagementTextMessage{
		PushMessageId: ds2pb.PushMessageId(opPushManagementText).Enum(),
		Message:       proto.String(text),
		Timestamp:     managementTextTimestamp,
		// Language: 0 is JP, 1 is EN.
		Unknown_4: proto.Uint32(uint32(s.srv.Config.ManagementTextLanguage)),
		Unknown_5: proto.Uint32(0),
	})
	if err != nil {
		log.Warn("management text: marshal failed", "err", err)
		return
	}

	cs.conn.SendPush(body)
	log.Info("management text pushed",
		"player_id", cs.playerID,
		"push_id", fmt.Sprintf("%#04x", opPushManagementText),
		"language", s.srv.Config.ManagementTextLanguage,
		"payload_bytes", len(body))
}
