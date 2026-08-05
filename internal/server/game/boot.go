package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// Game-service message opcodes.
//
// CONFIRMED ON PS3: RequestWaitForUserLogin (0x0386) was observed from a real
// BLUS41045 client on 2026-08-05 as the first message after the reliable-UDP
// session established, carrying exactly the expected protobuf. That is the first
// evidence that PS3 shares the PC/SOTFS opcode numbering; the rest of the table in
// docs/protocol-map.md is still reference-derived and unverified.
const (
	opRequestWaitForUserLogin       uint32 = 0x0386
	opRequestGetAnnounceMessageList uint32 = 0x03EC
)

// nextPlayerID hands out player ids. Ids are per-run and not persisted; a store
// lands with the rest of the player-data work.
func (s *Service) nextPlayerID() uint32 {
	s.playerSeq++
	return s.playerSeq
}

// handleMessage dispatches one decoded game message. It returns the reply payload
// to send, or nil to send nothing.
//
// Unhandled messages are logged with their payload rather than answered — the
// client tolerates silence better than a malformed reply, and the capture is what
// the remaining handlers get built from.
func (s *Service) handleMessage(log logger, cs *clientSession, msgType uint32, payload []byte) ([]byte, error) {
	switch msgType {
	case opRequestWaitForUserLogin:
		return s.handleWaitForUserLogin(log, cs, payload)
	case opRequestGetAnnounceMessageList:
		return s.handleGetAnnounceMessageList(log, cs, payload)
	case opRequestUpdateLoginPlayerCharacter:
		return s.handleUpdateLoginPlayerCharacter(log, cs, payload)

	case opRequestCreateBloodMessage:
		return s.handleCreateBloodMessage(log, cs, payload)
	case opRequestGetBloodMessageList:
		return s.handleGetBloodMessageList(log, cs, payload)
	case opRequestReentryBloodMessage:
		return s.handleReentryBloodMessage(log, cs, payload)
	case opRequestRemoveBloodMessage:
		return s.handleRemoveBloodMessage(log, cs, payload)
	case opRequestEvaluateBloodMessage:
		return s.handleEvaluateBloodMessage(log, cs, payload)
	case opRequestGetBloodMessageEvaluation:
		return s.handleGetBloodMessageEvaluation(log, cs, payload)

	case opRequestCreateGhostData:
		return s.handleCreateGhostData(log, cs, payload)
	case opRequestGetGhostDataList:
		return s.handleGetGhostDataList(log, cs, payload)

	case opRequestUpdatePlayerStatus:
		return s.handleUpdatePlayerStatus(log, cs, payload)
	case opRequestUpdatePlayerCharacter:
		return s.handleUpdatePlayerCharacter(log, cs, payload)
	default:
		return nil, nil
	}
}

// handleGetAnnounceMessageList answers the client's request for server
// announcements — the "Retrieving information" step, which blocks boot until it
// gets a reply.
//
// Both lists are returned present but empty. `changes` and `notices` are both
// `required`, so omitting either produces a message the client's proto2 parser
// rejects outright; an empty list is not the same as an absent one here.
//
// This is also the channel the reference uses to deliver bans and warnings, by
// returning an announcement and then disconnecting. Nothing needs that yet.
func (s *Service) handleGetAnnounceMessageList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetAnnounceMessageList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetAnnounceMessageList: %w", err)
	}
	log.Info("announce message list requested",
		"player_id", cs.playerID, "max_entries", req.GetMaxEntries())

	resp := &ds2pb.RequestGetAnnounceMessageListResponse{
		Changes: &ds2pb.AnnounceMessageDataList{},
		Notices: &ds2pb.AnnounceMessageDataList{},
	}
	return proto.Marshal(resp)
}

// handleWaitForUserLogin answers the first message of the game session and
// assigns the player id everything downstream is keyed by.
func (s *Service) handleWaitForUserLogin(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestWaitForUserLogin
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestWaitForUserLogin: %w", err)
	}

	// The field is named steam_id in the FromSoftware protos, but on PS3 it
	// carries the PSN online ID. It is the platform account id either way.
	accountID := req.GetSteamId()
	if accountID == "" {
		return nil, fmt.Errorf("RequestWaitForUserLogin has an empty account id")
	}

	cs.accountID = accountID
	cs.playerID = s.nextPlayerID()

	log.Info("player logged in to game service",
		"account_id", accountID, "player_id", cs.playerID,
		"unknown_1", req.GetUnknown_1(), "unknown_2", req.GetUnknown_2(),
		"unknown_3", req.GetUnknown_3(), "unknown_4", req.GetUnknown_4())

	resp := &ds2pb.RequestWaitForUserLoginResponse{
		SteamId:  proto.String(accountID),
		PlayerId: proto.Uint32(cs.playerID),
	}
	return proto.Marshal(resp)
}
