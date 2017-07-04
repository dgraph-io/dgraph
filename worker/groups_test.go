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
package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGroupsSimple(t *testing.T) {
	groups, err := getGroupIds("1,2,3,5")
	require.NoError(t, err)
	require.Equal(t, []uint32{1, 2, 3, 5}, groups)
}

func TestParseGroupsError1(t *testing.T) {
	_, err := getGroupIds("1,2,-3,5")
	require.Error(t, err)
}

func TestParseGroupsError2(t *testing.T) {
	_, err := getGroupIds("1,2,-3,5")
	require.Error(t, err)
}

func TestParseGroupsError3(t *testing.T) {
	_, err := getGroupIds("1,wombat,3")
	require.Error(t, err)
}

func TestParseGroupsError4(t *testing.T) {
	_, err := getGroupIds("1,,3")
	require.Error(t, err)
}

func TestParseGroupsError5(t *testing.T) {
	_, err := getGroupIds("1, 2,3")
	require.Error(t, err)
}

func TestParseGroupsRange1(t *testing.T) {
	groups, err := getGroupIds("1-5")
	require.NoError(t, err)
	require.Equal(t, []uint32{1, 2, 3, 4, 5}, groups)
}

func TestParseGroupsRange2(t *testing.T) {
	groups, err := getGroupIds("1-3,5-7")
	require.NoError(t, err)
	require.Equal(t, []uint32{1, 2, 3, 5, 6, 7}, groups)
}

func TestParseGroupsRange3(t *testing.T) {
	groups, err := getGroupIds("1-3,5,7-8,123")
	require.NoError(t, err)
	require.Equal(t, []uint32{1, 2, 3, 5, 7, 8, 123}, groups)
}

func TestParseGroupsRange4(t *testing.T) {
	groups, err := getGroupIds("3-3,5,7-3,123")
	require.NoError(t, err)
	require.Equal(t, []uint32{3, 5, 123}, groups)
}

func TestParseGroupsError6(t *testing.T) {
	_, err := getGroupIds("1--2")
	require.Error(t, err)
}

func TestParseGroupsError7(t *testing.T) {
	_, err := getGroupIds("1--")
	require.Error(t, err)
}

func TestParseGroupsError8(t *testing.T) {
	_, err := getGroupIds("1-5-")
	require.Error(t, err)
}

func TestParseGroupsError9(t *testing.T) {
	_, err := getGroupIds("1-")
	require.Error(t, err)

	_, err = getGroupIds("-1")
	require.Error(t, err)
}

func TestParseGroupsError10(t *testing.T) {
	// duplicated id is an error
	_, err := getGroupIds("1-5,3")
	require.Error(t, err)
}

func TestParseGroupsError11(t *testing.T) {
	// duplicated id is an error
	_, err := getGroupIds("1-5,4-7")
	require.Error(t, err)
}

func TestParseGroupsError12(t *testing.T) {
	// duplicated id is an error
	_, err := getGroupIds("1,2,2,3")
	require.Error(t, err)
}
