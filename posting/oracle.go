/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package posting

import (
	"context"

	"github.com/dgraph-io/dgraph/protos/intern"
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
	commits map[uint64]uint64   // startTs => commitTs map
	aborts  map[uint64]struct{} // key is startTs

	// We know for sure that transactions with startTs <= maxpending have either been
	// aborted/committed.
	maxpending uint64

	// Used for waiting logic for transactions with startTs > maxpending so that we don't read an
	// uncommitted transaction.
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

func (o *oracle) CommitTs(startTs uint64) uint64 {
	o.RLock()
	defer o.RUnlock()
	return o.commits[startTs]
}

func (o *oracle) Aborted(startTs uint64) bool {
	o.RLock()
	defer o.RUnlock()
	_, ok := o.aborts[startTs]
	return ok
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

func (o *oracle) SetMaxPending(maxPending uint64) {
	o.Lock()
	defer o.Unlock()
	o.maxpending = maxPending
}

func (o *oracle) CurrentState() *intern.OracleDelta {
	od := new(intern.OracleDelta)
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

func (o *oracle) ProcessOracleDelta(od *intern.OracleDelta) {
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
