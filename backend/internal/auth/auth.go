// Package auth provides player session token hashing and the request
// principal used by HTTP middleware/handlers. Player tokens are never
// stored in plaintext: only an HMAC-SHA256 hash (keyed by
// PLAYER_TOKEN_SECRET) is persisted, so a database leak alone cannot be
// used to impersonate a player.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
)

type ctxKey int

const principalCtxKey ctxKey = 1

// Role is the caller's role within the club relevant to the current request.
type Role string

const (
	RoleHost   Role = "HOST"
	RolePlayer Role = "PLAYER"
)

// Principal identifies the authenticated player for the current request.
type Principal struct {
	PlayerID uuid.UUID
}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalCtxKey).(Principal)
	return p, ok
}

// Hasher computes deterministic HMAC-SHA256 hashes of raw player tokens
// using the server-side secret, so lookups can be done by hash equality
// without ever storing the raw token.
type Hasher struct {
	secret []byte
}

func NewHasher(secret string) Hasher {
	return Hasher{secret: []byte(secret)}
}

func (h Hasher) Hash(rawToken string) string {
	mac := hmac.New(sha256.New, h.secret)
	mac.Write([]byte(rawToken))
	return hex.EncodeToString(mac.Sum(nil))
}
