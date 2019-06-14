package common

import (
	"testing"
)

func TestBlake2bHash(t *testing.T) {
	in := []byte{0x1}
	h, err := Blake2bHash(in)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(h)
}
