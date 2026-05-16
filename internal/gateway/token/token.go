// Package token provides lightweight session token generation for the Gateway.
//
// SECURITY: The current implementation generates opaque random tokens.
// This is a Phase 1 skeleton suitable for development and single-vps staging.
// Production hardening needed before real user authentication:
//   - Sign tokens with HMAC or JWT using a config-driven secret.
//   - Validate tokens on the game server side.
//   - Add token revocation and expiry enforcement.
//   - Rotate signing keys without downtime.
package token

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

const tokenBytes = 32

// Generator creates session tokens for gateway join responses.
type Generator struct{}

// NewGenerator creates a token generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate creates a new opaque session token with the given expiry.
func (g *Generator) Generate(userID, roomInstanceID string, expiresAt time.Time) (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
