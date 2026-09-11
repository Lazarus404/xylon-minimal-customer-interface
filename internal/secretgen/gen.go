package secretgen

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const defaultBytes = 32

// Generate returns a base64-encoded secret from cryptographically random bytes.
func Generate() (string, error) {
	buf := make([]byte, defaultBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}
