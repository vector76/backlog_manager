package model

import (
	"crypto/rand"
	"math/big"
	"strings"
)

const idChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// GenerateFeatureID generates a collision-resistant feature ID with the "ft-" prefix.
// It starts with 4 random alphanumeric characters and escalates length if a collision
// is detected (via the exists function).
func GenerateFeatureID(exists func(id string) bool) (string, error) {
	for length := 4; length <= 8; length++ {
		id, err := randomID("ft-", length)
		if err != nil {
			return "", err
		}
		if !exists(id) {
			return id, nil
		}
		// At max length, keep trying without increasing further
		if length == 8 {
			for {
				id, err = randomID("ft-", length)
				if err != nil {
					return "", err
				}
				if !exists(id) {
					return id, nil
				}
			}
		}
	}
	// unreachable
	return "", nil
}

func randomID(prefix string, length int) (string, error) {
	var b strings.Builder
	b.WriteString(prefix)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(idChars))))
		if err != nil {
			return "", err
		}
		b.WriteByte(idChars[n.Int64()])
	}
	return b.String(), nil
}
