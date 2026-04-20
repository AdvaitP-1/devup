package util

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
)

// GenerateRequestID returns a random hex string for idempotency keys.
func GenerateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// EnvMap returns the current process environment as a map.
func EnvMap() map[string]string {
	m := make(map[string]string)
	for _, e := range os.Environ() {
		if idx := strings.IndexByte(e, '='); idx > 0 {
			m[e[:idx]] = e[idx+1:]
		}
	}
	return m
}
