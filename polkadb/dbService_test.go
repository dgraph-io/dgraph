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

func newTestDBService() (*DbService, func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "test_data")
	if err != nil {
		panic("failed to create test file: " + err.Error())
	}
	db, err := NewDatabaseService(dir)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	return db, func() {
		db.Stop()
		if err := os.RemoveAll(dir); err != nil {
			fmt.Println("removal of temp directory test_data failed")
		}
	}
}

func TestDbService_Start(t *testing.T) {
	db, remove := newTestDBService()
	defer remove()

	err := db.Start()
	if err == nil {
		t.Fatalf("get returned wrong result, got %v", err)
	}
}

func TestDb_Close(t *testing.T) {
	db, remove := newTestDBService()
	defer remove()

	err := db.StateDB.Db.Close()
	if err != nil {
		t.Fatalf("get returned wrong result, got %v", err)
	}
}
