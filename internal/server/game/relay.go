package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// opRequestSendMessageToPlayers is the client-to-client relay tunnel.
//
// Direction matters for this opcode: client->server 0x0320 is this request,
// while server->client 0x0320 is how every push is framed.
const opRequestSendMessageToPlayers uint32 = 0x0320

// maxRelayRecipients caps how many players one relay may target.
//
// This is CVE-2022-24125 mitigation, carried over from the reference: without a
// cap a client can address the entire server and inject arbitrary push messages
// into every session. Six is what the reference settled on — enough for a full
// co-op party, far short of a broadcast.
const maxRelayRecipients = 6

// handleSendMessageToPlayers forwards an opaque, client-authored push to named
// recipients.
//
// Exactly two pushes are never built by the server: the client serializes them
// itself and tunnels them through here. Scanning the v1.10 EBOOT's whole .text for
// this opcode as the send helper's argument finds two sites and no more —
// 0x16040FC building PushRequestAllowBreakInTarget, and 0x15DF124 building
// PushRequestAllowQuickMatch. Everything else the client expects from us.
// PushRequestAllowBreakInTarget is the one that matters most — an invasion
// completes only when the host's "allow" reaches the invader through this path,
// which is why invasions timed out while this went unhandled even though the
// break-in push itself was landing.
//
// Both bodies exist to carry one opaque blob (field_5 / player_struct): a
// NexusRevolution2 P2P matching packet, magic "NXRV", version 100, holding the
// sender's PSN online id and a session id. It is the peer's connection handshake,
// nothing the server can synthesise. docs/protocol-map-ps3.md §5.2.7-§5.2.8.
//
// The body is forwarded verbatim. The server does not parse or validate it, which
// is exactly why the recipient cap above exists.
func (s *Service) handleSendMessageToPlayers(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestSendMessageToPlayers
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestSendMessageToPlayers: %w", err)
	}

	ids := req.GetPlayerIds()
	if len(ids) > maxRelayRecipients {
		log.Warn("relay recipient list truncated",
			"player_id", cs.playerID, "requested", len(ids), "cap", maxRelayRecipients)
		ids = ids[:maxRelayRecipients]
	}

	body := req.GetMessage()
	delivered, offline := 0, 0
	for _, id := range ids {
		if id == cs.playerID {
			// Relaying to yourself is meaningless and is the shape a loop would
			// take, so drop it.
			continue
		}
		target, live := s.sessionForPlayerLocked(id)
		if !live {
			offline++
			continue
		}
		target.conn.SendPush(body)
		delivered++
	}

	log.Info("relayed client message",
		"from_player_id", cs.playerID, "recipients", len(ids),
		"delivered", delivered, "offline", offline, "payload_bytes", len(body))

	return proto.Marshal(&ds2pb.RequestSendMessageToPlayersResponse{})
}
