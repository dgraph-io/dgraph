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
	"flag"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

// newTestContext creates a cli context for a test given a set of flags and values
func newTestContext(description string, flags []string, values []interface{}) (*cli.Context, error) {
	set := flag.NewFlagSet(description, 0)
	for i := range values {
		switch v := values[i].(type) {
		case bool:
			set.Bool(flags[i], v, "")
		case string:
			set.String(flags[i], v, "")
		case uint:
			set.Uint(flags[i], v, "")
		default:
			return nil, fmt.Errorf("unexpected cli value type: %T", values[i])
		}
	}
	ctx := cli.NewContext(nil, set, nil)
	return ctx, nil
}

// TestStartLogger
func TestStartLogger(t *testing.T) {
	testApp := cli.NewApp()
	testApp.Writer = ioutil.Discard

	testcases := []struct {
		description string
		flags       []string
		values      []interface{}
		expected    error
	}{
		{
			"Test gossamer --verbosity info",
			[]string{"verbosity"},
			[]interface{}{"info"},
			nil,
		},
		{
			"Test gossamer --verbosity debug",
			[]string{"verbosity"},
			[]interface{}{"debug"},
			nil,
		},
		{
			"Test gossamer --verbosity trace",
			[]string{"verbosity"},
			[]interface{}{"trace"},
			nil,
		},
		{
			"Test gossamer --verbosity blah",
			[]string{"verbosity"},
			[]interface{}{"blah"},
			fmt.Errorf("Unknown level: blah"),
		},
	}

	for _, c := range testcases {
		c := c // bypass scopelint false positive
		t.Run(c.description, func(t *testing.T) {
			ctx, err := newTestContext(c.description, c.flags, c.values)
			require.Nil(t, err)

			err = startLogger(ctx)
			require.Equal(t, c.expected, err)
		})
	}
}
