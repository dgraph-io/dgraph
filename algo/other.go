package algo

import "math/rand"

const (
	letterBytes   = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ."
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
)

// RandStringBytesMask generates a random string of length n.
func RandStringBytesMask(n int) string {
	str := make([]byte, n)
	len := len(letterBytes)
	for i := 0; i < n; {
		if idx := int(rand.Int63() & letterIdxMask); idx < len {
			str[i] = letterBytes[idx]
			i++
		}
	}
	return string(str)
}
