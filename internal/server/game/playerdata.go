package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// CONFIRMED ON PS3 by live capture from a real BLUS41045 client, 2026-08-05.
const (
	opRequestUpdateLoginPlayerCharacter uint32 = 0x03B6
	opRequestUpdatePlayerStatus         uint32 = 0x03B8
	opRequestUpdatePlayerCharacter      uint32 = 0x03A8
)

// handleUpdateLoginPlayerCharacter assigns the character slot the client will
// play as. This is the "Initializing online mode..." step.
//
// The client sends character_id = 0 to mean "allocate one for me", along with the
// ids it already holds locally. We pick the lowest positive id it is not already
// using. A non-zero id means the client is re-asserting a slot it already has, so
// it is echoed back unchanged.
//
// Ids are per-session and not persisted; once there is a store, allocation must
// also avoid ids already recorded for this account rather than only the ones the
// client volunteers.
func (s *Service) handleUpdateLoginPlayerCharacter(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdateLoginPlayerCharacter
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdateLoginPlayerCharacter: %w", err)
	}

	characterID := req.GetCharacterId()
	if characterID == 0 {
		characterID = lowestFreeCharacterID(req.GetLocalCharacterIds())
		log.Info("allocated character id",
			"player_id", cs.playerID, "character_id", characterID,
			"local_character_ids", req.GetLocalCharacterIds())
	} else {
		log.Info("client asserted character id",
			"player_id", cs.playerID, "character_id", characterID)
	}
	cs.characterID = characterID

	resp := &ds2pb.RequestUpdateLoginPlayerCharacterResponse{
		CharacterId: proto.Uint32(characterID),
	}
	return proto.Marshal(resp)
}

// lowestFreeCharacterID returns the smallest id >= 1 that is not in taken.
func lowestFreeCharacterID(taken []uint32) uint32 {
	inUse := make(map[uint32]bool, len(taken))
	for _, id := range taken {
		inUse[id] = true
	}
	for id := uint32(1); ; id++ {
		if !inUse[id] {
			return id
		}
	}
}

// handleUpdatePlayerStatus records the periodic status blob the client uploads.
//
// This is the heartbeat that drives every matchmaking filter: soul memory,
// current area and covenant all come from here. The blob is a
// DS2_Frpg2PlayerData.AllStatus protobuf; we keep it opaque for now and only
// record that it arrived, since nothing consumes it until matchmaking exists.
//
// Returns no reply. Decompilation shows this opcode registers no response
// callback (docs/protocol-map-ps3.md), and a live client reached in-game with it
// unanswered — so the client genuinely does not wait on one. The PC reference
// sends an empty response, which is one of the places it differs from PS3.
func (s *Service) handleUpdatePlayerStatus(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdatePlayerStatus
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdatePlayerStatus: %w", err)
	}
	cs.status = append(cs.status[:0], req.GetStatus()...)
	log.Debug("player status updated",
		"player_id", cs.playerID, "status_bytes", len(cs.status))
	return nil, nil
}

// handleUpdatePlayerCharacter records the opaque character save blob.
//
// Stored verbatim and never interpreted, as the reference does. Returns no reply
// for the same reason as handleUpdatePlayerStatus.
func (s *Service) handleUpdatePlayerCharacter(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdatePlayerCharacter
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdatePlayerCharacter: %w", err)
	}
	log.Debug("player character updated",
		"player_id", cs.playerID, "character_id", req.GetCharacterId(),
		"data_bytes", len(req.GetCharacterData()))
	return nil, nil
}

// opRequestGetLoginPlayerCharacter is request/response and was going unanswered.
const opRequestGetLoginPlayerCharacter uint32 = 0x03B3

// handleGetLoginPlayerCharacter returns a player's current character blob.
//
// Character data is not persisted yet, so this replies with the requested ids and
// an empty blob rather than staying silent: an unanswered request/response stalls
// whatever UI is waiting on it, which is worse than an empty answer.
func (s *Service) handleGetLoginPlayerCharacter(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetLoginPlayerCharacter
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetLoginPlayerCharacter: %w", err)
	}
	log.Debug("login player character requested",
		"player_id", cs.playerID, "for_player_id", req.GetPlayerId())
	return proto.Marshal(&ds2pb.RequestGetLoginPlayerCharacterResponse{
		PlayerId:      proto.Int64(req.GetPlayerId()),
		CharacterId:   proto.Uint32(cs.characterID),
		CharacterData: []byte{},
	})
}
