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

package posting

import (
	"context"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var o *oracle

func Oracle() *oracle {
	return o
}

func init() {
	o = new(oracle)
	o.init()
}

type oracle struct {
	x.SafeMutex
	commits    map[uint64]uint64
	aborts     map[uint64]struct{}
	maxpending uint64

	// Used for waiting logic.
	waiters map[uint64][]chan struct{}
}

func (o *oracle) init() {
	o.commits = make(map[uint64]uint64)
	o.aborts = make(map[uint64]struct{})
	o.waiters = make(map[uint64][]chan struct{})
}

func (o *oracle) Done(startTs uint64) {
	o.Lock()
	defer o.Unlock()
	delete(o.commits, startTs)
	delete(o.aborts, startTs)
}

func (o *oracle) commitTs(startTs uint64) uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.commits[startTs]
}

func (o *oracle) addToWaiters(startTs uint64) (chan struct{}, bool) {
	o.Lock()
	defer o.Unlock()
	if o.maxpending >= startTs {
		return nil, false
	}
	ch := make(chan struct{})
	o.waiters[startTs] = append(o.waiters[startTs], ch)
	return ch, true
}

func (o *oracle) MaxPending() uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.maxpending
}

func (o *oracle) CurrentState() *protos.OracleDelta {
	od := new(protos.OracleDelta)
	od.Commits = make(map[uint64]uint64)
	o.RLock()
	defer o.RUnlock()
	for startTs := range o.aborts {
		od.Aborts = append(od.Aborts, startTs)
	}
	for startTs, commitTs := range o.commits {
		od.Commits[startTs] = commitTs
	}
	return od
}

func (o *oracle) WaitForTs(ctx context.Context, startTs uint64) error {
	ch, ok := o.addToWaiters(startTs)
	if !ok {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (o *oracle) ProcessOracleDelta(od *protos.OracleDelta) {
	o.Lock()
	defer o.Unlock()
	for startTs, commitTs := range od.Commits {
		o.commits[startTs] = commitTs
	}
	for _, startTs := range od.Aborts {
		o.aborts[startTs] = struct{}{}
	}
	if od.MaxPending <= o.maxpending {
		return
	}
	for startTs, toNotify := range o.waiters {
		if startTs > od.MaxPending {
			continue
		}
		for _, ch := range toNotify {
			close(ch)
		}
		delete(o.waiters, startTs)
	}
	o.maxpending = od.MaxPending
}
