package game

import (
	"context"
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

// handleMessage dispatches one decoded game message.
//
// It returns the reply payload (nil for messages that take no reply), whether the
// opcode was handled at all, and any error. The handled flag matters for
// diagnosis: several opcodes are deliberately answered with silence because the
// client registers no response callback for them, and without this they would be
// logged identically to opcodes we simply have not implemented — which is exactly
// the signal used to find the next thing to build.
//
// Genuinely unhandled messages are logged with their payload rather than
// answered: the client tolerates silence better than a malformed reply, and the
// capture is what the remaining handlers get built from.
func (s *Service) handleMessage(log logger, cs *clientSession, msgType uint32, payload []byte) ([]byte, bool, error) {
	reply, err := s.dispatch(log, cs, msgType, payload)
	if err != nil {
		return nil, true, err
	}
	if reply == nil && !handledOpcodes[msgType] {
		return nil, false, nil
	}
	return reply, true, nil
}

// handledOpcodes lists opcodes we deliberately answer with no reply, so they are
// not mistaken for unimplemented ones.
var handledOpcodes = map[uint32]bool{
	opRequestUpdatePlayerStatus:      true,
	opRequestUpdatePlayerCharacter:   true,
	opRequestCreateBloodstain:        true,
	opRequestNotifyDeath:             true,
	opRequestNotifyOfflineDeathCount: true,
	opRequestNotifyKillPlayer:        true,
	opRequestNotifyJoinSession:       true,
	opRequestNotifyLeaveSession:      true,
	opRequestNotifyJoinGuestPlayer:   true,
	opRequestNotifyLeaveGuestPlayer:  true,
	opRequestNotifyRingBell:          true,
	opRequestNotifyKillEnemy:         true,
	opRequestNotifyBuyItem:           true,
	opRequestNotifyDisconnectSession: true,
	opRequestNotifyMirrorKnight:      true,
}

func (s *Service) dispatch(log logger, cs *clientSession, msgType uint32, payload []byte) ([]byte, error) {
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

	case opRequestCreateBloodstain:
		return s.handleCreateBloodstain(log, cs, payload)
	case opRequestGetBloodstainList:
		return s.handleGetBloodstainList(log, cs, payload)
	case opRequestGetDeadingGhost:
		return s.handleGetDeadingGhost(log, cs, payload)

	case opRequestCreateGhostData:
		return s.handleCreateGhostData(log, cs, payload)
	case opRequestGetGhostDataList:
		return s.handleGetGhostDataList(log, cs, payload)

	case opRequestCreateSign:
		return s.handleCreateSign(log, cs, payload)
	case opRequestUpdateSign:
		return s.handleUpdateSign(log, cs, payload)
	case opRequestRemoveSign:
		return s.handleRemoveSign(log, cs, payload)
	case opRequestGetSignList:
		return s.handleGetSignList(log, cs, payload)
	case opRequestSummonSign:
		return s.handleSummonSign(log, cs, payload)
	case opRequestRejectSign:
		return s.handleRejectSign(log, cs, payload)

	case opRequestCreateMirrorKnightSign:
		return s.handleCreateMirrorKnightSign(log, cs, payload)
	case opRequestUpdateMirrorKnightSign:
		return s.handleUpdateMirrorKnightSign(log, cs, payload)
	case opRequestRemoveMirrorKnightSign:
		return s.handleRemoveMirrorKnightSign(log, cs, payload)
	case opRequestGetMirrorKnightSignList:
		return s.handleGetMirrorKnightSignList(log, cs, payload)
	case opRequestSummonMirrorKnightSign:
		return s.handleSummonMirrorKnightSign(log, cs, payload)
	case opRequestRejectMirrorKnightSign:
		return s.handleRejectMirrorKnightSign(log, cs, payload)
	case opRequestNotifyMirrorKnight:
		return s.handleNotifyMirrorKnight(log, cs, payload)

	case opRequestGetTotalDeathCount:
		return s.handleGetTotalDeathCount(log, cs, payload)
	case opRequestNotifyDeath:
		return s.handleNotifyDeath(log, cs, payload)
	case opRequestNotifyOfflineDeathCount:
		return s.handleNotifyOfflineDeathCount(log, cs, payload)
	case opRequestNotifyKillPlayer:
		return s.handleNotifyKillPlayer(log, cs, payload)

	case opRequestRegisterPowerStoneData:
		return s.handleRegisterPowerStoneData(log, cs, payload)
	case opRequestGetPowerStoneRanking:
		return s.handleGetPowerStoneRanking(log, cs, payload)
	case opRequestGetPowerStoneMyRanking:
		return s.handleGetPowerStoneMyRanking(log, cs, payload)
	case opRequestGetPowerStoneRankingRecordCoun:
		return s.handleGetPowerStoneRankingRecordCount(log, cs, payload)

	case opRequestSendMessageToPlayers:
		return s.handleSendMessageToPlayers(log, cs, payload)

	case opRequestRegisterQuickMatch:
		return s.handleRegisterQuickMatch(log, cs, payload)
	case opRequestUnregisterQuickMatch:
		return s.handleUnregisterQuickMatch(log, cs, payload)
	case opRequestUpdateQuickMatch:
		return s.handleUpdateQuickMatch(log, cs, payload)
	case opRequestSearchQuickMatch:
		return s.handleSearchQuickMatch(log, cs, payload)
	case opRequestJoinQuickMatch:
		return s.handleJoinQuickMatch(log, cs, payload)
	case opRequestRejectQuickMatch:
		return s.handleRejectQuickMatch(log, cs, payload)

	case opRequestGetVisitorList:
		return s.handleGetVisitorList(log, cs, payload)
	case opRequestVisit:
		return s.handleVisit(log, cs, payload)
	case opRequestRejectVisit:
		return s.handleRejectVisit(log, cs, payload)

	case opRequestGetBreakInTargetList:
		return s.handleGetBreakInTargetList(log, cs, payload)
	case opRequestBreakInTarget:
		return s.handleBreakInTarget(log, cs, payload)
	case opRequestRejectBreakInTarget:
		return s.handleRejectBreakInTarget(log, cs, payload)

	case opRequestGetLoginPlayerCharacter:
		return s.handleGetLoginPlayerCharacter(log, cs, payload)
	case opRequestGetPlayerCharacter:
		return s.handleGetPlayerCharacter(log, cs, payload)
	case opRequestGetPlayerCharacterList:
		return s.handleGetPlayerCharacterList(log, cs, payload)

	case opServerPing:
		return s.handleServerPing(log, cs, payload)
	case opRequestMeasureUploadBandwidth:
		return s.handleMeasureUploadBandwidth(log, cs, payload)
	case opRequestMeasureDownloadBandwith:
		return s.handleMeasureDownloadBandwidth(log, cs, payload)
	case opRequestBenchmarkThroughput:
		return s.handleBenchmarkThroughput(log, cs, payload)

	case opRequestNotifyJoinSession:
		return s.handleNotifyJoinSession(log, cs, payload)
	case opRequestNotifyLeaveSession:
		return s.handleNotifyLeaveSession(log, cs, payload)
	case opRequestNotifyJoinGuestPlayer:
		return s.handleNotifyJoinGuestPlayer(log, cs, payload)
	case opRequestNotifyLeaveGuestPlayer:
		return s.handleNotifyLeaveGuestPlayer(log, cs, payload)
	case opRequestNotifyRingBell:
		return s.handleNotifyRingBell(log, cs, payload)
	case opRequestNotifyKillEnemy:
		return s.handleNotifyKillEnemy(log, cs, payload)
	case opRequestNotifyBuyItem:
		return s.handleNotifyBuyItem(log, cs, payload)
	case opRequestNotifyDisconnectSession:
		return s.handleNotifyDisconnectSession(log, cs, payload)

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

	// FromSoftware named this field steam_id in the PC protos; we have renamed it
	// psn_id since this server targets PS3, where it carries the PSN online ID.
	// The wire format is unchanged - only the name.
	accountID := req.GetPsnId()
	if accountID == "" {
		return nil, fmt.Errorf("RequestWaitForUserLogin has an empty account id")
	}

	cs.accountID = accountID
	// Persisted and stable: the same PSN account always gets the same player id.
	// Player ids appear in blood messages, signs and the leaderboard, and other
	// clients cache them — a per-run id would make all of that point at the wrong
	// person after a restart.
	playerID, err := s.store.PlayerID(context.Background(), accountID)
	if err != nil {
		return nil, err
	}
	cs.playerID = playerID

	log.Info("player logged in to game service",
		"account_id", accountID, "player_id", cs.playerID,
		"unknown_1", req.GetUnknown_1(), "unknown_2", req.GetUnknown_2(),
		"unknown_3", req.GetUnknown_3(), "unknown_4", req.GetUnknown_4())

	// Push the configured server text once the client has an identity. This is
	// queued behind the login reply rather than sent instead of it — the client
	// will not proceed while a request/response is outstanding.
	s.sendManagementText(log, cs)

	resp := &ds2pb.RequestWaitForUserLoginResponse{
		PsnId:    proto.String(accountID),
		PlayerId: proto.Uint32(cs.playerID),
	}
	return proto.Marshal(resp)
}
