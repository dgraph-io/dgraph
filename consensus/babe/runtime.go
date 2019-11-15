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

package babe

import (
	scale "github.com/ChainSafe/gossamer/codec"
)

// gets the configuration data for Babe from the runtime
func (b *Session) configurationFromRuntime() error {
	ret, err := b.rt.Exec("BabeApi_configuration", 1, []byte{})
	if err != nil {
		return err
	}

	bc := new(BabeConfiguration)
	bc.GenesisAuthorities = []AuthorityData{}
	_, err = scale.Decode(ret, bc)

	if err != nil {
		return err
	}

	// Directly set the babe session's config
	b.config = bc

	return err
}
