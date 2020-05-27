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
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewTestDir tests the NewTestDir method
func TestNewTestDir(t *testing.T) {
	testDir := NewTestDir(t)

	expected := path.Join(TestDir, t.Name())

	require.Equal(t, expected, testDir)
	require.Equal(t, PathExists(testDir), true)

	RemoveTestDir(t)
}

// TestNewTestBasePath tests the NewTestBasePath method
func TestNewTestBasePath(t *testing.T) {
	basePath := "test"

	testDir := NewTestBasePath(t, basePath)

	expected := path.Join(TestDir, t.Name(), basePath)

	require.Equal(t, expected, testDir)
	require.Equal(t, PathExists(testDir), true)

	RemoveTestDir(t)
}

// TestRemoveTestDir tests the RemoveTestDir method
func TestRemoveTestDir(t *testing.T) {
	testDir := NewTestDir(t)

	expected := path.Join(TestDir, t.Name())

	require.Equal(t, expected, testDir)
	require.Equal(t, PathExists(testDir), true)

	RemoveTestDir(t)
	require.Equal(t, PathExists(testDir), false)
}
