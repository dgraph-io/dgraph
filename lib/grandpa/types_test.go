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

package grandpa

import (
	"bytes"
	"testing"

	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/scale"

	"github.com/stretchr/testify/require"
)

func TestPubkeyToVoter(t *testing.T) {
	voters := newTestVoters()
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	state := NewState(voters, 0, 0)
	voter, err := state.pubkeyToVoter(kr.Alice().Public().(*ed25519.PublicKey))
	require.NoError(t, err)
	require.Equal(t, voters[0], voter)
}

func TestJustificationEncoding(t *testing.T) {
	just := &Justification{
		Vote:        testVote,
		Signature:   testSignature,
		AuthorityID: testAuthorityID,
	}

	enc, err := just.Encode()
	require.NoError(t, err)

	rw := &bytes.Buffer{}
	rw.Write(enc)
	dec, err := new(Justification).Decode(rw)
	require.NoError(t, err)
	require.Equal(t, just, dec)
}

func TestJustificationArrayEncoding(t *testing.T) {
	just := []*Justification{
		{
			Vote:        testVote,
			Signature:   testSignature,
			AuthorityID: testAuthorityID,
		},
	}

	enc, err := scale.Encode(just)
	require.NoError(t, err)

	dec, err := scale.Decode(enc, make([]*Justification, 1))
	require.NoError(t, err)
	require.Equal(t, just, dec.([]*Justification))
}

func TestFullJustification(t *testing.T) {
	just := &Justification{
		Vote:        testVote,
		Signature:   testSignature,
		AuthorityID: testAuthorityID,
	}

	fj := FullJustification([]*Justification{just})
	enc, err := fj.Encode()
	require.NoError(t, err)

	rw := &bytes.Buffer{}
	rw.Write(enc)
	dec, err := FullJustification{}.Decode(rw)
	require.NoError(t, err)
	require.Equal(t, fj, dec)
}
