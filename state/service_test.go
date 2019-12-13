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
package state

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestService_Start(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "test_data")
	if err != nil {
		t.Fatal("failed to create temp dir: " + err.Error())
	}
	db := NewService(dir)

	err = db.Start()
	if err != nil {
		t.Fatal(err)
	}
}
