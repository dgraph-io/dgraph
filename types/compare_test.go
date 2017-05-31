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

package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArgs(t *testing.T) {
	val := "Alice\""
	args, err := Args(val)
	require.NoError(t, err)
	require.Equal(t, len(args), 1)
	require.Equal(t, args[0], "Alice\"")
}

func TestArgs1(t *testing.T) {
	val := `["Alice"", "Bob", "Charlie"""]`
	args, err := Args(val)
	require.NoError(t, err)
	require.Equal(t, len(args), 3)
	fmt.Println(args)
}
