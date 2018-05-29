/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"bytes"
	"errors"
	"sync"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type task struct {
	rid  uint64 // raft index corresponding to the task
	pid  string // proposal id corresponding to the task
	edge *intern.DirectedEdge
}

type scheduler struct {
	sync.Mutex
	n *node
}

func (s *scheduler) init(n *node) {
	s.n = n
}

func (s *scheduler) waitForConflictResolution(attr string) error {
	for i := 0; i < 10; i++ {
		tctxs := posting.Txns().Iterate(func(key []byte) bool {
			pk := x.Parse(key)
			return pk.Attr == attr
		})
		if len(tctxs) == 0 {
			return nil
		}
		tryAbortTransactions(tctxs)
	}
	return errors.New("Unable to abort transactions")
}

func updateTxns(raftIndex uint64, startTs uint64) *posting.Txn {
	txn := &posting.Txn{
		StartTs: startTs,
		Indices: []uint64{raftIndex},
	}
	return posting.Txns().PutOrMergeIndex(txn)
}

// We don't support schema mutations across nodes in a transaction.
// Wait for all transactions to either abort or complete and all write transactions
// involving the predicate are aborted until schema mutations are done.

// 1 watermark would be done in the defer call. Rest n(number of edges) would be done when
// processTasks calls processMutation. When all are done, then we would send back error on
// proposal channel and finally mutation would return to the user. This ensures they are
// applied to memory before we return.
func (s *scheduler) schedule(proposal *intern.Proposal, index uint64) (err error) {
	defer func() {
		s.n.props.Done(proposal.Key, err)
	}()

	if proposal.Mutations.DropAll {
		// Ensures nothing get written to disk due to commit proposals.
		posting.Txns().Reset()
		if err = s.n.Applied.WaitForMark(s.n.ctx, index-1); err != nil {
			posting.TxnMarks().Done(index)
			return err
		}
		schema.State().DeleteAll()
		err = posting.DeleteAll()
		posting.TxnMarks().Done(index)
		return err
	}

	if proposal.Mutations.StartTs == 0 {
		posting.TxnMarks().Done(index)
		return errors.New("StartTs must be provided.")
	}

	startTs := proposal.Mutations.StartTs
	if len(proposal.Mutations.Schema) > 0 {
		if err = s.n.Applied.WaitForMark(s.n.ctx, index-1); err != nil {
			posting.TxnMarks().Done(index)
			return err
		}
		for _, supdate := range proposal.Mutations.Schema {
			// This is neceassry to ensure that there is no race between when we start reading
			// from badger and new mutation getting commited via raft and getting applied.
			// Before Moving the predicate we would flush all and wait for watermark to catch up
			// but there might be some proposals which got proposed but not comitted yet.
			// It's ok to reject the proposal here and same would happen on all nodes because we
			// would have proposed membershipstate, and all nodes would have the proposed state
			// or some state after that before reaching here.
			if tablet := groups().Tablet(supdate.Predicate); tablet != nil && tablet.ReadOnly {
				err = errPredicateMoving
				break
			}
			s.waitForConflictResolution(supdate.Predicate)
			err = s.n.processSchemaMutations(proposal.Key, index, startTs, supdate)
			if err != nil {
				break
			}
		}
		posting.TxnMarks().Done(index)
		return
	}

	// Scheduler tracks tasks at subject, predicate level, so doing
	// schema stuff here simplies the design and we needn't worry about
	// serializing the mutations per predicate or schema mutations
	// We derive the schema here if it's not present
	// Since raft committed logs are serialized, we can derive
	// schema here without any locking

	// stores a map of predicate and type of first mutation for each predicate
	schemaMap := make(map[string]types.TypeID)
	for _, edge := range proposal.Mutations.Edges {
		if tablet := groups().Tablet(edge.Attr); tablet != nil && tablet.ReadOnly {
			updateTxns(index, proposal.Mutations.StartTs)
			return errPredicateMoving
		}
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			// We should only have one edge drop in one mutation call.
			ctx, _ := s.n.props.CtxAndTxn(proposal.Key)
			if err = s.n.Applied.WaitForMark(ctx, index-1); err != nil {
				posting.TxnMarks().Done(index)
				return
			}
			defer posting.TxnMarks().Done(index)
			if err = s.waitForConflictResolution(edge.Attr); err != nil {
				return err
			}
			err = posting.DeletePredicate(ctx, edge.Attr)
			return
		}
		// Dont derive schema when doing deletion.
		if edge.Op == intern.DirectedEdge_DEL {
			continue
		}
		if _, ok := schemaMap[edge.Attr]; !ok {
			schemaMap[edge.Attr] = posting.TypeID(edge)
		}
	}

	total := len(proposal.Mutations.Edges)
	s.n.props.IncRef(proposal.Key, total)
	x.ActiveMutations.Add(int64(total))
	for attr, storageType := range schemaMap {
		if _, err := schema.State().TypeOf(attr); err != nil {
			// Schema doesn't exist
			// Since committed entries are serialized, updateSchemaIfMissing is not
			// needed, In future if schema needs to be changed, it would flow through
			// raft so there won't be race conditions between read and update schema
			updateSchemaType(attr, storageType, index)
		}
	}

	m := proposal.Mutations
	pctx := s.n.props.pctx(proposal.Key)
	pctx.txn = updateTxns(index, m.StartTs)
	var t task
	for _, edge := range m.Edges {
		t = task{
			rid:  index,
			pid:  proposal.Key,
			edge: edge,
		}
		err = posting.ErrRetry
		for err == posting.ErrRetry {
			err = s.n.processMutation(&t)
		}
		s.n.props.Done(t.pid, err)
		x.ActiveMutations.Add(-1)
	}
	err = nil
	return
}
