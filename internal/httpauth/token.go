// Package httpauth contains authentication helpers shared by HTTP endpoints.
package httpauth

import (
	"crypto/sha256"
	"crypto/subtle"
)

// TokenMatches compares authentication tokens without data-dependent timing.
func TokenMatches(provided, expected string) bool {
	providedHash := sha256.Sum256([]byte(provided))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(providedHash[:], expectedHash[:]) == 1
}
