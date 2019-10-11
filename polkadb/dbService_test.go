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

package polkadb

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

// Returns started dbService
func newTestDBService(t *testing.T) (*DbService, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "test_data")
	if err != nil {
		t.Fatal("failed to create temp dir: " + err.Error())
	}
	db, err := NewDbService(dir)
	if err != nil {
		t.Fatal("failed to create test dbService: " + err.Error())
	}
	db.Start()
	return db, func() {
		db.Stop()
		if err := os.RemoveAll(dir); err != nil {
			fmt.Println("removal of temp directory test_data failed")
		}
	}
}

func TestDbService_Start(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "test_data")
	if err != nil {
		t.Fatal("failed to create temp dir: " + err.Error())
	}
	db, err := NewDbService(dir)
	if err != nil {
		t.Fatal("failed to create test dbService: " + err.Error())
	}

	err = db.Start()
	if err != nil {
		t.Fatal(err)
	}
}
