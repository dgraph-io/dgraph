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

package gql

import (
	"testing"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/stretchr/testify/require"
)

func TestExpandVariables(t *testing.T) {
	n := protos.NQuad{SubjectVar: "A"}
	nq := NQuad{&n}
	// Object variable didn't give us any uids.
	edges, err := nq.ExpandVariables(nil, []uint64{1, 2, 3}, []uint64{})
	require.Equal(t, 0, len(edges))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Atleast one out of ObjectId/ObjectValue should be set")

	// Object value with subject var.
	n.ObjectValue = &protos.Value{&protos.Value_StrVal{"Abc"}}
	edges, err = nq.ExpandVariables(nil, []uint64{1, 2, 3}, []uint64{})
	require.Equal(t, 3, len(edges))
	require.NoError(t, err)

	n.ObjectValue = nil
	n.SubjectVar = ""
	n.ObjectVar = "A"
	// No subject and non-empty ObjectVar
	edges, err = nq.ExpandVariables(nil, []uint64{}, []uint64{1, 2, 3})
	require.Equal(t, 0, len(edges))
	require.NoError(t, err)

	n.SubjectVar = "A"
	// SubjectVar and ObjectVar
	edges, err = nq.ExpandVariables(nil, []uint64{1, 2, 3}, []uint64{4, 5, 6})
	require.Equal(t, 9, len(edges))
	require.NoError(t, err)

	// Subject and ObjectVar
	nq.Subject = "1"
	uidMap := map[string]uint64{}
	uidMap["1"] = 1
	edges, err = nq.ExpandVariables(uidMap, []uint64{}, []uint64{4, 5, 6})
	require.Equal(t, 3, len(edges))
	require.NoError(t, err)

	n.ObjectValue = &protos.Value{&protos.Value_StrVal{"Abc"}}
	edges, err = nq.ExpandVariables(uidMap, []uint64{4, 5, 6}, []uint64{})
	require.Equal(t, 3, len(edges))
	require.NoError(t, err)
	n.ObjectValue = nil

	nq.Subject = ""
	nq.ObjectId = "1"
	edges, err = nq.ExpandVariables(uidMap, []uint64{4, 5, 6}, []uint64{})
	require.Equal(t, 3, len(edges))
	require.NoError(t, err)

}
