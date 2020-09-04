package extrinsic

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/keystore"

	"github.com/stretchr/testify/require"
)

func TestAuthoritiesChangeExt_Encode(t *testing.T) {
	t.Skip()

	// TODO: scale isn't working for arrays of [32]byte
	kp, err := sr25519.GenerateKeypair()
	require.NoError(t, err)

	pub := kp.Public().Encode()
	pubk := [32]byte{}
	copy(pubk[:], pub)

	authorities := [][32]byte{pubk}

	ext := NewAuthoritiesChangeExt(authorities)

	enc, err := ext.Encode()
	require.NoError(t, err)

	r := &bytes.Buffer{}
	r.Write(enc)
	res, err := DecodeExtrinsic(r)
	require.NoError(t, err)

	require.Equal(t, ext, res)
}

var alice, _ = common.HexToHash("0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d")
var bob, _ = common.HexToHash("0x90b5ab205c6974c9ea841be688864633dc9ca8a357843eeacf2314649965fe22")

func TestTransfer_AsSignedExtrinsic(t *testing.T) {
	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)

	transfer := NewTransfer(alice, bob, 1000, 1)
	_, err = transfer.AsSignedExtrinsic(kr.Alice().Private().(*sr25519.PrivateKey))
	require.NoError(t, err)
}

func TestTransferExt_Encode(t *testing.T) {
	transfer := NewTransfer(alice, bob, 1000, 1)
	sig := [64]byte{}
	ext := NewTransferExt(transfer, sig)

	enc, err := ext.Encode()
	require.NoError(t, err)

	r := &bytes.Buffer{}
	r.Write(enc)
	res, err := DecodeExtrinsic(r)
	require.NoError(t, err)

	require.Equal(t, ext, res)
}

func TestTransferExt_Decode(t *testing.T) {
	// from substrate test runtime
	enc := []byte{1, 212, 53, 147, 199, 21, 253, 211, 28, 97, 20, 26, 189, 4, 169, 159, 214, 130, 44, 133, 88, 133, 76, 205, 227, 154, 86, 132, 231, 165, 109, 162, 125, 144, 181, 171, 32, 92, 105, 116, 201, 234, 132, 27, 230, 136, 134, 70, 51, 220, 156, 168, 163, 87, 132, 62, 234, 207, 35, 20, 100, 153, 101, 254, 34, 69, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 114, 13, 234, 84, 192, 135, 6, 254, 94, 142, 207, 135, 126, 177, 25, 27, 198, 159, 97, 165, 198, 228, 77, 117, 113, 253, 247, 97, 221, 110, 47, 9, 87, 209, 62, 254, 81, 200, 217, 45, 214, 53, 170, 217, 160, 137, 43, 78, 183, 89, 45, 2, 64, 120, 114, 100, 116, 148, 247, 92, 234, 57, 255, 139}
	r := &bytes.Buffer{}
	r.Write(enc)
	res, err := DecodeExtrinsic(r)
	require.NoError(t, err)

	amount := binary.LittleEndian.Uint64([]byte{69, 0, 0, 0, 0, 0, 0, 0})
	nonce := binary.LittleEndian.Uint64([]byte{1, 0, 0, 0, 0, 0, 0, 0})

	sig, _ := common.HexToBytes("0x720dea54c08706fe5e8ecf877eb1191bc69f61a5c6e44d7571fdf761dd6e2f0957d13efe51c8d92dd635aad9a0892b4eb7592d02407872647494f75cea39ff8b")
	sigb := [sr25519.SignatureLength]byte{}
	copy(sigb[:], sig)

	expected := &TransferExt{
		transfer: &Transfer{
			from:   alice,
			to:     bob,
			amount: amount,
			nonce:  nonce,
		},
		signature: sigb,
	}

	var transfer *TransferExt
	var ok bool

	if transfer, ok = res.(*TransferExt); !ok {
		t.Fatal("Fail: got wrong extrinsic type")
	}

	require.Equal(t, expected, transfer)
}

func TestIncludeDataExt_Encode(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	ext := NewIncludeDataExt(data)

	enc, err := ext.Encode()
	require.NoError(t, err)

	r := &bytes.Buffer{}
	r.Write(enc)
	res, err := DecodeExtrinsic(r)
	require.NoError(t, err)

	require.Equal(t, ext, res)
}

func TestIncludeDataExt_Decode(t *testing.T) {
	enc := []byte{2, 32, 111, 0, 0, 0, 0, 0, 0, 0}
	r := &bytes.Buffer{}
	r.Write(enc)
	res, err := DecodeExtrinsic(r)
	require.NoError(t, err)

	expected := &IncludeDataExt{
		data: []byte{111, 0, 0, 0, 0, 0, 0, 0},
	}

	require.Equal(t, expected, res)
}

func TestStorageChangeExt_Encode(t *testing.T) {
	key := []byte("noot")
	value := optional.NewBytes(true, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0})
	ext := NewStorageChangeExt(key, value)

	enc, err := ext.Encode()
	require.NoError(t, err)

	r := &bytes.Buffer{}
	r.Write(enc)
	res, err := DecodeExtrinsic(r)
	require.NoError(t, err)

	require.Equal(t, ext, res)
}

func TestStorageChangeExt_Decode(t *testing.T) {
	enc := []byte{3, 16, 77, 1, 2, 3, 1, 4, 99}
	r := &bytes.Buffer{}
	r.Write(enc)
	res, err := DecodeExtrinsic(r)
	require.NoError(t, err)

	expected := &StorageChangeExt{
		key:   []byte{77, 1, 2, 3},
		value: optional.NewBytes(true, []byte{99}),
	}

	require.Equal(t, expected, res)
}
