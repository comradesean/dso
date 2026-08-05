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
		BootstrapHTTPPort:      80,
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
	c.BootstrapHTTPEnabled = envBool("DSO_BOOTSTRAP_HTTP", c.BootstrapHTTPEnabled)
	c.BootstrapHTTPPort = envInt("DSO_BOOTSTRAP_HTTP_PORT", c.BootstrapHTTPPort)
	c.BootstrapContentsFile = envStr("DSO_BOOTSTRAP_CONTENTS_FILE", c.BootstrapContentsFile)
	c.BootstrapCalibrationDir = envStr("DSO_BOOTSTRAP_CALIBRATION_DIR", c.BootstrapCalibrationDir)
	c.CalibrationVersion = envStr("DSO_CALIBRATION_VERSION", c.CalibrationVersion)
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
