package tok

import (
	"math"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type encL struct {
	ints   []int32
	tokens []string
}

type byEnc struct{ encL }

func (o byEnc) Less(i, j int) bool {
	return o.ints[i] < o.ints[j]
}

func (o byEnc) Len() int { return len(o.ints) }

func (o byEnc) Swap(i, j int) {
	o.ints[i], o.ints[j] = o.ints[j], o.ints[i]
	o.tokens[i], o.tokens[j] = o.tokens[j], o.tokens[i]
}

func TestIntEncoding(t *testing.T) {
	a := int32(2<<24 + 10)
	b := int32(-2<<24 - 1)
	c := int32(math.MaxInt32)
	d := int32(math.MinInt32)
	enc := encL{}
	arr := []int32{a, b, c, d, 1, 2, 3, 4, -1, -2, -3, 0, 234, 10000, 123, -1543}
	enc.ints = arr
	for _, it := range arr {
		encoded, err := encodeInt(int32(it))
		require.NoError(t, err)
		enc.tokens = append(enc.tokens, encoded[0])
	}
	sort.Sort(byEnc{enc})
	for i := 1; i < len(enc.tokens); i++ {
		// The corresponding string tokens should be greater.
		require.True(t, enc.tokens[i-1] < enc.tokens[i])
	}
}
