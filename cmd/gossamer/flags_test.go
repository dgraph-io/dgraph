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
	"io/ioutil"
	"testing"

	"github.com/ChainSafe/gossamer/dot"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

// TestFixFlagOrder tests the FixFlagOrder method
func TestFixFlagOrder(t *testing.T) {
	testCfg, testConfig := newTestConfigWithFile(t)
	genFile := dot.NewTestGenesisRawFile(t, testCfg)

	defer utils.RemoveTestDir(t)

	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
	}{
		{
			"Test gossamer --config --genesis-raw --log --force",
			[]string{"config", "genesis-raw", "log", "force"},
			[]interface{}{testConfig.Name(), genFile.Name(), "trace", true},
		},
		{
			"Test gossamer --config --genesis-raw --force --log",
			[]string{"config", "genesis-raw", "force", "log"},
			[]interface{}{testConfig.Name(), genFile.Name(), true, "trace"},
		},
		{
			"Test gossamer --config --force --genesis-raw --log",
			[]string{"config", "force", "genesis-raw", "log"},
			[]interface{}{testConfig.Name(), true, genFile.Name(), "trace"},
		},
		{
			"Test gossamer --force --config --genesis-raw --log",
			[]string{"force", "config", "genesis-raw", "log"},
			[]interface{}{true, testConfig.Name(), genFile.Name(), "trace"},
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)

			updatedInitAction := FixFlagOrder(initAction)
			err = updatedInitAction(ctx)
			require.Nil(t, err)

			updatedExportAction := FixFlagOrder(exportAction)
			err = updatedExportAction(ctx)
			require.Nil(t, err)
		})
	}
}
