package game

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// CONFIRMED ON PS3 by live capture from a real BLUS41045 client, 2026-08-05.
const opRequestUpdateLoginPlayerCharacter uint32 = 0x03B6

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
