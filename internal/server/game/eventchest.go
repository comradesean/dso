package game

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

// The Majula event chest.
//
// SOLVED AND REPRODUCED LIVE 2026-08-06 — see tasks/majula-event-chest.md for the
// full chain. The short version:
//
//   - Majula holds two chest objects at identical coordinates. 10045500 is the
//     ordinary one (Soul Vessel). 10045510 is the event one, and its component
//     reads a different item lot: ItemLotParam2_SvrEvent row 10045500.
//   - That component refuses to arm while a per-object claim counter is >= a
//     u16 threshold in OnlineEventParam row 0 at +2. That threshold is zero in
//     every regulation FromSoftware ever published, so 0 >= 0 held forever and
//     the chest could not open offline.
//   - On a successful claim the game writes the threshold INTO the counter. So
//     each new, higher threshold reopens the chest exactly once. That is the
//     whole weekly-rotation mechanism, and it is entirely client-side.
//
// Which makes a rotation two same-size edits pushed over 0x038B: raise the
// threshold to re-arm, and rewrite the lot row to change the prize.
const (
	// eventChestLotRow is the ItemLotParam2_SvrEvent row the event chest reads.
	// It is the only map-object lot id present in that table at all.
	eventChestLotRow = 10045500

	// eventChestItemOffset is the item id within an ITEM_LOT_PARAM2 row.
	//
	// CONFIRMED by claim: the row held 0x0399EFA0 = 60420000 here, and the chest
	// handed over a Torch.
	eventChestItemOffset = 0x2C

	// eventChestThresholdOffset is the claim threshold within OnlineEventParam
	// row 0 — the u16 at +2, read by the arm method at 0x58E360.
	eventChestThresholdOffset = 2

	onlineEventParamName = "OnlineEventParam.param"
	svrEventLotParamName = "ItemLotParam2_SvrEvent.param"
)

// eventChestPushes arms the chest with the prize for the current period.
//
// Two pushes, never two entries in one. The applier accepts at most ONE entry
// per pass: 0x770438 rejects anything not strictly greater than the best
// accepted so far, and 0x770454 recomputes that comparison after each accept, so
// two entries carrying the same version means the second is dropped. We send the
// same version every time deliberately, so bundling would guarantee the loss.
//
// The caller spaces them apart in time for the same reason — see
// sendResourcePushes. Landing in one frame is as bad as sharing one push.
//
// The lot goes first so that if only one lands, the chest stays shut rather than
// opening on the wrong prize.
func (s *Service) eventChestPushes(log logger) []resourcePush {
	cfg := s.srv.Config
	if len(cfg.EventChestRotation) == 0 {
		return nil
	}

	period := s.eventChestPeriod()
	index := int64(0)
	if period > 0 {
		if elapsed := time.Since(s.eventChestEpoch()); elapsed > 0 {
			index = int64(elapsed / period)
		}
	}

	item := cfg.EventChestRotation[index%int64(len(cfg.EventChestRotation))]

	// The threshold only has to exceed the client's stored counter, and we cannot
	// read that. Making it climb with the period keeps it ahead of any counter a
	// previous period wrote, and the base covers thresholds already spent before
	// the rotation was switched on.
	threshold := cfg.EventChestThresholdBase + uint64(index)
	if threshold > 0xFFFF {
		log.Warn("event chest: threshold would overflow u16, rotation halted",
			"threshold", threshold)
		return nil
	}

	lot, err := os.ReadFile(cfg.EventChestLotParamFile)
	if err != nil {
		log.Warn("event chest: cannot read lot param", "file", cfg.EventChestLotParamFile, "err", err)
		return nil
	}
	if err := setParamRowU32(lot, eventChestLotRow, eventChestItemOffset, uint32(item)); err != nil {
		log.Warn("event chest: cannot set item", "err", err)
		return nil
	}

	online, err := os.ReadFile(cfg.EventChestOnlineEventFile)
	if err != nil {
		log.Warn("event chest: cannot read online event param", "file", cfg.EventChestOnlineEventFile, "err", err)
		return nil
	}
	if err := setParamRowU16(online, 0, eventChestThresholdOffset, uint16(threshold)); err != nil {
		log.Warn("event chest: cannot set threshold", "err", err)
		return nil
	}

	log.Info("event chest rotation",
		"period_index", index,
		"item_id", item,
		"threshold", threshold)

	return []resourcePush{
		{path: svrEventLotParamName, data: lot},
		{path: onlineEventParamName, data: online},
	}
}

func (s *Service) eventChestPeriod() time.Duration {
	d, err := time.ParseDuration(s.srv.Config.EventChestPeriod)
	if err != nil || d <= 0 {
		return 168 * time.Hour // one week, as the original event ran
	}
	return d
}

func (s *Service) eventChestEpoch() time.Time {
	t, err := time.Parse(time.RFC3339, s.srv.Config.EventChestEpoch)
	if err != nil {
		return time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	}
	return t
}

// paramRowOffset finds a row's data offset in a PARAM file.
//
// Layout is documented in docs/regulation-format.md section 8: row count is the
// u16 at +0x0A and the row index is rowCount entries of 12 bytes at +0x40, each
// (id, dataOffset, nameOffset), all big-endian.
func paramRowOffset(data []byte, rowID uint32) (int, error) {
	if len(data) < 0x40 {
		return 0, fmt.Errorf("param too short: %d bytes", len(data))
	}
	rowCount := int(binary.BigEndian.Uint16(data[0x0A:0x0C]))
	for i := range rowCount {
		e := 0x40 + i*12
		if e+12 > len(data) {
			return 0, fmt.Errorf("row index runs past end of file")
		}
		if binary.BigEndian.Uint32(data[e:e+4]) == rowID {
			return int(binary.BigEndian.Uint32(data[e+4 : e+8])), nil
		}
	}
	return 0, fmt.Errorf("row %d not found among %d rows", rowID, rowCount)
}

// setParamRowU32 and setParamRowU16 edit a field in place.
//
// In place is not an implementation detail here: the client compares the pushed
// payload's size against the loaded resource's and discards any mismatch without
// a word (0x770DE4). Editing bytes of the real file is the only way to be sure
// the size is right, which is why these take the loaded file rather than
// building one.
func setParamRowU32(data []byte, rowID uint32, offset int, value uint32) error {
	row, err := paramRowOffset(data, rowID)
	if err != nil {
		return err
	}
	if row+offset+4 > len(data) {
		return fmt.Errorf("field at row %d +%d runs past end of file", rowID, offset)
	}
	binary.BigEndian.PutUint32(data[row+offset:], value)
	return nil
}

func setParamRowU16(data []byte, rowID uint32, offset int, value uint16) error {
	row, err := paramRowOffset(data, rowID)
	if err != nil {
		return err
	}
	if row+offset+2 > len(data) {
		return fmt.Errorf("field at row %d +%d runs past end of file", rowID, offset)
	}
	binary.BigEndian.PutUint16(data[row+offset:], value)
	return nil
}
