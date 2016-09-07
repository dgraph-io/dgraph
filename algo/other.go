package algo

import (
	"crypto/rand"
	"encoding/base32"
)

// RandStringBytesMask generates a random string of length n.
func RandStringBytesMask(n int) (string, error) {
	randomBytes := make([]byte, n)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}
	return base32.StdEncoding.EncodeToString(randomBytes), nil
}
