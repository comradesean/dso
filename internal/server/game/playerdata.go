package game

import (
	"context"
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
// ids it already holds locally. We pick the lowest positive id that neither the
// client nor the store already has. A non-zero id means the client is
// re-asserting a slot it already has, so it is echoed back unchanged.
//
// Consulting the store matters: the client only volunteers the slots it knows
// about, so allocating from that alone would hand out an id already recorded for
// this player on another console and silently merge two characters.
func (s *Service) handleUpdateLoginPlayerCharacter(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdateLoginPlayerCharacter
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdateLoginPlayerCharacter: %w", err)
	}

	characterID := req.GetCharacterId()
	if characterID == 0 {
		known, err := s.store.CharacterIDs(context.Background(), cs.playerID)
		if err != nil {
			return nil, err
		}
		characterID = lowestFreeCharacterID(append(known, req.GetLocalCharacterIds()...))
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
// DS2_Frpg2PlayerData.AllStatus protobuf — ordinary protobuf, never an opaque
// payload; it was simply unparsed until matchmaking needed it. It is still
// stored verbatim as well, because we decode only a fraction of it.
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

	// Refresh the matching profile. Log at Info only when the visitor pool
	// changes: the blob arrives constantly, but entering or leaving a pool is
	// the event that decides whether auto-summons can find this player at all,
	// and it is the first thing to check when one "does nothing".
	prevPool := visitorPoolFor(cs.profile)
	cs.profile = profileFromStatus(req.GetStatus())
	if pool := visitorPoolFor(cs.profile); pool != prevPool {
		log.Info("visitor pool changed",
			"player_id", cs.playerID, "from", prevPool, "to", pool,
			"covenant", cs.profile.covenant,
			"activity_area", cs.profile.onlineActivityArea,
			"soul_memory", cs.profile.soulMemory,
			"tier", soulMemoryTier(cs.profile.soulMemory),
			"at_bonfire", cs.profile.sittingAtBonfire)
	}

	if cs.characterID != 0 {
		if err := s.store.SaveCharacterStatus(context.Background(),
			cs.playerID, cs.characterID, req.GetStatus()); err != nil {
			return nil, err
		}
	}
	log.Debug("player status updated",
		"player_id", cs.playerID, "character_id", cs.characterID,
		"status_bytes", len(cs.status), "parsed", cs.profile.received)
	return nil, nil
}

// handleUpdatePlayerCharacter persists the opaque character save blob.
//
// Stored verbatim and never interpreted, as the reference does. Returns no reply
// for the same reason as handleUpdatePlayerStatus.
func (s *Service) handleUpdatePlayerCharacter(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestUpdatePlayerCharacter
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestUpdatePlayerCharacter: %w", err)
	}
	if err := s.store.SaveCharacterData(context.Background(),
		cs.playerID, req.GetCharacterId(), req.GetCharacterData()); err != nil {
		return nil, err
	}
	log.Debug("player character updated",
		"player_id", cs.playerID, "character_id", req.GetCharacterId(),
		"data_bytes", len(req.GetCharacterData()))
	return nil, nil
}

// opRequestGetLoginPlayerCharacter is request/response and was going unanswered.
const opRequestGetLoginPlayerCharacter uint32 = 0x03B3

// handleGetLoginPlayerCharacter returns a player's current character blob.
func (s *Service) handleGetLoginPlayerCharacter(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetLoginPlayerCharacter
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetLoginPlayerCharacter: %w", err)
	}
	targetID := uint32(req.GetPlayerId())
	characterID := cs.characterID
	data := []byte{}

	// The blob belongs to whichever player was asked about, which is not always
	// the caller — the client asks about others too.
	if c, ok, err := s.store.GetCharacter(context.Background(), targetID, characterID); err != nil {
		return nil, err
	} else if ok {
		data = c.Data
	}

	log.Debug("login player character requested",
		"player_id", cs.playerID, "for_player_id", targetID,
		"character_id", characterID, "data_bytes", len(data))
	return proto.Marshal(&ds2pb.RequestGetLoginPlayerCharacterResponse{
		PlayerId:      proto.Int64(req.GetPlayerId()),
		CharacterId:   proto.Uint32(characterID),
		CharacterData: data,
	})
}

// Character reads. Both are request/response and were going unanswered.
const (
	opRequestGetPlayerCharacter     uint32 = 0x03A9
	opRequestGetPlayerCharacterList uint32 = 0x03B5
)

// handleGetPlayerCharacter returns one character's saved blob.
//
// All three response fields are `required`, so a character we have never seen
// still gets a complete reply with an empty blob rather than silence — an
// unanswered request/response stalls whatever UI is waiting on it, and the client
// will not open other online UI while one is outstanding.
func (s *Service) handleGetPlayerCharacter(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetPlayerCharacter
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetPlayerCharacter: %w", err)
	}

	data := []byte{}
	c, ok, err := s.store.GetCharacter(context.Background(), req.GetPlayerId(), req.GetCharacterId())
	if err != nil {
		return nil, err
	}
	if ok {
		data = c.Data
	}

	log.Debug("player character requested",
		"by_player_id", cs.playerID, "for_player_id", req.GetPlayerId(),
		"character_id", req.GetCharacterId(), "found", ok, "data_bytes", len(data))

	return proto.Marshal(&ds2pb.RequestGetPlayerCharacterResponse{
		PlayerId:      proto.Uint32(req.GetPlayerId()),
		CharacterId:   proto.Uint32(req.GetCharacterId()),
		CharacterData: data,
	})
}

// handleGetPlayerCharacterList answers the character-list request.
//
// Its response message is defined with NO fields at all, so an empty reply is the
// complete answer. The request's own shape (player_id, character_id, and a
// character_data blob) reads more like an update than a query, and the field is
// even misspelled `charatcer_id` in the schema — but with nothing to return, none
// of that changes what we send. Logged in full so a capture can settle what the
// client actually means by it.
func (s *Service) handleGetPlayerCharacterList(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetPlayerCharacterList
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetPlayerCharacterList: %w", err)
	}
	log.Info("player character list requested",
		"by_player_id", cs.playerID, "for_player_id", req.GetPlayerId(),
		"character_id", req.GetCharatcerId(), "data_bytes", len(req.GetCharacterData()))
	return proto.Marshal(&ds2pb.RequestGetPlayerCharacterListResponse{})
}
