package game

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/sstreight/dso/internal/proto/ds2datapb"
	"github.com/sstreight/dso/internal/proto/ds2pb"
)

// opPushRegulationFileUpdate is the PushMessageId for
// RegulationFileUpdatePushMessage.
//
// CONFIRMED, from the same evidence as opPushManagementText: the push
// dispatcher at EBOOT 0x158C138 special-cases three ids by value before its
// red-black-tree lookup — 0x0389 -> 0x1587F60, 0x038B -> 0x158B150, 0x038C ->
// 0x1588218.
const opPushRegulationFileUpdate = 0x038B

// regulationPushSentinelStart / End are the "always valid" window the client
// itself constructs when the fields are absent (handler 0x15F74B8 defaults them
// via cellRtc).
//
// No reader for start_at/end_at was found in the holder or applier module, but
// that is a failed search, not an absence proof — this project has conflated the
// two before, most memorably when the bell receive path was declared dead. Two
// reasons to keep it open: the handler spends real work constructing meaningful
// sentinels where zeroing would satisfy any uninitialised-parse concern for
// free, and the fields are 16-byte structs, so a comparison would take their
// address and could live outside the module that was scanned.
//
// Sending the sentinels costs nothing, reproduces exactly what the client would
// have built for itself, and cannot trip a required-field check if the real
// schema marks them required. See tasks/regulation-push-038b.md.
var (
	regulationPushSentinelStart = &ds2datapb.DateTime{
		Year: proto.Uint32(2000), Month: proto.Uint32(1), Day: proto.Uint32(1),
		Hours: proto.Uint32(0), Minutes: proto.Uint32(0), Seconds: proto.Uint32(0),
		Tzdiff: proto.Uint32(0),
	}
	regulationPushSentinelEnd = &ds2datapb.DateTime{
		Year: proto.Uint32(2100), Month: proto.Uint32(1), Day: proto.Uint32(1),
		Hours: proto.Uint32(0), Minutes: proto.Uint32(0), Seconds: proto.Uint32(0),
		Tzdiff: proto.Uint32(0),
	}
)

// sendRegulationPush pushes one whole replacement resource to a client.
//
// UNVERIFIED END TO END. The receive path is traced from the push dispatcher to
// an applier that runs every frame and reloads the live resource in place, so a
// push should take effect mid-session with no restart. What is NOT established
// is whether the applier's resource repository — reached via *(*(0x1E1D810))+24
// — is the same one the rest of the game reads params from; the Majula chest's
// threshold reader gets there via *(*(0x1E1EAB4+32))+24, a different global. If
// those differ, this lands somewhere nothing looks and fails silently.
//
// That question is far cheaper to answer on a console than in a disassembler,
// which is what this code is for.
//
// Constraints the client enforces, all of which fail SILENTLY:
//
//   - For a .param the payload size must equal the loaded resource's size
//     exactly (0x770DE4). Rows can be edited, never added or removed.
//   - For a .fmg the payload is capped at 1024 bytes (0x76A0F0).
//   - VersionRequired must equal the version the client already holds, unless
//     that is 0, in which case the check is skipped (0x7705A8).
//   - VersionNew must be <= 999999 and, within one push, strictly greater than
//     any entry accepted before it (0x770438). We send a single entry.
func (s *Service) sendRegulationPush(log logger, cs *clientSession) {
	cfg := s.srv.Config
	if cfg.RegulationPushFile == "" {
		return
	}

	data, err := os.ReadFile(cfg.RegulationPushFile)
	if err != nil {
		log.Warn("regulation push: cannot read payload", "file", cfg.RegulationPushFile, "err", err)
		return
	}

	// The client prepends L"param:/" itself for anything that is not a .fmg, so
	// the bare file name is what it expects. The normalisation stage between
	// 0x76FF50 and 0x770200 is undecoded, though, so this stays overridable —
	// if a bare name does nothing, the prefixed form is the next thing to try.
	path := cfg.RegulationPushPath
	if path == "" {
		path = filepath.Base(cfg.RegulationPushFile)
	}

	entries := s.regulationPushEntries(log, path, data)
	if len(entries) == 0 {
		return
	}

	body, err := proto.Marshal(&ds2pb.RegulationFileUpdatePushMessage{
		PushMessageId: ds2pb.PushMessageId(opPushRegulationFileUpdate).Enum(),
		UpdateMsg:     &ds2pb.RegulationFileUpdateMessage{DiffDataList: entries},
	})
	if err != nil {
		log.Warn("regulation push: marshal failed", "err", err)
		return
	}

	cs.conn.SendPush(body)
	log.Info("regulation push sent",
		"player_id", cs.playerID,
		"push_id", fmt.Sprintf("%#04x", opPushRegulationFileUpdate),
		"path", path,
		"payload_bytes", len(data),
		"entries", len(entries),
		"versions_required", cfg.RegulationPushVersionRequired,
		"sweep", cfg.RegulationPushVersionSweep)
}

// regulationPushEntries builds the diff list.
//
// Normally that is one entry. With a sweep configured it is one entry per
// candidate version, which exists because of the check at 0x7705A8:
//
//	holder->current_version != elem.target_regulation_version -> skip, silently
//
// We cannot read holder->current_version from here, and the client reports no
// version we can map onto it — the calibration manifest's own Version field is 1
// in every published calibration, so it carries no information either. Guessing
// one value per login is a slow way to search a space we cannot see into.
//
// The client accepts at most one entry, since at most one candidate can equal a
// single stored value. So sweeping costs one login instead of forty, and where
// the payload is an FMG each entry can carry its own text naming the candidate
// it was built for — which makes the game itself report the answer.
func (s *Service) regulationPushEntries(log logger, path string, data []byte) []*ds2pb.RegulationFileDiffData {
	cfg := s.srv.Config

	candidates := parseVersionSweep(cfg.RegulationPushVersionSweep)
	if len(candidates) == 0 {
		versionNew := cfg.RegulationPushVersionNew
		if versionNew == 0 {
			versionNew = cfg.RegulationPushVersionRequired + 1
		}
		return []*ds2pb.RegulationFileDiffData{
			regulationDiffEntry(path, data, uint32(cfg.RegulationPushVersionRequired), uint32(versionNew)),
		}
	}

	entries := make([]*ds2pb.RegulationFileDiffData, 0, len(candidates))
	for _, v := range candidates {
		payload := data
		if labelled, ok := labelFMG(data, fmt.Sprintf("0x038B APPLIED. VERSION MATCHED = %d", v)); ok {
			payload = labelled
		}
		// version_new must be <= 999999 (0x770418) and, among entries the client
		// accepts, strictly increasing (0x770438). Only one entry can be accepted,
		// so v+1 is safe and keeps the resulting version predictable.
		entries = append(entries, regulationDiffEntry(path, payload, v, min(v+1, 999999)))
	}
	log.Info("regulation push: sweeping candidate versions", "count", len(entries))
	return entries
}

func regulationDiffEntry(path string, data []byte, required, next uint32) *ds2pb.RegulationFileDiffData {
	return &ds2pb.RegulationFileDiffData{
		VersionNew:      proto.Uint32(next),
		VersionRequired: proto.Uint32(required),
		Path:            proto.String(path),
		DiffData:        data,
		StartAt:         regulationPushSentinelStart,
		EndAt:           regulationPushSentinelEnd,
	}
}

func parseVersionSweep(spec string) []uint32 {
	var out []uint32
	for _, f := range strings.Split(spec, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if v, err := strconv.ParseUint(f, 10, 32); err == nil {
			out = append(out, uint32(v))
		}
	}
	return out
}

// labelFMG rewrites the single string in regulationEnglish.fmg in place.
//
// That file is 128 bytes and holds exactly one string, id 100 at offset 0x2C —
// the Majula obelisk, "The letters are worn beyond recognition.", 40 characters
// of UTF-16BE. Replacing it with another 40-character string leaves the file
// length and every offset inside it untouched, so the result is still a valid
// FMG without rebuilding anything.
//
// This is deliberately narrow: it returns false for anything that is not
// byte-for-byte the shape it expects, rather than trying to be a general FMG
// writer. tools/gamedata/regparam.py is the general one.
func labelFMG(data []byte, label string) ([]byte, bool) {
	const stringOff = 0x2C
	const chars = 40

	if len(data) != 128 {
		return nil, false
	}

	runes := []rune(label)
	if len(runes) > chars {
		runes = runes[:chars]
	}
	for len(runes) < chars {
		runes = append(runes, ' ')
	}

	out := make([]byte, len(data))
	copy(out, data)
	for i, r := range runes {
		out[stringOff+i*2] = byte(r >> 8)
		out[stringOff+i*2+1] = byte(r)
	}
	return out, true
}

// maybeSendRegulationPush sends the push after an optional delay.
//
// The delay exists because the applier reloads a resource in place, but the
// chest's arm method (0x58E360) is not known to re-run on its own — it may only
// fire when the object is registered at map load. Pushing after the player is
// already standing in Majula therefore risks changing data nothing re-reads.
// A delay lets the push be timed against a deliberate area reload.
func (s *Service) maybeSendRegulationPush(log logger, cs *clientSession) {
	if s.srv.Config.RegulationPushFile == "" {
		return
	}

	delay := time.Duration(s.srv.Config.RegulationPushDelaySeconds) * time.Second
	if delay <= 0 {
		s.sendRegulationPush(log, cs)
		return
	}

	go func() {
		time.Sleep(delay)
		s.sendRegulationPush(log, cs)
	}()
}
