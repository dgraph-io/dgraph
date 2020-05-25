// Copyright 2019 ChainSafe Systems (ON) Corp.
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

package keystore

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto"
	"github.com/stretchr/testify/require"
)

func TestLoadKeystore(t *testing.T) {
	ks, err := LoadKeystore("alice")
	require.Nil(t, err)

	require.Equal(t, 1, ks.NumSr25519Keys())
}

var testKeyTypes = []struct {
	testType     string
	expectedType string
}{
	{testType: "babe", expectedType: crypto.Sr25519Type},
	{testType: "gran", expectedType: crypto.Ed25519Type},
	{testType: "acco", expectedType: crypto.Sr25519Type},
	{testType: "aura", expectedType: crypto.Sr25519Type},
	{testType: "imon", expectedType: crypto.Sr25519Type},
	{testType: "audi", expectedType: crypto.Sr25519Type},
	{testType: "dumy", expectedType: crypto.Sr25519Type},
	{testType: "xxxx", expectedType: "unknown keytype"},
}

func TestDetermineKeyType(t *testing.T) {
	for _, test := range testKeyTypes {
		output := DetermineKeyType(test.testType)
		require.Equal(t, test.expectedType, output)
	}
}
