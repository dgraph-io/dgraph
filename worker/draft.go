/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgo/protos/api"
	dy "github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type proposalCtx struct {
	ch  chan error
	ctx context.Context
	// Since each proposal consists of multiple tasks we need to store
	// non-nil error returned by task
	err   error
	index uint64 // RAFT index for the proposal.
	// Used for writing all deltas at end
	txn *posting.Txn
}

type proposals struct {
	sync.RWMutex
	all map[string]*proposalCtx
}

// uniqueKey is meant to be unique across all the replicas.
func uniqueKey() string {
	return fmt.Sprintf("%02d-%d", groups().Node.Id, groups().Node.Rand.Uint64())
}

func (p *proposals) Store(key string, pctx *proposalCtx) bool {
	p.Lock()
	defer p.Unlock()
	if _, has := p.all[key]; has {
		return false
	}
	p.all[key] = pctx
	return true
}

func (p *proposals) Delete(key string) {
	if len(key) == 0 {
		return
	}
	p.Lock()
	defer p.Unlock()
	delete(p.all, key)
}

func (p *proposals) pctx(key string) *proposalCtx {
	p.RLock()
	defer p.RUnlock()
	if pctx := p.all[key]; pctx != nil {
		return pctx
	}
	return new(proposalCtx)
}

func (p *proposals) CtxAndTxn(key string) (context.Context, *posting.Txn) {
	p.RLock()
	defer p.RUnlock()
	pd, has := p.all[key]
	if !has {
		// See the race condition note in Done.
		return context.Background(), new(posting.Txn)
	}
	return pd.ctx, pd.txn
}

func (p *proposals) Done(key string, err error) {
	if len(key) == 0 {
		return
	}
	p.Lock()
	defer p.Unlock()
	pd, has := p.all[key]
	if !has {
		// If we assert here, there would be a race condition between a context
		// timing out, and a proposal getting applied immediately after. That
		// would cause assert to fail. So, don't assert.
		return
	}
	x.AssertTrue(pd.index != 0)
	if err != nil {
		pd.err = err
	}
	delete(p.all, key)
	pd.ch <- pd.err
}

type node struct {
	*conn.Node

	// Fields which are never changed after init.
	applyCh chan raftpb.Entry
	ctx     context.Context
	stop    chan struct{} // to send the stop signal to Run
	done    chan struct{} // to check whether node is running or not
	gid     uint32
	props   proposals

	canCampaign bool
	elog        trace.EventLog
}

func (n *node) WaitForMinProposal(ctx context.Context, read *api.LinRead) error {
	if read == nil {
		return nil
	}
	// TODO: Now that we apply txn updates via Raft, waiting based on Txn timestamps is sufficient.
	// It ensures that we have seen all applied mutations before a txn commit proposal is applied.
	// if read.Sequencing == api.LinRead_SERVER_SIDE {
	// 	return n.WaitLinearizableRead(ctx)
	// }
	if read.Ids == nil {
		return nil
	}
	gid := n.RaftContext.Group
	min := read.Ids[gid]
	return n.Applied.WaitForMark(ctx, min)
}

func newNode(store *raftwal.DiskStorage, gid uint32, id uint64, myAddr string) *node {
	x.Printf("Node ID: %v with GroupID: %v\n", id, gid)

	rc := &intern.RaftContext{
		Addr:  myAddr,
		Group: gid,
		Id:    id,
	}
	m := conn.NewNode(rc, store)
	props := proposals{
		all: make(map[string]*proposalCtx),
	}

	n := &node{
		Node: m,
		ctx:  context.Background(),
		gid:  gid,
		// processConfChange etc are not throttled so some extra delta, so that we don't
		// block tick when applyCh is full
		applyCh: make(chan raftpb.Entry, Config.NumPendingProposals+1000),
		props:   props,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		elog:    trace.NewEventLog("Dgraph", "ApplyCh"),
	}
	return n
}

type header struct {
	proposalId uint32
	msgId      uint16
}

func (h *header) Length() int {
	return 6 // 4 bytes for proposalId, 2 bytes for msgId.
}

func (h *header) Encode() []byte {
	result := make([]byte, h.Length())
	binary.LittleEndian.PutUint32(result[0:4], h.proposalId)
	binary.LittleEndian.PutUint16(result[4:6], h.msgId)
	return result
}

func (h *header) Decode(in []byte) {
	h.proposalId = binary.LittleEndian.Uint32(in[0:4])
	h.msgId = binary.LittleEndian.Uint16(in[4:6])
}

var errInternalRetry = errors.New("Retry Raft proposal internally")

// proposeAndWait sends a proposal through RAFT. It waits on a channel for the proposal
// to be applied(written to WAL) to all the nodes in the group.
func (n *node) proposeAndWait(ctx context.Context, proposal *intern.Proposal) error {
	if n.Raft() == nil {
		return x.Errorf("Raft isn't initialized yet")
	}
	// TODO: Should be based on number of edges (amount of work)
	pendingProposals <- struct{}{}
	x.PendingProposals.Add(1)
	defer func() { <-pendingProposals; x.PendingProposals.Add(-1) }()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Do a type check here if schema is present
	// In very rare cases invalid entries might pass through raft, which would
	// be persisted, we do best effort schema check while writing
	if proposal.Mutations != nil {
		for _, edge := range proposal.Mutations.Edges {
			if tablet := groups().Tablet(edge.Attr); tablet != nil && tablet.ReadOnly {
				return errPredicateMoving
			} else if tablet.GroupId != groups().groupId() {
				// Tablet can move by the time request reaches here.
				return errUnservedTablet
			}

			su, ok := schema.State().Get(edge.Attr)
			if !ok {
				continue
			} else if err := ValidateAndConvert(edge, &su); err != nil {
				return err
			}
		}
		for _, schema := range proposal.Mutations.Schema {
			if tablet := groups().Tablet(schema.Predicate); tablet != nil && tablet.ReadOnly {
				return errPredicateMoving
			}
			if err := checkSchema(schema); err != nil {
				return err
			}
		}
	}

	propose := func() error {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		che := make(chan error, 1)
		pctx := &proposalCtx{
			ch:  che,
			ctx: cctx,
		}
		key := uniqueKey()
		x.AssertTruef(n.props.Store(key, pctx), "Found existing proposal with key: [%v]", key)
		defer n.props.Delete(key) // Ensure that it gets deleted on return.
		proposal.Key = key

		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Proposing data with key: %s", key)
		}

		data, err := proposal.Marshal()
		if err != nil {
			return err
		}

		if err = n.Raft().Propose(cctx, data); err != nil {
			return x.Wrapf(err, "While proposing")
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Waiting for the proposal.")
		}

		select {
		case err = <-che:
			// We arrived here by a call to n.props.Done().
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Done with error: %v", err)
			}
			return err
		case <-ctx.Done():
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("External context timed out with error: %v.", ctx.Err())
			}
			return ctx.Err()
		case <-cctx.Done():
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Internal context timed out with error: %v. Retrying...", cctx.Err())
			}
			return errInternalRetry
		}
	}

	// Some proposals can be stuck if leader change happens. For e.g. MsgProp message from follower
	// to leader can be dropped/end up appearing with empty Data in CommittedEntries.
	// Having a timeout here prevents the mutation being stuck forever in case they don't have a
	// timeout. We should always try with a timeout and optionally retry.
	err := errInternalRetry
	for err == errInternalRetry {
		err = propose()
	}
	return err
}

func (n *node) processEdge(ridx uint64, pkey string, edge *intern.DirectedEdge) error {
	ctx, txn := n.props.CtxAndTxn(pkey)
	if txn.ShouldAbort() {
		return dy.ErrConflict
	}
	rv := x.RaftValue{Group: n.gid, Index: ridx}
	ctx = context.WithValue(ctx, "raft", rv)

	// Index updates would be wrong if we don't wait.
	// Say we do <0x1> <name> "janardhan", <0x1> <name> "pawan",
	// while applying the second mutation we check the old value
	// of name and delete it from "janardhan"'s index. If we don't
	// wait for commit information then mutation won't see the value

	// We used to Oracle().WaitForTs here.
	// TODO: Need to ensure that keys which hold values can only keep one pending txn at a
	// time. Otherwise, their index generated would be wrong.

	if err := runMutation(ctx, edge, txn); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("process mutation: %v", err)
		}
		return err
	}
	return nil
}

func (n *node) processSchemaMutations(pkey string, index uint64,
	startTs uint64, s *intern.SchemaUpdate) error {
	ctx, _ := n.props.CtxAndTxn(pkey)
	rv := x.RaftValue{Group: n.gid, Index: index}
	ctx = context.WithValue(ctx, "raft", rv)
	if err := runSchemaMutation(ctx, s, startTs); err != nil {
		if tr, ok := trace.FromContext(n.ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		return err
	}
	return nil
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

	if cc.Type == raftpb.ConfChangeRemoveNode {
		n.DeletePeer(cc.NodeID)
	} else if len(cc.Context) > 0 {
		var rc intern.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		n.Connect(rc.Id, rc.Addr)
	}

	cs := n.Raft().ApplyConfChange(cc)
	n.SetConfState(cs)
	n.DoneConfChange(cc.ID, nil)
}

func waitForConflictResolution(attr string) error {
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
func (n *node) applyMutations(proposal *intern.Proposal, index uint64) error {
	if proposal.Mutations.DropAll {
		// Ensures nothing get written to disk due to commit proposals.
		posting.Txns().Reset()
		schema.State().DeleteAll()
		err := posting.DeleteAll()
		posting.TxnMarks().Done(index)
		return err
	}

	if proposal.Mutations.StartTs == 0 {
		posting.TxnMarks().Done(index)
		return errors.New("StartTs must be provided.")
	}

	startTs := proposal.Mutations.StartTs
	if len(proposal.Mutations.Schema) > 0 {
		defer posting.TxnMarks().Done(index)
		for _, supdate := range proposal.Mutations.Schema {
			// This is neceassry to ensure that there is no race between when we start reading
			// from badger and new mutation getting commited via raft and getting applied.
			// Before Moving the predicate we would flush all and wait for watermark to catch up
			// but there might be some proposals which got proposed but not comitted yet.
			// It's ok to reject the proposal here and same would happen on all nodes because we
			// would have proposed membershipstate, and all nodes would have the proposed state
			// or some state after that before reaching here.
			if tablet := groups().Tablet(supdate.Predicate); tablet != nil && tablet.ReadOnly {
				return errPredicateMoving
			}
			if err := waitForConflictResolution(supdate.Predicate); err != nil {
				return err
			}
			if err := n.processSchemaMutations(proposal.Key, index, startTs, supdate); err != nil {
				return err
			}
		}
		return nil
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
			ctx, _ := n.props.CtxAndTxn(proposal.Key)
			defer posting.TxnMarks().Done(index)
			if err := waitForConflictResolution(edge.Attr); err != nil {
				return err
			}
			return posting.DeletePredicate(ctx, edge.Attr)
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
	pctx := n.props.pctx(proposal.Key)
	pctx.txn = updateTxns(index, m.StartTs)
	for _, edge := range m.Edges {
		err := posting.ErrRetry
		for err == posting.ErrRetry {
			err = n.processEdge(index, proposal.Key, edge)
		}
		if err != nil {
			return err
		}
		x.ActiveMutations.Add(-1)
	}
	return nil
}

func (n *node) applyCommitted(proposal *intern.Proposal, index uint64) error {
	if proposal.DeprecatedId != 0 {
		proposal.Key = fmt.Sprint(proposal.DeprecatedId)
	}

	// One final applied and synced watermark would be emitted when proposal ctx ref count
	// becomes zero.
	pctx := n.props.pctx(proposal.Key)
	if pctx == nil {
		// This is during replay of logs after restart or on a replica.
		pctx = &proposalCtx{
			ch:  make(chan error, 1),
			ctx: n.ctx,
		}
		// We assert here to make sure that we do add the proposal to the map.
		x.AssertTruef(n.props.Store(proposal.Key, pctx),
			"Found existing proposal with key: [%v]", proposal.Key)
	}
	pctx.index = index

	// TODO: We should be able to remove this as well.
	posting.TxnMarks().Begin(index)
	if proposal.Mutations != nil {
		// syncmarks for this shouldn't be marked done until it's comitted.
		n.elog.Printf("Applying mutations for key: %s", proposal.Key)
		return n.applyMutations(proposal, index)
	}

	defer posting.TxnMarks().Done(index)
	if len(proposal.Kv) > 0 {
		return n.processKeyValues(proposal.Key, proposal.Kv)

	} else if proposal.State != nil {
		n.elog.Printf("Applying state for key: %s", proposal.Key)
		// This state needn't be snapshotted in this group, on restart we would fetch
		// a state which is latest or equal to this.
		groups().applyState(proposal.State)
		return nil

	} else if len(proposal.CleanPredicate) > 0 {
		return n.deletePredicate(proposal.Key, proposal.CleanPredicate)

	} else if proposal.DeprecatedTxnContext != nil {
		n.elog.Printf("Applying txncontext for key: %s", proposal.Key)
		delta := &intern.OracleDelta{}
		tctx := proposal.DeprecatedTxnContext
		if tctx.CommitTs == 0 {
			delta.Aborts = append(delta.Aborts, tctx.StartTs)
		} else {
			delta.Commits = make(map[uint64]uint64)
			delta.Commits[tctx.StartTs] = tctx.CommitTs
		}
		return n.commitOrAbort(proposal.Key, delta)
	} else if proposal.Delta != nil {
		n.elog.Printf("Applying Oracle Delta for key: %s", proposal.Key)
		return n.commitOrAbort(proposal.Key, proposal.Delta)
	} else {
		x.Fatalf("Unknown proposal")
	}
	return nil
}

func (n *node) processApplyCh() {
	for e := range n.applyCh {
		proposal := &intern.Proposal{}
		if err := proposal.Unmarshal(e.Data); err != nil {
			x.Fatalf("Unable to unmarshal proposal: %v %q\n", err, e.Data)
		}
		err := n.applyCommitted(proposal, e.Index)
		n.elog.Printf("Applied proposal with key: %s, index: %d. Err: %v", proposal.Key, e.Index, err)
		n.props.Done(proposal.Key, err)
		n.Applied.Done(e.Index)
	}
}

func (n *node) commitOrAbort(pkey string, delta *intern.OracleDelta) error {
	ctx, _ := n.props.CtxAndTxn(pkey)

	applyTxnStatus := func(startTs, commitTs uint64) {
		var err error
		for i := 0; i < 3; i++ {
			err = commitOrAbort(ctx, startTs, commitTs)
			if err == nil || err == posting.ErrInvalidTxn {
				break
			}
			x.Printf("Error while applying txn status (%d -> %d): %v", startTs, commitTs, err)
		}
		// TODO: Even after multiple tries, if we're unable to apply the status of a transaction,
		// what should we do? Maybe do a printf, and let them know that there might be a disk issue.
		posting.Txns().Done(startTs)
		posting.Oracle().Done(startTs)
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Status of commitOrAbort startTs %d: %v\n", startTs, err)
		}
	}

	for startTs, commitTs := range delta.GetCommits() {
		applyTxnStatus(startTs, commitTs)
	}
	for _, startTs := range delta.GetAborts() {
		applyTxnStatus(startTs, 0)
	}
	// TODO: Use MaxPending to track the txn watermark. That's the only thing we need really.
	// delta.GetMaxPending
	posting.Oracle().ProcessOracleDelta(delta)
	return nil
}

func (n *node) deletePredicate(pkey string, predicate string) error {
	ctx, _ := n.props.CtxAndTxn(pkey)
	return posting.DeletePredicate(ctx, predicate)
}

func (n *node) processKeyValues(pkey string, kvs []*intern.KV) error {
	ctx, _ := n.props.CtxAndTxn(pkey)
	return populateKeyValues(ctx, kvs)
}

func (n *node) applyAllMarks(ctx context.Context) {
	// Get index of last committed.
	lastIndex := n.Applied.LastIndex()
	n.Applied.WaitForMark(ctx, lastIndex)
}

func (n *node) leaderBlocking() (*conn.Pool, error) {
	pool := groups().Leader(groups().groupId())
	if pool == nil {
		// Functions like retrieveSnapshot and joinPeers are blocking at initial start and
		// leader election for a group might not have happened when it is called. If we can't
		// find a leader, get latest state from
		// Zero.
		if err := UpdateMembershipState(context.Background()); err != nil {
			return nil, fmt.Errorf("Error while trying to update membership state: %+v", err)
		}
		return nil, fmt.Errorf("Unable to reach leader in group %d", n.gid)
	}
	return pool, nil
}

func (n *node) retrieveSnapshot() error {
	pool, err := n.leaderBlocking()
	if err != nil {
		return err
	}

	// Wait for watermarks to sync since populateShard writes directly to db, otherwise
	// the values might get overwritten
	// Safe to keep this line
	n.applyAllMarks(n.ctx)

	// Need to clear pl's stored in memory for the case when retrieving snapshot with
	// index greater than this node's last index
	// Should invalidate/remove pl's to this group only ideally
	posting.EvictLRU()
	if _, err := n.populateShard(pstore, pool); err != nil {
		return fmt.Errorf("Cannot retrieve snapshot from peer, error: %v\n", err)
	}
	// Populate shard stores the streamed data directly into db, so we need to refresh
	// schema for current group id
	if err := schema.LoadFromDb(); err != nil {
		return fmt.Errorf("Error while initilizating schema: %+v\n", err)
	}
	groups().triggerMembershipSync()
	return nil
}

func (n *node) Run() {
	firstRun := true
	var leader bool
	// See also our configuration of HeartbeatTick and ElectionTick.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	// Ensure we don't exit unless any snapshot in progress in done.
	closer := y.NewCloser(2)
	go n.snapshotPeriodically(closer)

	logTicker := time.NewTicker(time.Minute)
	defer logTicker.Stop()

	for {
		select {
		case <-logTicker.C:
			n.elog.Printf("Size of applyCh: %d", len(n.applyCh))

		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			var tr trace.Trace
			if len(rd.Entries) > 0 || !raft.IsEmptySnap(rd.Snapshot) || !raft.IsEmptyHardState(rd.HardState) {
				// Optionally, trace this run.
				tr = trace.New("Dgraph", "RunLoop")
			}

			if rd.SoftState != nil {
				groups().triggerMembershipSync()
				leader = rd.RaftState == raft.StateLeader
			}
			if leader {
				// Leader can send messages in parallel with writing to disk.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					n.Send(msg)
				}
			}
			if tr != nil {
				tr.LazyPrintf("Handled ReadStates and SoftState.")
			}

			// Store the hardstate and entries. Note that these are not CommittedEntries.
			n.SaveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			if tr != nil {
				tr.LazyPrintf("Saved %d entries. Snapshot, HardState empty? (%v, %v)",
					len(rd.Entries),
					raft.IsEmptySnap(rd.Snapshot),
					raft.IsEmptyHardState(rd.HardState))
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				// We don't send snapshots to other nodes. But, if we get one, that means
				// either the leader is trying to bring us up to state; or this is the
				// snapshot that I created. Only the former case should be handled.
				var rc intern.RaftContext
				x.Check(rc.Unmarshal(rd.Snapshot.Data))
				x.AssertTrue(rc.Group == n.gid)
				if rc.Id != n.Id {
					// NOTE: Retrieving snapshot here is OK, after storing it above in WAL, because
					// rc.Id != n.Id.
					x.Printf("-------> SNAPSHOT [%d] from %d\n", n.gid, rc.Id)
					// It's ok to block tick while retrieving snapshot, since it's a follower
					n.retryUntilSuccess(n.retrieveSnapshot, 100*time.Millisecond)
					x.Printf("-------> SNAPSHOT [%d]. DONE.\n", n.gid)
				} else {
					x.Printf("-------> SNAPSHOT [%d] from %d [SELF]. Ignoring.\n", n.gid, rc.Id)
				}
			}
			if tr != nil {
				tr.LazyPrintf("Applied or retrieved snapshot.")
			}

			// Now schedule or apply committed entries.
			for idx, entry := range rd.CommittedEntries {
				// Need applied watermarks for schema mutation also for read linearazibility
				// Applied watermarks needs to be emitted as soon as possible sequentially.
				// If we emit Mark{4, false} and Mark{4, true} before emitting Mark{3, false}
				// then doneUntil would be set as 4 as soon as Mark{4,true} is done and before
				// Mark{3, false} is emitted. So it's safer to emit watermarks as soon as
				// possible sequentially
				n.Applied.Begin(entry.Index)

				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)
					// Not present in proposal map.
					n.Applied.Done(entry.Index)
					groups().triggerMembershipSync()

				} else if len(entry.Data) == 0 {
					// TODO: Say something. Do something.
					tr.LazyPrintf("Found empty data at index: %d", entry.Index)
					tr.SetError()
					n.Applied.Done(entry.Index)

				} else {
					// When applyCh fills up, this would automatically block.
					n.applyCh <- entry
				}

				// Move to debug log later.
				// Sometimes after restart there are too many entries to replay, so log so that we
				// know Run loop is replaying them.
				if tr != nil && idx%5000 == 4999 {
					tr.LazyPrintf("Handling committed entries. At idx: [%v]\n", idx)
				}
			}
			if tr != nil {
				tr.LazyPrintf("Handled %d committed entries.", len(rd.CommittedEntries))
			}

			if !leader {
				// Followers should send messages later.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					n.Send(msg)
				}
			}
			if tr != nil {
				tr.LazyPrintf("Follower queued messages.")
			}

			n.Raft().Advance()
			if firstRun && n.canCampaign {
				go n.Raft().Campaign(n.ctx)
				firstRun = false
			}
			if tr != nil {
				tr.LazyPrintf("Advanced Raft. Done.")
				tr.Finish()
			}

		case <-n.stop:
			if peerId, has := groups().MyPeer(); has && n.AmLeader() {
				n.Raft().TransferLeadership(n.ctx, Config.RaftId, peerId)
				go func() {
					select {
					case <-n.ctx.Done(): // time out
						if tr, ok := trace.FromContext(n.ctx); ok {
							tr.LazyPrintf("context timed out while transfering leadership")
						}
					case <-time.After(1 * time.Second):
						if tr, ok := trace.FromContext(n.ctx); ok {
							tr.LazyPrintf("Timed out transfering leadership")
						}
					}
					n.Raft().Stop()
					closer.SignalAndWait()
					close(n.done)
				}()
			} else {
				n.Raft().Stop()
				closer.SignalAndWait()
				close(n.done)
			}
		case <-n.done:
			return
		}
	}
}

func (n *node) Stop() {
	select {
	case n.stop <- struct{}{}:
	case <-n.done:
		// already stopped.
		return
	}
	<-n.done // wait for Run to respond.
}

func (n *node) snapshotPeriodically(closer *y.Closer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Some proposals like predicate move can consume around 32MB per proposal, so keeping
			// too many proposals would increase the memory usage so snapshot as soon as
			// possible
			n.snapshot(10)

		case <-closer.HasBeenClosed():
			closer.Done()
			return
		}
	}
}

func (n *node) abortOldTransactions(pending uint64) {
	pl := groups().Leader(0)
	if pl == nil {
		return
	}
	zc := intern.NewZeroClient(pl.Get())
	// Aborts if not already committed.
	startTimestamps := posting.Txns().TxnsSinceSnapshot(pending)
	req := &intern.TxnTimestamps{Ts: startTimestamps}
	zc.TryAbort(context.Background(), req)
}

func (n *node) snapshot(skip uint64) {
	txnWatermark := posting.TxnMarks().DoneUntil()
	existing, err := n.Store.Snapshot()
	x.Checkf(err, "Unable to get existing snapshot")

	lastSnapshotIdx := existing.Metadata.Index
	if txnWatermark <= lastSnapshotIdx+skip {
		appliedWatermark := n.Applied.DoneUntil()
		// If difference grows above 1.5 * ForceAbortDifference we try to abort old transactions
		if appliedWatermark-txnWatermark > 1.5*x.ForceAbortDifference && skip != 0 {
			// Print warning if difference grows above 3 * x.ForceAbortDifference. Shouldn't ideally
			// happen as we abort oldest 20% when it grows above 1.5 times.
			if appliedWatermark-txnWatermark > 3*x.ForceAbortDifference {
				x.Printf("Couldn't take snapshot, txn watermark: [%d], applied watermark: [%d]\n",
					txnWatermark, appliedWatermark)
			}
			// Try aborting pending transactions here.
			n.abortOldTransactions(appliedWatermark - txnWatermark)
		}
		return
	}

	snapshotIdx := txnWatermark - skip
	if tr, ok := trace.FromContext(n.ctx); ok {
		tr.LazyPrintf("Taking snapshot for group: %d at watermark: %d\n", n.gid, snapshotIdx)
	}

	rc, err := n.RaftContext.Marshal()
	x.Check(err)

	err = n.Store.CreateSnapshot(snapshotIdx, n.ConfState(), rc)
	x.Checkf(err, "While creating snapshot")
	x.Printf("Writing snapshot at index: %d, applied mark: %d\n", snapshotIdx,
		n.Applied.DoneUntil())
}

func (n *node) joinPeers() error {
	pl, err := n.leaderBlocking()
	if err != nil {
		return err
	}

	gconn := pl.Get()
	c := intern.NewRaftClient(gconn)
	x.Printf("Calling JoinCluster via leader: %s", pl.Addr)
	if _, err := c.JoinCluster(n.ctx, n.RaftContext); err != nil {
		return x.Errorf("Error while joining cluster: %+v\n", err)
	}
	x.Printf("Done with JoinCluster call\n")
	return nil
}

// Checks if its a peer from the leader of the group.
func (n *node) isMember() (bool, error) {
	pl, err := n.leaderBlocking()
	if err != nil {
		return false, err
	}

	gconn := pl.Get()
	c := intern.NewRaftClient(gconn)
	x.Printf("Calling IsPeer")
	pr, err := c.IsPeer(n.ctx, n.RaftContext)
	if err != nil {
		return false, x.Errorf("Error while joining cluster: %+v\n", err)
	}
	x.Printf("Done with IsPeer call\n")
	return pr.Status, nil
}

func (n *node) retryUntilSuccess(fn func() error, pause time.Duration) {
	var err error
	for {
		if err = fn(); err == nil {
			break
		}
		x.Printf("Error while calling fn: %v. Retrying...\n", err)
		time.Sleep(pause)
	}
}

// InitAndStartNode gets called after having at least one membership sync with the cluster.
func (n *node) InitAndStartNode() {
	idx, restart, err := n.PastLife()
	x.Check(err)
	n.Applied.SetDoneUntil(idx)
	posting.TxnMarks().SetDoneUntil(idx)

	if _, hasPeer := groups().MyPeer(); !restart && hasPeer {
		// The node has other peers, it might have crashed after joining the cluster and before
		// writing a snapshot. Check from leader, if it is part of the cluster. Consider this a
		// restart if it is part of the cluster, else start a new node.
		for {
			if restart, err = n.isMember(); err == nil {
				break
			}
			x.Printf("Error while calling hasPeer: %v. Retrying...\n", err)
			time.Sleep(time.Second)
		}
	}

	if restart {
		x.Printf("Restarting node for group: %d\n", n.gid)
		sp, err := n.Store.Snapshot()
		x.Checkf(err, "Unable to get existing snapshot")
		if !raft.IsEmptySnap(sp) {
			members := groups().members(n.gid)
			for _, id := range sp.Metadata.ConfState.Nodes {
				m, ok := members[id]
				if ok {
					n.Connect(id, m.Addr)
				}
			}
		}
		n.SetRaft(raft.RestartNode(n.Cfg))
	} else {
		x.Printf("New Node for group: %d\n", n.gid)
		if peerId, hasPeer := groups().MyPeer(); hasPeer {
			// Get snapshot before joining peers as it can take time to retrieve it and we dont
			// want the quorum to be inactive when it happens.

			x.Printf("Retrieving snapshot from peer: %d", peerId)
			n.retryUntilSuccess(n.retrieveSnapshot, time.Second)

			x.Println("Trying to join peers.")
			n.retryUntilSuccess(n.joinPeers, time.Second)
			n.SetRaft(raft.StartNode(n.Cfg, nil))
		} else {
			peers := []raft.Peer{{ID: n.Id}}
			n.SetRaft(raft.StartNode(n.Cfg, peers))
			// Trigger election, so this node can become the leader of this single-node cluster.
			n.canCampaign = true
		}
	}
	go n.processApplyCh()
	go n.Run()
	go n.BatchAndSendMessages()
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	return r.Status().Lead == r.Status().ID
}
