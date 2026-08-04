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
		KeyDir:            "data/keys",
		AuthModeValue:     AuthNoop,
		AllowInsecureAuth: true,
		EnforceAppVersion: false,
		LogLevel:          "info",
		LogFormat:         "text",
	}
}

// Load returns the default config overlaid with DSO_-prefixed environment
// variables, then validates it.
func Load() (Config, error) {
	c := Default()

	c.Game = GameType(envStr("DSO_GAME", string(c.Game)))
	c.Platform = Platform(envStr("DSO_PLATFORM", string(c.Platform)))
	c.BindAddress = envStr("DSO_SERVER_BIND_ADDRESS", c.BindAddress)
	c.AdvertiseAddress = envStr("DSO_SERVER_ADVERTISE_ADDRESS", c.AdvertiseAddress)
	c.AdvertisePrivateAddress = envStr("DSO_SERVER_ADVERTISE_PRIVATE_ADDRESS", c.AdvertisePrivateAddress)
	c.LoginPort = envInt("DSO_SERVER_LOGIN_PORT", c.LoginPort)
	c.AuthPort = envInt("DSO_SERVER_AUTH_PORT", c.AuthPort)
	c.GamePort = envInt("DSO_SERVER_GAME_PORT", c.GamePort)
	c.KeyDir = envStr("DSO_CRYPTO_KEY_DIR", c.KeyDir)
	c.AuthModeValue = AuthMode(envStr("DSO_AUTH_MODE", string(c.AuthModeValue)))
	c.AllowInsecureAuth = envBool("DSO_ALLOW_INSECURE_AUTH", c.AllowInsecureAuth)
	c.EnforceAppVersion = envBool("DSO_AUTH_ENFORCE_APP_VERSION", c.EnforceAppVersion)
	c.AppVersionMin = envUint("DSO_AUTH_APP_VERSION_MIN", c.AppVersionMin)
	c.AppVersionMax = envUint("DSO_AUTH_APP_VERSION_MAX", c.AppVersionMax)
	c.LogLevel = envStr("DSO_LOGGING_LEVEL", c.LogLevel)
	c.LogFormat = envStr("DSO_LOGGING_FORMAT", c.LogFormat)

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
