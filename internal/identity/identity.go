// Package identity abstracts ticket validation so the platform authentication
// (Steam on PC, PSN on PS3) is pluggable. Milestone 1 ships only the no-op
// validator, which accepts any ticket and derives a stable identity string.
package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// Identity is the authenticated player identity.
type Identity struct {
	ID       string // stable per-player key
	Platform string // "psn" | "steam" | "dev"
}

// Validator validates a platform ticket. hintID is the identity string the
// client already supplied (e.g. the steam_id field), which some validators use
// as-is and others cross-check against the ticket.
type Validator interface {
	Validate(ctx context.Context, hintID string, ticket []byte) (Identity, error)
	Name() string
}

// Noop accepts any ticket. It uses hintID as the identity, or a hash of the
// ticket when hintID is empty. Intended for development and LAN use only.
type Noop struct{}

func (Noop) Name() string { return "noop" }

func (Noop) Validate(_ context.Context, hintID string, ticket []byte) (Identity, error) {
	id := hintID
	if id == "" {
		sum := sha256.Sum256(ticket)
		id = "dev-" + hex.EncodeToString(sum[:8])
	}
	return Identity{ID: id, Platform: "dev"}, nil
}
