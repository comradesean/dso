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

// regulationPushByteBudget caps the total payload bytes in one push.
//
// The largest message this transport has been seen to carry is ~10.7 KB (13
// fragments at 900 bytes); this leaves headroom above that without inviting a
// 55-fragment message nobody has tested.
const regulationPushByteBudget = 16384

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
// CONFIRMED LIVE (2026-08-06). The client accepts the entry and applies it on the
// next frame — no restart, no calibration download. Proven twice: a replacement
// regulationEnglish.fmg moved the holder's version counter, and a replacement
// OnlineEventParam.param armed the Majula event chest, which reset in front of
// the player and paid out. See tasks/regulation-push-038b.md.
//
// That second result also closed the one question the disassembly could not: the
// applier's resource repository (*(*(0x1E1D810))+24) IS the one the game reads
// params from (*(*(0x1E1EAB4+32))+24), despite the different globals.
//
// Constraints the client enforces. EVERY ONE OF THEM FAILS SILENTLY — no error,
// no log, no visible difference — so a negative result isolates nothing on its
// own. Change one variable at a time, and read the holder's version counter with
// a debugger rather than guessing from in-game effects:
//
//   - VersionRequired must equal the version in the holder at 0x1E1D388. That is
//     the game's BUILD version (11500 = 1.15) at boot, and becomes whatever
//     VersionNew we last sent after each accepted push (0x7705A8, 0x770480). It
//     is skipped only when the holder's value is 0, which it never is once a
//     regulation has loaded. Nothing in the protocol reports it. Sweeping a small
//     window is the practical answer; see regulationPushEntries.
//   - For a .param the payload size must equal the loaded resource's size
//     exactly (0x770DE4). Rows can be edited, never added or removed.
//   - For a .fmg the payload is capped at 1024 bytes (0x76A0F0).
//   - VersionNew must be <= 999999 and, among entries accepted in one push,
//     strictly greater than any accepted before it (0x770438, 0x770418).
//
// One caveat about testing: the .fmg route memcpys into the buffer the file was
// loaded into, so anything that already copied a string out of it keeps showing
// the old text. A visible-text probe can therefore read "no change" on complete
// success, which is how the obelisk misled us. The version counter cannot.
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

	// For a param the client prepends L"param:/" itself, so the bare file name is
	// correct and is what armed the chest. For an FMG it prepends nothing and
	// looks the string up as-is, and an FMG's resource path is nothing like its
	// name inside the archive — see tasks/regulation-push-038b.md.
	//
	// Accepts a comma-separated list so several candidate paths can be tried in
	// one login. Sent as separate pushes rather than separate entries in one:
	// entries share the version chain and at most one is accepted per pass, so
	// bundling them would test exactly one path and quietly discard the rest.
	//
	// DANGER, for .fmg payloads only: DO NOT list speculative paths. A wrong path
	// that MISSES is harmless, but a wrong path that HITS is memory corruption —
	// 0x76A0F0 checks only that the payload is <= 1024 bytes and never compares it
	// against the destination, so it memcpys over whatever resource matched and
	// leaves everything past your bytes stale with a string table pointing into
	// garbage. Sending nine candidates crashed the client twice on load.
	//
	// Params are safe to sweep this way: their route requires an exact size match,
	// so a wrong destination is skipped rather than clobbered.
	paths := parsePushPaths(cfg.RegulationPushPath)
	if len(paths) == 0 {
		paths = []string{filepath.Base(cfg.RegulationPushFile)}
	}
	for _, path := range paths {
		s.pushResource(log, cs, path, data)
	}
}

func parsePushPaths(spec string) []string {
	var out []string
	for _, f := range strings.Split(spec, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// pushResource sends one resource file to one client.
func (s *Service) pushResource(log logger, cs *clientSession, path string, data []byte) {
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
		"sweep", s.srv.Config.RegulationPushVersionSweep)
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
		// Default to leaving the client's version untouched; see the sweep branch
		// below for why incrementing is a trap.
		versionNew := cfg.RegulationPushVersionNew
		if versionNew == 0 {
			versionNew = cfg.RegulationPushVersionRequired
		}
		return []*ds2pb.RegulationFileDiffData{
			regulationDiffEntry(path, data, uint32(cfg.RegulationPushVersionRequired), uint32(versionNew)),
		}
	}

	// Every entry carries its OWN full copy of the payload, so a wide sweep over a
	// large file makes an enormous push. Eleven copies of the 4420-byte lot param
	// is ~48 KB, which at 900-byte fragments is ~55 packets — four times the
	// largest message this transport has been observed to carry.
	//
	// Cap the total and say so when it bites. A silently truncated sweep would
	// look exactly like a working one right up until the version it needed was the
	// one dropped.
	if len(data) > 0 {
		if room := max(1, regulationPushByteBudget/len(data)); room < len(candidates) {
			log.Warn("regulation push: sweep truncated to fit the transport",
				"payload_bytes", len(data),
				"candidates", len(candidates),
				"sending", room,
				"dropped", candidates[room:])
			candidates = candidates[:room]
		}
	}

	entries := make([]*ds2pb.RegulationFileDiffData, 0, len(candidates))
	for _, v := range candidates {
		payload := data
		if labelled, ok := labelFMG(data, fmt.Sprintf("0x038B APPLIED. VERSION MATCHED = %d", v)); ok {
			payload = labelled
		}
		// version_new == version_required, so an accepted push leaves the client's
		// version WHERE IT WAS and the same sweep window keeps working forever.
		//
		// Incrementing looks natural — FromSoftware presumably chained diffs that
		// way — and it is a trap for us. We send up to three pushes per login, so
		// the counter would climb ~3 each time and walk off an eleven-wide window
		// after three or four logins, at which point everything stops with no
		// error anywhere. Widening the window is not an answer either: each entry
		// carries a full copy of the payload, and the lot param alone is 4420
		// bytes.
		//
		// Nothing forbids this. The only bounds are <= 999999 (0x770418) and
		// strictly-greater-than-the-best-accepted-within-this-pass (0x770438,
		// accumulator starts at -1), and at most one entry per push can match a
		// single stored value, so that second rule never binds.
		entries = append(entries, regulationDiffEntry(path, payload, v, v))
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
