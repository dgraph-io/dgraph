// Copyright 2020 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.
package extrinsic

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"testing"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/stretchr/testify/require"
)

var kr keystore.Sr25519Keyring

type testTransCall struct {
	Type byte
	To   [32]byte
	Amt  *big.Int
}

var testTransFunc *Function

func TestCreateUncheckedExtrinsic(t *testing.T) {
	var nonce uint64 = 0
	signer := kr.Alice()
	genesisHash := common.Hash{}
	additional := struct {
		SpecVersion      uint32
		GenesisHash      common.Hash
		CurrentBlockHash common.Hash
	}{193, genesisHash, genesisHash}

	ux, err := CreateUncheckedExtrinsic(testTransFunc, new(big.Int).SetUint64(nonce), signer, additional)
	require.NoError(t, err)

	require.Equal(t, testTransFunc, &ux.Function)
	require.Equal(t, signer.Public().Encode(), ux.Signature.Address)
	require.Equal(t, 64, len(ux.Signature.Sig))
}

func TestCreateUncheckedExtrinsicUnsigned(t *testing.T) {
	ux, err := CreateUncheckedExtrinsicUnsigned(createFunction())
	require.NoError(t, err)
	require.Equal(t, testTransFunc, &ux.Function)
}

func TestUncheckedExtrinsic_Encode(t *testing.T) {
	var nonce uint64 = 0
	signer := kr.Alice()
	genesisHash := common.Hash{}
	additional := struct {
		SpecVersion      uint32
		GenesisHash      common.Hash
		CurrentBlockHash common.Hash
	}{193, genesisHash, genesisHash}

	ux, err := CreateUncheckedExtrinsic(testTransFunc, new(big.Int).SetUint64(nonce), signer, additional)
	require.NoError(t, err)

	uxEnc, err := ux.Encode()
	require.NoError(t, err)

	require.Equal(t, 141, len(uxEnc))
}

func TestMain(m *testing.M) {
	k, err := keystore.NewSr25519Keyring()
	if err != nil {
		log.Fatal(fmt.Errorf("error initializing keyring"))
	}
	kr = *k
	testTransFunc = createFunction()
	// Start all tests
	code := m.Run()
	os.Exit(code)
}

func createFunction() *Function {
	testCallData := testTransCall{
		Type: byte(255),
		To:   [32]byte{},
		Amt:  big.NewInt(1000),
	}

	return &Function{
		Pall:         Balances,
		PallFunc:     PB_Transfer,
		FuncCallData: testCallData,
	}
}
