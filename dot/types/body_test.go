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

package types

import (
	"reflect"
	"testing"
)

func TestBodyToExtrinsics(t *testing.T) {
	exts := []Extrinsic{{1, 2, 3}, {7, 8, 9, 0}, {0xa, 0xb}}

	body, err := NewBodyFromExtrinsics(exts)
	if err != nil {
		t.Fatal(err)
	}

	res, err := body.AsExtrinsics()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res, exts) {
		t.Fatalf("Fail: got %x expected %x", res, exts)
	}
}
