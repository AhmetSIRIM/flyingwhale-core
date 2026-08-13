// Package webhook checks an inbound webhook request against a secret the
// receiver and the sender both hold.
package webhook

import (
	"crypto/subtle"
	"strings"
)

const bearerPrefix = "Bearer "

// Authorized reports whether header authenticates a webhook request against
// secret. An empty secret fails closed: it denies every header, including
// an empty one, since a deployment without a configured secret must reject
// every request rather than accept an absent header. The comparison is
// constant-time, and header may present the secret either raw or with a
// "Bearer " prefix.
func Authorized(secret, header string) bool {
	if secret == "" {
		return false
	}
	secretBytes := []byte(secret)
	matched := subtle.ConstantTimeCompare([]byte(header), secretBytes)
	if presented, hasPrefix := strings.CutPrefix(header, bearerPrefix); hasPrefix {
		matched |= subtle.ConstantTimeCompare([]byte(presented), secretBytes)
	}
	return matched == 1
}
