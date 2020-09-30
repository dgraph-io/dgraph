package transaction

import (
	"sort"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

func TestPool(t *testing.T) {
	tests := []*ValidTransaction{
		{
			Extrinsic: []byte("a"),
			Validity:  &Validity{Priority: 1},
		},
		{
			Extrinsic: []byte("b"),
			Validity:  &Validity{Priority: 4},
		},
		{
			Extrinsic: []byte("c"),
			Validity:  &Validity{Priority: 2},
		},
		{
			Extrinsic: []byte("d"),
			Validity:  &Validity{Priority: 17},
		},
		{
			Extrinsic: []byte("e"),
			Validity:  &Validity{Priority: 2},
		},
	}

	p := NewPool()
	hashes := make([]common.Hash, len(tests))
	for i, tx := range tests {
		h := p.Insert(tx)
		hashes[i] = h
	}

	transactions := p.Transactions()
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Extrinsic[0] < transactions[j].Extrinsic[0]
	})
	require.Equal(t, tests, transactions)

	for _, h := range hashes {
		p.Remove(h)
	}
	require.Equal(t, 0, len(p.Transactions()))
}
