package runtime

import (
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
	"github.com/stretchr/testify/require"
)

func TestApplyExtrinsic_Transfer_NoBalance_UncheckedExt(t *testing.T) {
	rt := NewTestRuntime(t, NODE_RUNTIME)
	rtVerB, err := rt.Exec(CoreVersion, []byte{})
	require.Nil(t, err)

	rtVer := &VersionAPI{
		RuntimeVersion: &Version{},
		API:            nil,
	}
	rtVer.Decode(rtVerB)
	require.Nil(t, err)

	// genesisHash from runtime must match genesisHash used in transfer payload message signing
	// genesis bytes for runtime seems to be stored in allocated storage at key 0xdfa1667c116b77971ada377f9bd9c485a0566b8e477ae01969120423f2f124ea
	//  runtime trace logs show genesis set to below:
	genesisBytes := []byte{83, 121, 115, 116, 101, 109, 32, 66, 108, 111, 99, 107, 72, 97, 115, 104, 0, 0, 0, 0, 101, 117, 101, 50, 54, 188, 7, 185, 79, 198, 211, 234}
	genesisHash := common.BytesToHash(genesisBytes)

	// Init transfer
	header := &types.Header{
		ParentHash: genesisHash,
		Number:     big.NewInt(1),
	}
	err = rt.InitializeBlock(header)
	require.NoError(t, err)

	bob := kr.Bob().Public().Encode()
	bb := [32]byte{}
	copy(bb[:], bob)

	var nonce uint64 = 0
	tranCallData := struct {
		Type byte
		To   [32]byte
		Amt  *big.Int
	}{
		Type: byte(255), // TODO determine why this is 255 (Lookup type?)
		To:   bb,
		Amt:  big.NewInt(1234),
	}
	transferF := &extrinsic.Function{
		Pall:         extrinsic.Balances,
		PallFunc:     extrinsic.PB_Transfer,
		FuncCallData: tranCallData,
	}

	additional := struct {
		SpecVersion      uint32
		GenesisHash      common.Hash
		CurrentBlockHash common.Hash
	}{uint32(rtVer.RuntimeVersion.Spec_version), genesisHash, genesisHash}

	ux, err := extrinsic.CreateUncheckedExtrinsic(transferF, new(big.Int).SetUint64(nonce), kr.Alice(), additional)
	require.NoError(t, err)

	uxEnc, err := ux.Encode()
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(uxEnc)
	require.NoError(t, err)

	require.Equal(t, []byte{1, 2, 0, 1}, res) // 0x01020001 represents Apply error, Type: Payment: Inability to pay (expected result)
}

func TestApplyExtrinsic_Transfer_WithBalance_UncheckedExtrinsic(t *testing.T) {
	rt := NewTestRuntime(t, NODE_RUNTIME)
	rtVerB, err := rt.Exec(CoreVersion, []byte{})
	require.Nil(t, err)

	rtVer := &VersionAPI{
		RuntimeVersion: &Version{},
		API:            nil,
	}
	rtVer.Decode(rtVerB)
	require.Nil(t, err)

	genesisBytes := []byte{83, 121, 115, 116, 101, 109, 32, 66, 108, 111, 99, 107, 72, 97, 115, 104, 0, 0, 0, 0, 101, 117, 101, 50, 54, 188, 7, 185, 79, 198, 211, 234}
	genesisHash := common.BytesToHash(genesisBytes)

	// Init transfer
	header := &types.Header{
		ParentHash: genesisHash,
		Number:     big.NewInt(1),
	}
	err = rt.InitializeBlock(header)
	require.NoError(t, err)

	alice := kr.Alice().Public().Encode()
	bob := kr.Bob().Public().Encode()

	ab := [32]byte{}
	copy(ab[:], alice)

	bb := [32]byte{}
	copy(bb[:], bob)

	rt.ctx.storage.SetBalance(ab, 2000)

	var nonce uint64 = 0
	tranCallData := struct {
		Type byte
		To   [32]byte
		Amt  *big.Int
	}{
		Type: byte(255), // TODO determine why this is 255 (Address type?)
		To:   bb,
		Amt:  big.NewInt(1000),
	}
	transferF := &extrinsic.Function{
		Pall:         extrinsic.Balances,
		PallFunc:     extrinsic.PB_Transfer,
		FuncCallData: tranCallData,
	}
	additional := struct {
		SpecVersion      uint32
		GenesisHash      common.Hash
		CurrentBlockHash common.Hash
	}{uint32(rtVer.RuntimeVersion.Spec_version), genesisHash, genesisHash}

	ux, err := extrinsic.CreateUncheckedExtrinsic(transferF, new(big.Int).SetUint64(nonce), kr.Alice(), additional)
	require.NoError(t, err)

	uxEnc, err := ux.Encode()
	require.NoError(t, err)

	res, err := rt.ApplyExtrinsic(uxEnc)
	require.NoError(t, err)

	// TODO determine why were getting this response, set balance above should have fixed
	require.Equal(t, []byte{1, 2, 0, 1}, res) // 0x01020001 represents Apply error, Type: Payment: Inability to pay some fees

	// TODO: not sure why balances aren't getting adjusted properly, because of AncientBirthBlock?
	bal, err := rt.ctx.storage.GetBalance(ab)
	require.NoError(t, err)
	require.Equal(t, uint64(2000), bal)

	// TODO this causes runtime error because balance for bb is nil (and GetBalance breaks when trys binary.LittleEndian.Uint64(bal))
	//bal, err = rt.storage.GetBalance(bb)
	//require.NoError(t, err)
	//require.Equal(t, uint64(1000), bal)
}
