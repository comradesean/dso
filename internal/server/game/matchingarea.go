package game

import (
	"fmt"
	"sort"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// opRequestGetRightMatchingArea asks which areas are worth travelling to.
//
// ADDED IN v1.10 — it does not exist on the launch disc.
// docs/protocol-map-ps3.md lists it among six opcodes that "do not exist in this
// binary", and that is correct *for the build it describes*: the `li r4,0x03fa`
// encoding (38 80 03 fa) occurs zero times in the v1.00 EBOOT and twice in v1.10.
//
//	CONFIRMED LIVE 2026-08-05 — two real BLUS41045 v1.10 clients on separate
//	machines each sent 0x03FA at boot with a 29-byte payload decoding cleanly as
//	RequestGetRightMatchingArea{matching_parameter}.
//
// See versions.go. The lesson generalises: "absent" from the decompilation means
// absent from v1.00, not from every build we support.
//
// What it is FOR: the response is a list of (online_area_id, population) pairs.
// Patch 1.10 added population hints to the bonfire warp screen — highlighting the
// areas with the best chance of finding other players — and this is the opcode
// that feeds them. It had been going unanswered, which for a request/response
// opcode is never harmless: the client retries silently and will not open other
// online UI while one is outstanding.
const opRequestGetRightMatchingArea uint32 = 0x03FA

// matchingAreaReportLimit is how many areas the response carries.
//
// Three, because that is what FromSoftware's own server sent in every one of the
// 97 responses in the capture corpus — never two, never four.
const matchingAreaReportLimit = 3

// handleGetRightMatchingArea reports where other players actually are.
//
// Population is counted from live sessions by the area in each player's last
// status update. That is genuinely the number the client is asking for, and it
// costs nothing — the sessions are already in memory.
//
// The requester's own area is included: they are a real player in it, and the
// hint is about where the population is, not about where to go next.
func (s *Service) handleGetRightMatchingArea(log logger, cs *clientSession, payload []byte) ([]byte, error) {
	var req ds2pb.RequestGetRightMatchingArea
	if err := proto.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("parse RequestGetRightMatchingArea: %w", err)
	}

	// CONFIRMED LIVE 2026-08-06: this drives the orange glow on the bonfire
	// travel screen, marking the area with the most activity. Worth recording
	// because 0x03FA exists only in v1.10 — it is absent from the v1.00 binary,
	// which briefly had us believing the opcode was not real.
	//
	// The requester is counted along with everyone else, so a player's own area
	// always contributes at least one to its own glow. Whether the real server
	// excluded the asker is unknown and it is plausible either way; a "where is
	// everyone" display including you is not obviously wrong. It would only
	// become visible with players spread across several areas.
	counts := make(map[uint32]uint32)
	for _, other := range s.sessions {
		if other.playerID == 0 || other.areaID == 0 {
			continue
		}
		counts[other.areaID]++
	}

	// FROM SOFTWARE SENT EXACTLY THREE, SORTED BY POPULATION DESCENDING.
	// Measured from their live server: 97 of 97 captured responses carry three
	// entries, and 97 of 97 are in descending order (tasks/live-capture-corpus.md).
	//
	// Both halves matter and we had neither. Ranging over a Go map yields a
	// DELIBERATELY RANDOMISED order, so the client was handed a different
	// ordering on every request — and if it renders these positionally, the
	// highlighted area would move at random. Sending every area rather than three
	// is also a guess about a client we cannot see; matching the observed shape
	// costs nothing and removes the guess.
	//
	// Ties break on the lower area id so the result is fully deterministic.
	areas := make([]*ds2pb.RequestGetRightMatchingAreaResponse_AreaInfo, 0, len(counts))
	for areaID, n := range counts {
		areas = append(areas, &ds2pb.RequestGetRightMatchingAreaResponse_AreaInfo{
			OnlineAreaId: proto.Uint32(areaID),
			Population:   proto.Uint32(n),
		})
	}
	sort.Slice(areas, func(i, j int) bool {
		if areas[i].GetPopulation() != areas[j].GetPopulation() {
			return areas[i].GetPopulation() > areas[j].GetPopulation()
		}
		return areas[i].GetOnlineAreaId() < areas[j].GetOnlineAreaId()
	})
	if len(areas) > matchingAreaReportLimit {
		areas = areas[:matchingAreaReportLimit]
	}

	log.Info("matching area populations requested",
		"player_id", cs.playerID, "areas_reported", len(areas),
		"calibration", req.GetMatchingParameter().GetCalibrationVersion(),
		"soul_level", req.GetMatchingParameter().GetSoulLevel())

	return proto.Marshal(&ds2pb.RequestGetRightMatchingAreaResponse{AreaInfo: areas})
}
