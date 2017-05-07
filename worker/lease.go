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
	"context"

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

var (
	// to serialize access to acquire lease
	m x.SafeMutex
)

func assignNew(ctx context.Context, N uint64, group uint32) (uint64, error) {
	m.Lock()
	uid.LeaseManager().RLock()
	nextId, leasedId := uid.LeaseManager().Get()
	available := uid.LeaseManager().NumAvailable()
	uid.LeaseManager().RUnlock()

	if N == 0 {
		m.Unlock()
		return nextId, nil
	}
	if available < N {
		if err := proposeLease(ctx, nextId, leasedId+100000, group); err != nil {
			m.Unlock()
			return 0, err
		}
	}

	uid.LeaseManager().Lock()
	startId := uid.LeaseManager().AssignNew(N)
	nextId, leasedId = uid.LeaseManager().Get()
	uid.LeaseManager().Unlock()
	m.Unlock()

	// Persist next Id can be done in parallel, only downside being
	// on failure of proposal that range of uid's would be lost since we
	// assign concurrently
	if err := proposeLease(ctx, nextId, leasedId, group); err != nil {
		return 0, err
	}

	return startId, nil
}

func proposeLease(ctx context.Context, nextId uint64, leasedId uint64, group uint32) error {
	lease := &taskp.UIDLease{GroupId: group, NextId: nextId, LeasedId: leasedId}
	proposal := &taskp.Proposal{UidLease: lease}
	n := groups().Node(group)
	x.AssertTruef(n != nil, "Node doesn't serve group %d", group)
	return n.ProposeAndWait(ctx, proposal)
}
