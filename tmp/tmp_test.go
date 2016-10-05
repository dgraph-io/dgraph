package tmp

import (
	"fmt"
	"testing"

	"github.com/facebookgo/ensure"
	"github.com/stretchr/testify/assert"
)

func TestBasic(t *testing.T) {
	a := []uint64{5, 6}
	b := []int{5, 6}
	assert.EqualValues(t, a, b)
}

func TestBasic2(t *testing.T) {
	a := []uint64{5, 6}
	b := []uint64{5, 6}
	ensure.DeepEqual(t, a, b, fmt.Sprintf("Extra %s", "zzz"))
}
