package algo

import (
	"fmt"
	"testing"

	"github.com/dgraph-io/dgraph/codec"
)

func TestIntersectCompressedWithLinJump(t *testing.T) {
	N := 100
	enc := codec.Encoder{BlockSize: 10}
	for i := 0; i < N; i += 10 {
		enc.Add(uint64(i))
	}
	pack := enc.Done()
	dec := codec.Decoder{Pack: pack}

	v := make([]uint64, 0)
	for i := 0; i < N; i += 10 {
		v = append(v, uint64(i))
	}

	o := make([]uint64, 0)
	IntersectCompressedWithLinJump(&dec, v, &o)

	fmt.Println(v)
	fmt.Println(o)
}
