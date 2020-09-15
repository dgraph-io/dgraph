package types

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

func TestGrandpaAuthorityDataRaw(t *testing.T) {
	ad := new(GrandpaAuthorityDataRaw)
	buf := &bytes.Buffer{}
	data, _ := common.HexToBytes("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d714103640000000000000000b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d7170100000000000000")
	buf.Write(data)

	ad, err := ad.Decode(buf)
	require.NoError(t, err)
	require.Equal(t, uint64(0), ad.ID)
	require.Equal(t, "eea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364", fmt.Sprintf("%x", ad.Key))
}

func TestGrandpaAuthorityDataRawToAuthorityData(t *testing.T) {
	ad := make([]*GrandpaAuthorityDataRaw, 2)
	buf := &bytes.Buffer{}
	data, _ := common.HexToBytes("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d714103640000000000000000b64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d7170100000000000000")
	buf.Write(data)

	authA, _ := common.HexToHash("0xeea1eabcac7d2c8a6459b7322cf997874482bfc3d2ec7a80888a3a7d71410364")
	authB, _ := common.HexToHash("0xb64994460e59b30364cad3c92e3df6052f9b0ebbb8f88460c194dc5794d6d717")

	expected := []*GrandpaAuthorityDataRaw{
		{Key: authA, ID: 0},
		{Key: authB, ID: 1},
	}

	var err error
	for i := range ad {
		ad[i], err = ad[i].Decode(buf)
		require.NoError(t, err)
	}

	require.Equal(t, expected, ad)
}
