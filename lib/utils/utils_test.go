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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPathExists tests the NewTestDir method
func TestPathExists(t *testing.T) {
	require.Equal(t, PathExists("../utils"), true)
	require.Equal(t, PathExists("../utilzzz"), false)
}

// TestHomeDir tests the HomeDir method
func TestHomeDir(t *testing.T) {
	require.NotEqual(t, HomeDir(), "")
}

// TestExpandDir tests the ExpandDir method
func TestExpandDir(t *testing.T) {
	testDirA := "~/.gossamer-test"

	homeDir := HomeDir()
	expandedDirA := ExpandDir(testDirA)

	require.NotEqual(t, testDirA, expandedDirA)
	require.Equal(t, strings.Contains(expandedDirA, homeDir), true)

	testDirB := NewTestBasePath(t, "test")
	defer RemoveTestDir(t)

	expandedDirB := ExpandDir(testDirB)

	require.Equal(t, testDirB, expandedDirB)
	require.Equal(t, strings.Contains(expandedDirB, homeDir), false)
}

// TestBasePath tests the BasePath method
func TestBasePath(t *testing.T) {
	testDir := NewTestBasePath(t, "test")
	defer RemoveTestDir(t)

	homeDir := HomeDir()
	basePath := BasePath(testDir)

	require.NotEqual(t, testDir, basePath)
	require.Equal(t, strings.Contains(basePath, homeDir), true)
}

// TestKeystoreDir tests the KeystoreDir method
func TestKeystoreDir(t *testing.T) {
	testDir := NewTestBasePath(t, "test")
	defer RemoveTestDir(t)

	homeDir := HomeDir()
	basePath := BasePath(testDir)

	keystoreDir, err := KeystoreDir(testDir)
	require.Nil(t, err)

	require.NotEqual(t, testDir, basePath)
	require.Equal(t, strings.Contains(keystoreDir, homeDir), true)
}
