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

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueType(t *testing.T) {
	require.Equal(t, ValueType(false, false, false), ValueUid)
	require.Equal(t, ValueType(false, false, true), ValueEmpty)
	require.Equal(t, ValueType(false, true, false), ValueUid)
	require.Equal(t, ValueType(false, true, true), ValueEmpty)
	require.Equal(t, ValueType(true, false, false), ValuePlain)
	require.Equal(t, ValueType(true, false, true), ValuePlain)
	require.Equal(t, ValueType(true, true, false), ValueLang)
	require.Equal(t, ValueType(true, true, true), ValueLang)
}
