/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"reflect"
	"testing"
)

func TestTokens(t *testing.T) {
	for _, test := range []struct {
		input  string
		output []string
	}{
		{
			input: "55.21.81.100/32",
			output: []string{
				"55.21.81.100/32",
				"55.21.81.100/31",
				"55.21.81.100/30",
				"55.21.81.96/29",
				"55.21.81.96/28",
				"55.21.81.96/27",
				"55.21.81.64/26",
				"55.21.81.0/25",
				"55.21.81.0/24",
				"55.21.80.0/23",
				"55.21.80.0/22",
				"55.21.80.0/21",
				"55.21.80.0/20",
				"55.21.64.0/19",
				"55.21.64.0/18",
				"55.21.0.0/17",
				"55.21.0.0/16",
				"55.20.0.0/15",
				"55.20.0.0/14",
				"55.16.0.0/13",
				"55.16.0.0/12",
				"55.0.0.0/11",
				"55.0.0.0/10",
				"55.0.0.0/9",
				"55.0.0.0/8",
				"54.0.0.0/7",
				"52.0.0.0/6",
				"48.0.0.0/5",
				"48.0.0.0/4",
				"32.0.0.0/3",
				"0.0.0.0/2",
				"0.0.0.0/1",
			},
		},
		{
			input: "21.85.0.0/16",
			output: []string{
				"21.85.0.0/16",
				"21.84.0.0/15",
				"21.84.0.0/14",
				"21.80.0.0/13",
				"21.80.0.0/12",
				"21.64.0.0/11",
				"21.64.0.0/10",
				"21.0.0.0/9",
				"21.0.0.0/8",
				"20.0.0.0/7",
				"20.0.0.0/6",
				"16.0.0.0/5",
				"16.0.0.0/4",
				"0.0.0.0/3",
				"0.0.0.0/2",
				"0.0.0.0/1",
			},
		},
	} {
		got, err := Tokens(test.input)
		if err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(got, test.output) {
			t.Errorf("Got=%v Want=%v", got, test.output)
		}
	}
}
