package bp128

import (
	"math/rand"
	"testing"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"
)

func TestEncoder(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	for _ = range make([]int, 33) {
		var data []uint64
		last := uint64(rand.Intn(100))
		data = append(data, last)
		size := rand.Intn(10e6)
		if size < 0 {
			size = 1e6
		}
		for i := 1; i < size; i++ {
			last += uint64(rand.Intn(33))
			data = append(data, last)
		}

		var bp BPackEncoder
		uids := make([]uint64, 0, BlockSize)
		for _, uid := range data {
			uids = append(uids, uid)
			if len(uids) == BlockSize {
				bp.PackAppend(uids)
				uids = uids[:0]
			}
		}
		if len(uids) > 0 {
			bp.PackAppend(uids)
		}

		out := make([]byte, bp.Size())
		bp.WriteTo(out)
		t.Logf("Dataset length: %d. Output size: %s",
			len(data), humanize.Bytes(uint64(len(out))))

		// Write done. Now let's read it back.
		var bi BPackIterator
		bi.Init(out, 0)
		var i int

		for bi.StartIdx() < bi.Length() {
			uids := bi.Uids()
			require.Equal(t, data[i:i+len(uids)], uids)
			i += len(uids)

			bi.Next()
			if !bi.Valid() {
				break
			}
		}
		require.Equal(t, len(data), i)
	}
}
