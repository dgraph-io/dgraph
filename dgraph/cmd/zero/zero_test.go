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
package zero

import (
	"context"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRemoveNode(t *testing.T) {
	server := &Server{
		state: &intern.MembershipState{
			Groups: map[uint32]*intern.Group{1: {Members: map[uint64]*intern.Member{}}},
		},
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	err := server.removeNode(nil, 3, 1)
	require.Error(t, err)
	err = server.removeNode(nil, 1, 2)
	require.Error(t, err)
}
