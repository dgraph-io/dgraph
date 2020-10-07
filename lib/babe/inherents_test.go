package babe

import (
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/runtime/wasmer"
	"github.com/ChainSafe/gossamer/lib/scale"

	"github.com/stretchr/testify/require"
)

func TestInherentExtrinsics_Timestamp(t *testing.T) {
	rt := wasmer.NewTestInstance(t, runtime.NODE_RUNTIME)

	idata := NewInherentsData()
	err := idata.SetInt64Inherent(Timstap0, uint64(time.Now().Unix()))
	require.NoError(t, err)

	ienc, err := idata.Encode()
	require.NoError(t, err)

	ret, err := rt.InherentExtrinsics(ienc)
	require.NoError(t, err)

	exts, err := scale.Decode(ret, [][]byte{})
	require.NoError(t, err)

	for _, ext := range exts.([][]byte) {
		in, err := scale.Encode(ext)
		require.NoError(t, err)

		t.Log(in)

		ret, err := rt.ApplyExtrinsic(in)
		require.NoError(t, err)
		require.Equal(t, []byte{0, 0}, ret)
	}
}

func TestInherentExtrinsics_Finalnum(t *testing.T) {
	rt := wasmer.NewTestInstance(t, runtime.NODE_RUNTIME)

	idata := NewInherentsData()
	err := idata.SetInt64Inherent(Timstap0, uint64(time.Now().Unix()))
	require.NoError(t, err)

	err = idata.SetBigIntInherent(Finalnum, big.NewInt(1))
	require.NoError(t, err)

	ienc, err := idata.Encode()
	require.NoError(t, err)

	ret, err := rt.InherentExtrinsics(ienc)
	require.NoError(t, err)

	exts, err := scale.Decode(ret, [][]byte{})
	require.NoError(t, err)

	for _, ext := range exts.([][]byte) {
		in, err := scale.Encode(ext) //nolint
		require.NoError(t, err)

		ret, err := rt.ApplyExtrinsic(in) //nolint
		require.NoError(t, err)
		require.Equal(t, []byte{0, 0}, ret)
	}
}
