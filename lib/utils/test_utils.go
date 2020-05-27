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

package utils

import (
	"fmt"
	"os"
	"path"
	"testing"
)

// TestDir test data directory
const TestDir = "./test_data"

// NewTestDir create new test data directory
func NewTestDir(t *testing.T) string {
	testDir := path.Join(TestDir, t.Name())

	err := os.Mkdir(TestDir, os.ModePerm)
	if err != nil && !PathExists(TestDir) {
		fmt.Println(fmt.Errorf("failed to create test directory: %s", err))
	}

	err = os.Mkdir(testDir, os.ModePerm)
	if err != nil && !PathExists(testDir) {
		fmt.Println(fmt.Errorf("failed to create test directory: %s", err))
	}

	return testDir
}

// NewTestBasePath create new test data directory
func NewTestBasePath(t *testing.T, name string) string {
	testDir := path.Join(TestDir, t.Name())
	basePath := path.Join(testDir, name)

	err := os.Mkdir(TestDir, os.ModePerm)
	if err != nil && !PathExists(TestDir) {
		fmt.Println(fmt.Errorf("failed to create test directory: %s", err))
	}

	err = os.Mkdir(testDir, os.ModePerm)
	if err != nil && !PathExists(testDir) {
		fmt.Println(fmt.Errorf("failed to create test directory: %s", err))
	}

	err = os.Mkdir(basePath, os.ModePerm)
	if err != nil && !PathExists(basePath) {
		fmt.Println(fmt.Errorf("failed to create test data directory: %s", err))
	}

	return basePath
}

// RemoveTestDir removes the test data directory
func RemoveTestDir(t *testing.T) {
	testDir := path.Join(TestDir, t.Name())
	err := os.RemoveAll(testDir)
	if err != nil && !PathExists(testDir) {
		fmt.Println(fmt.Errorf("failed to remove test directory: %s", err))
	}
}
