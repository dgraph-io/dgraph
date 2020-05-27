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

package main

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/utils"
)

// TestAccountGenerate test "gossamer account --generate"
func TestAccountGenerate(t *testing.T) {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		"Test gossamer account --generate",
		[]string{"basepath", "generate"},
		[]interface{}{testDir, "true"},
	)
	if err != nil {
		t.Fatal(err)
	}

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: check contents of data directory - improve cmd account tests
}

// TestAccountGeneratePassword test "gossamer account --generate --password"
func TestAccountGeneratePassword(t *testing.T) {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		"Test gossamer account --generate --password",
		[]string{"basepath", "generate", "password"},
		[]interface{}{testDir, "true", "1234"},
	)
	if err != nil {
		t.Fatal(err)
	}

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: check contents of data directory - improve cmd account tests
}

// TestAccountGenerateType test "gossamer account --generate --type"
func TestAccountGenerateType(t *testing.T) {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		"Test gossamer account --generate --type",
		[]string{"basepath", "generate", "type"},
		[]interface{}{testDir, "true", "ed25519"},
	)
	if err != nil {
		t.Fatal(err)
	}

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: check contents of data directory - improve cmd account tests
}

// TestAccountImport test "gossamer account --import"
func TestAccountImport(t *testing.T) {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		"Test gossamer account --import",
		[]string{"basepath", "import"},
		[]interface{}{testDir, "testfile"},
	)
	if err != nil {
		t.Fatal(err)
	}

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: check contents of data directory - improve cmd account tests
}

// TestAccountList test "gossamer account --list"
func TestAccountList(t *testing.T) {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	ctx, err := newTestContext(
		"Test gossamer account --list",
		[]string{"basepath", "list"},
		[]interface{}{testDir, "true"},
	)
	if err != nil {
		t.Fatal(err)
	}

	command := accountCommand
	err = command.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// TODO: check contents of data directory - improve cmd account tests
}
