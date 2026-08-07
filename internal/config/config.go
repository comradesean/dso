// Package config defines the server configuration and loads it from defaults
// overlaid with DSO_-prefixed environment variables. Environment-first keeps
// the single-image, env-toggled deployment model simple for containers; a file
// loader can be layered on later without changing consumers.
package config

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// GameType selects which game the server hosts.
type GameType string

const (
	GameDarkSouls2 GameType = "DarkSouls2"
	GameDarkSouls3 GameType = "DarkSouls3"
)

// Platform selects platform-specific defaults (auth mode, app-version profile).
type Platform string

const (
	PlatformPC  Platform = "PC"
	PlatformPS3 Platform = "PS3"
)

// AuthMode selects the ticket validator.
type AuthMode string

const (
	AuthNoop  AuthMode = "noop"
	AuthPSN   AuthMode = "psn"
	AuthSteam AuthMode = "steam"
)

// Config is the full server configuration.
type Config struct {
	Game     GameType
	Platform Platform

	BindAddress             string // interface to listen on
	AdvertiseAddress        string // address handed to clients (must be reachable)
	AdvertisePrivateAddress string // address for peers on a private subnet (blank = same)

	LoginPort int
	AuthPort  int
	GamePort  int
	// DatabasePath is the SQLite file for persisted content. ":memory:" keeps
	// everything ephemeral, which is useful for testing.
	DatabasePath string

	KeyDir            string // RSA key pair directory (generated on first run)
	AuthModeValue     AuthMode
	AllowInsecureAuth bool

	// Version gate. During PS3 bring-up the real app version is unknown, so the
	// gate is lenient by default.
	EnforceAppVersion bool
	AppVersionMin     uint64
	AppVersionMax     uint64

	LogLevel  string
	LogFormat string // "text" or "json"

	// DebugRaw hexdumps all raw bytes on the login/auth connections, for
	// reverse-engineering an unknown client's wire format.
	DebugRaw bool

	// DebugForceBreakInReject makes every invasion attempt fail immediately with a
	// rejection push instead of invading. It exists to test which push alias the
	// client actually recognises as a rejection: provoking a real refusal needs
	// two players in an awkward state, whereas this fires on every orb use.
	//
	// Leave off for play.
	DebugForceBreakInReject bool

	// BreakInRejectPushID overrides the alias used for the rejection push.
	//
	// The client registers SIXTEEN aliases across 0x03B9-0x03C8 for four message
	// types, in four groups of four (registration order):
	//
	//	group 1: 0x3BD 0x3BE 0x3C0 0x3BF
	//	group 2: 0x3C1 0x3C2 0x3C4 0x3C3
	//	group 3: 0x3B9 0x3BA 0x3BC 0x3BB   <- BreakInTarget, 0x3B9 confirmed live
	//	group 4: 0x3C5 0x3C6 0x3C8 0x3C7
	//
	// Each group is four aliases of ONE type, so the reject push must lead one of
	// groups 1, 2 or 4 — the candidates are 0x3BD, 0x3C1 and 0x3C5. Being
	// configurable means all three can be tried in one session without a rebuild.
	//
	// Zero keeps the built-in default.
	BreakInRejectPushID uint64

	// ManagementText is pushed to the client after login as a
	// ManagementTextMessage. It is the only free-text server->client channel the
	// DS2 client has, and is the candidate mechanism behind the Majula obelisk
	// (whose offline text is "the letters are worn beyond recognition"). Empty
	// disables the push. ManagementTextLanguage is 0 for JP, 1 for EN.
	ManagementText         string
	ManagementTextLanguage uint64

	// RegulationPush* drive the 0x038B RegulationFileUpdatePushMessage, which
	// replaces one whole resource file in the running client — no restart, no
	// calibration download. See tasks/regulation-push-038b.md.
	//
	// RegulationPushFile is the payload; empty disables the push. For a .param
	// it MUST be exactly the same size as the resource it replaces, or the
	// client skips it in silence.
	//
	// RegulationPushPath is the `path` field. Empty uses the payload's base
	// name, which is what the client expects — it prepends "param:/" itself for
	// anything that is not a .fmg. Overridable because the client's path
	// normalisation was never fully decoded.
	//
	// RegulationPushVersionRequired must match the regulation version the client
	// already holds; 0 makes the client skip the check. RegulationPushVersionNew
	// becomes its new version and defaults to required+1.
	//
	// RegulationPushDelaySeconds delays the push after login, so it can be timed
	// against a deliberate area reload — some consumers may only re-read their
	// param when their object is registered at map load.
	RegulationPushFile            string
	RegulationPushPath            string
	RegulationPushVersionRequired uint64
	RegulationPushVersionNew      uint64

	// RegulationPushVersionSweep is a comma-separated list of candidate
	// version_required values. When set, one diff entry is sent per candidate in
	// a single push instead of one entry using RegulationPushVersionRequired.
	//
	// It exists because the client silently drops any entry whose
	// target_regulation_version differs from a value we cannot read and it never
	// reports. At most one candidate can match, so a sweep costs one login where
	// guessing costs one login per value -- and with an FMG payload each entry
	// carries text naming its own candidate, so the game displays the answer.
	RegulationPushVersionSweep string
	RegulationPushDelaySeconds uint64

	// RegulationPushGapSeconds spaces consecutive resource pushes apart.
	//
	// This is not politeness, it is correctness. The applier accepts at most ONE
	// entry per pass — 0x770454 recomputes cr4 after each accept, so every later
	// entry must be strictly greater than the best so far, and we deliberately
	// send the same version every time so the counter never moves. It then
	// destroys the whole diff list (0x77049C), rejected entries included. Two
	// pushes arriving in the same frame therefore means one of them is thrown
	// away, silently, and a lost push is indistinguishable from a wrong path.
	//
	// The applier runs per frame, so anything comfortably above one frame works;
	// the default leaves room for a loading hitch.
	RegulationPushGapSeconds uint64

	// ObeliskText replaces the Majula obelisk's message. Empty leaves it alone.
	//
	// The obelisk is string id 100 of the regulation FMG, which the client
	// registers as the bare resource "regulation.fmg" — the server synthesises a
	// whole FMG around this text and pushes it over 0x038B. "\n" starts a new
	// line. See tasks/regulation-push-038b.md.
	ObeliskText string

	// EventChest* drive the Majula event chest, which is solved: a u16 claim
	// threshold in OnlineEventParam row 0 gates it, and the prize is one row of
	// ItemLotParam2_SvrEvent. Both are pushed over 0x038B. See
	// tasks/majula-event-chest.md.
	//
	// EventChestRotation is the item ids to cycle through, one per period; empty
	// disables the whole feature. EventChestPeriod is a Go duration ("168h" is
	// weekly, as the original event ran). EventChestEpoch anchors the cycle.
	//
	// EventChestThresholdBase is where thresholds start. It must exceed any
	// threshold already claimed on the save, because the game writes the
	// threshold into its per-object counter on claim and will not re-arm while
	// counter >= threshold. We cannot read that counter, so this is set by hand.
	//
	// The two *File entries are the stock params the payloads are built from.
	// They are edited in place and must stay byte-identical in length to what the
	// client loaded, or it discards them in silence.
	EventChestRotation        []uint64
	EventChestPeriod          string
	EventChestEpoch           string
	EventChestThresholdBase   uint64
	EventChestLotParamFile    string
	EventChestOnlineEventFile string

	// Bootstrap HTTP: the DS2 PS3 client does an HTTP "calibration" check before
	// going online. BootstrapHTTPEnabled starts an HTTP server (port 80 by
	// default, needs privilege) that answers it; BootstrapContentsFile is an
	// optional file served for the contents request.
	BootstrapHTTPEnabled  bool
	BootstrapHTTPPort     int
	BootstrapContentsFile string

	// BootstrapCalibrationDir holds the calibration payloads (contents_NNNN.bin
	// and regulation_NNNN.bin). Requests are served by filename from here.
	// Defaults to the directory of BootstrapContentsFile when unset.
	BootstrapCalibrationDir string

	// QuickMatchAutoPair makes the server introduce two players who are both
	// waiting in an arena queue, instead of waiting for one of them to search.
	//
	// The client alternates between advertising and searching, and only searches
	// when the player interacts with the statue — so two players can sit queued
	// indefinitely with neither looking. Observed repeatedly in testing.
	//
	// This is an introduction, not a decision: both players have already declared
	// availability by registering, and the receiving client still chooses whether
	// to allow. But it does mean a join arrives that no client asked for, and
	// whether the other side accepts an allow it did not solicit is UNVERIFIED.
	// Off by default for that reason.
	QuickMatchAutoPair bool

	// CalibrationVersion selects which calibration the client actually receives.
	//
	// The EBOOT hardcodes the contents_0101.bin URL, so a real client can only
	// ever ask for 0101. Setting this to e.g. "0114" answers that request with
	// contents_0114.bin instead. Nothing else needs redirecting: the manifest
	// inside names its own regulation file, and the client fetches that by name.
	//
	// Empty means serve exactly what was asked for.
	CalibrationVersion string

	// DNS redirect: on a real PS3 (no IP/Hosts switch) the console is pointed at
	// this DNS server, which answers the FromSoftware hostnames with our
	// advertise address and forwards everything else upstream.
	DNSEnabled       bool
	DNSPort          int
	DNSUpstream      string
	DNSRedirectHosts []string
}

// Default returns the baseline configuration (DS2 on PS3, lenient auth for
// bring-up).
func Default() Config {
	return Config{
		Game:             GameDarkSouls2,
		Platform:         PlatformPS3,
		BindAddress:      "0.0.0.0",
		AdvertiseAddress: "127.0.0.1",
		// Dark Souls 2 PS3 clients read the login server from Network/SvrList.list,
		// which points at frpg2-ps3-ope.fromsoftware.jp:50011. Redirect that host
		// to us and listen on 50011. Auth/game ports are ours to choose (they are
		// handed to the client in the login/auth responses).
		LoginPort:         50011,
		AuthPort:          50000,
		GamePort:          50010,
		DatabasePath:      "data/dso.db",
		KeyDir:            "data/keys",
		AuthModeValue:     AuthNoop,
		AllowInsecureAuth: true,
		EnforceAppVersion: false,
		LogLevel:          "info",
		LogFormat:         "text",
		// English. The client reads this field as a signed int and the reference
		// server treats it as a language selector, so a wrong value is a plausible
		// reason for the push to be silently ignored on a NA console.
		ManagementTextLanguage: 1,
		// One applier pass accepts one entry and discards the rest, so pushes have
		// to arrive in separate frames. See RegulationPushGapSeconds.
		RegulationPushGapSeconds: 2,
		BootstrapHTTPPort:        80,
		DNSPort:                53,
		DNSUpstream:            "8.8.8.8:53",
		DNSRedirectHosts: []string{
			"frpg2-ps3-ope.fromsoftware.jp",
			"frpg2-ps3-internal.s3-website-us-west-2.amazonaws.com",
		},
	}
}

// Load returns the default config overlaid with DSO_-prefixed environment
// variables, then validates it.
func Load() (Config, error) {
	// An env file supplies defaults before anything is read. DSO_ENV_FILE picks a
	// different one; real environment variables still take precedence over both.
	envFile := DefaultEnvFile
	if v, ok := os.LookupEnv("DSO_ENV_FILE"); ok {
		envFile = v
	}
	if err := LoadEnvFile(envFile); err != nil {
		return Config{}, err
	}

	c := Default()

	c.Game = GameType(envStr("DSO_GAME", string(c.Game)))
	c.Platform = Platform(envStr("DSO_PLATFORM", string(c.Platform)))
	c.BindAddress = envStr("DSO_SERVER_BIND_ADDRESS", c.BindAddress)
	c.AdvertiseAddress = envStr("DSO_SERVER_ADVERTISE_ADDRESS", c.AdvertiseAddress)
	c.AdvertisePrivateAddress = envStr("DSO_SERVER_ADVERTISE_PRIVATE_ADDRESS", c.AdvertisePrivateAddress)
	c.LoginPort = envInt("DSO_SERVER_LOGIN_PORT", c.LoginPort)
	c.AuthPort = envInt("DSO_SERVER_AUTH_PORT", c.AuthPort)
	c.GamePort = envInt("DSO_SERVER_GAME_PORT", c.GamePort)
	c.DatabasePath = envStr("DSO_DATABASE_PATH", c.DatabasePath)
	c.KeyDir = envStr("DSO_CRYPTO_KEY_DIR", c.KeyDir)
	c.AuthModeValue = AuthMode(envStr("DSO_AUTH_MODE", string(c.AuthModeValue)))
	c.AllowInsecureAuth = envBool("DSO_ALLOW_INSECURE_AUTH", c.AllowInsecureAuth)
	c.EnforceAppVersion = envBool("DSO_AUTH_ENFORCE_APP_VERSION", c.EnforceAppVersion)
	c.AppVersionMin = envUint("DSO_AUTH_APP_VERSION_MIN", c.AppVersionMin)
	c.AppVersionMax = envUint("DSO_AUTH_APP_VERSION_MAX", c.AppVersionMax)
	c.LogLevel = envStr("DSO_LOGGING_LEVEL", c.LogLevel)
	c.LogFormat = envStr("DSO_LOGGING_FORMAT", c.LogFormat)
	c.DebugRaw = envBool("DSO_DEBUG_RAW", c.DebugRaw)
	c.DebugForceBreakInReject = envBool("DSO_DEBUG_FORCE_BREAKIN_REJECT", c.DebugForceBreakInReject)
	c.BreakInRejectPushID = envUint("DSO_BREAKIN_REJECT_PUSH_ID", c.BreakInRejectPushID)
	c.ManagementText = envStr("DSO_MANAGEMENT_TEXT", c.ManagementText)
	c.ManagementTextLanguage = envUint("DSO_MANAGEMENT_TEXT_LANGUAGE", c.ManagementTextLanguage)

	c.RegulationPushFile = envStr("DSO_REGULATION_PUSH_FILE", c.RegulationPushFile)
	c.RegulationPushPath = envStr("DSO_REGULATION_PUSH_PATH", c.RegulationPushPath)
	c.RegulationPushVersionRequired = envUint("DSO_REGULATION_PUSH_VERSION_REQUIRED", c.RegulationPushVersionRequired)
	c.RegulationPushVersionNew = envUint("DSO_REGULATION_PUSH_VERSION_NEW", c.RegulationPushVersionNew)
	c.RegulationPushVersionSweep = envStr("DSO_REGULATION_PUSH_VERSION_SWEEP", c.RegulationPushVersionSweep)
	c.RegulationPushGapSeconds = envUint("DSO_REGULATION_PUSH_GAP_SECONDS", c.RegulationPushGapSeconds)
	c.ObeliskText = envStr("DSO_OBELISK_TEXT", c.ObeliskText)

	c.EventChestRotation = envUintList("DSO_EVENT_CHEST_ROTATION", c.EventChestRotation)
	c.EventChestPeriod = envStr("DSO_EVENT_CHEST_PERIOD", c.EventChestPeriod)
	c.EventChestEpoch = envStr("DSO_EVENT_CHEST_EPOCH", c.EventChestEpoch)
	c.EventChestThresholdBase = envUint("DSO_EVENT_CHEST_THRESHOLD_BASE", c.EventChestThresholdBase)
	c.EventChestLotParamFile = envStr("DSO_EVENT_CHEST_LOT_PARAM_FILE", c.EventChestLotParamFile)
	c.EventChestOnlineEventFile = envStr("DSO_EVENT_CHEST_ONLINE_EVENT_FILE", c.EventChestOnlineEventFile)
	c.RegulationPushDelaySeconds = envUint("DSO_REGULATION_PUSH_DELAY_SECONDS", c.RegulationPushDelaySeconds)
	c.BootstrapHTTPEnabled = envBool("DSO_BOOTSTRAP_HTTP", c.BootstrapHTTPEnabled)
	c.BootstrapHTTPPort = envInt("DSO_BOOTSTRAP_HTTP_PORT", c.BootstrapHTTPPort)
	c.BootstrapContentsFile = envStr("DSO_BOOTSTRAP_CONTENTS_FILE", c.BootstrapContentsFile)
	c.BootstrapCalibrationDir = envStr("DSO_BOOTSTRAP_CALIBRATION_DIR", c.BootstrapCalibrationDir)
	c.CalibrationVersion = envStr("DSO_CALIBRATION_VERSION", c.CalibrationVersion)
	c.QuickMatchAutoPair = envBool("DSO_QUICKMATCH_AUTOPAIR", c.QuickMatchAutoPair)
	c.DNSEnabled = envBool("DSO_DNS", c.DNSEnabled)
	c.DNSPort = envInt("DSO_DNS_PORT", c.DNSPort)
	c.DNSUpstream = envStr("DSO_DNS_UPSTREAM", c.DNSUpstream)

	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate checks the configuration for consistency, failing fast on values
// that would make the server unreachable or unsafe.
func (c Config) Validate() error {
	switch c.Game {
	case GameDarkSouls2, GameDarkSouls3:
	default:
		return fmt.Errorf("config: unknown game %q", c.Game)
	}
	if _, err := netip.ParseAddr(c.AdvertiseAddress); err != nil {
		return fmt.Errorf("config: invalid advertise_address %q: %w", c.AdvertiseAddress, err)
	}
	if c.AdvertisePrivateAddress != "" {
		if _, err := netip.ParseAddr(c.AdvertisePrivateAddress); err != nil {
			return fmt.Errorf("config: invalid advertise_private_address %q: %w", c.AdvertisePrivateAddress, err)
		}
	}
	for name, port := range map[string]int{"login": c.LoginPort, "auth": c.AuthPort, "game": c.GamePort} {
		if port <= 0 || port > 65535 {
			return fmt.Errorf("config: %s port %d out of range", name, port)
		}
	}
	switch c.AuthModeValue {
	case AuthNoop, AuthPSN, AuthSteam:
	default:
		return fmt.Errorf("config: unknown auth mode %q", c.AuthModeValue)
	}
	return nil
}

// PrivateKeyPath and PublicKeyPath return the on-disk key locations.
func (c Config) PrivateKeyPath() string { return c.KeyDir + "/server.private.pem" }
func (c Config) PublicKeyPath() string  { return c.KeyDir + "/server.public.pem" }

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// envUintList parses a comma-separated list of unsigned integers. A malformed
// element is skipped rather than failing the whole list: these lists are edited
// by hand in dso.env, and losing every item because one has a stray character
// would be a worse failure than losing that one.
func envUintList(key string, def []uint64) []uint64 {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	var out []uint64
	for _, f := range strings.Split(raw, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if v, err := strconv.ParseUint(f, 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

func envUint(key string, def uint64) uint64 {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	}
	return def
}
