package common

import (
	"golang.org/x/crypto/blake2b"
)

func Blake2bHash(in []byte) (Hash, error) {
	h, err := blake2b.New256(nil)
	if err != nil {
		return [32]byte{}, err
	}

	var res []byte
	_, err = h.Write(in)
	if err != nil {
		return [32]byte{}, err
	}

	res = h.Sum(nil)
	var buf = [32]byte{}
	copy(buf[:], res)
	return buf, err
}
