// Package idgen provides cryptographically random ID generation for
// unguessable tokens and human-friendly join codes.
package idgen

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// PlayerToken returns a cryptographically random, URL-safe token suitable
// for accountless player authentication. It must never be logged or stored
// in plaintext.
func PlayerToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

const joinCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I to avoid ambiguity

// JoinCode returns a cryptographically random, human-typeable club join
// code of the given length.
func JoinCode(length int) (string, error) {
	if length <= 0 {
		length = 6
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate join code: %w", err)
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = joinCodeAlphabet[int(b)%len(joinCodeAlphabet)]
	}
	return string(out), nil
}
