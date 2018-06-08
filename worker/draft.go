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
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
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
	// The key is hex encoded version of <raft_id_of_node><random_uint64>
	// This should make sure its not same across replicas.
	keys map[string]*proposalCtx
}

func uniqueKey() string {
	b := make([]byte, 16)
	copy(b[:8], groups().Node.raftIdBuffer)
	groups().Node.rand.Read(b[8:])
	return hex.EncodeToString(b)
}

func (p *proposals) Store(key string, pctx *proposalCtx) bool {
	p.Lock()
	defer p.Unlock()
	if _, has := p.keys[key]; has {
		return false
	}
	p.keys[key] = pctx
	return true
}

func (p *proposals) pctx(key string) *proposalCtx {
	p.RLock()
	defer p.RUnlock()
	return p.keys[key]
}

func (p *proposals) CtxAndTxn(key string) (context.Context, *posting.Txn) {
	p.RLock()
	defer p.RUnlock()
	pd, has := p.keys[key]
	x.AssertTrue(has)
	return pd.ctx, pd.txn
}

func (p *proposals) Done(key string, err error) {
	p.Lock()
	defer p.Unlock()
	pd, has := p.keys[key]
	if !has {
		return
	}
	x.AssertTrue(pd.index != 0)
	if err != nil {
		pd.err = err
	}
	delete(p.keys, key)
	pd.ch <- pd.err
}

type node struct {
	*conn.Node

	// Changed after init but not protected by SafeMutex
	requestCh chan linReadReq

	// Fields which are never changed after init.
	applyCh chan raftpb.Entry
	ctx     context.Context
	stop    chan struct{} // to send the stop signal to Run
	done    chan struct{} // to check whether node is running or not
	gid     uint32
	props   proposals

	canCampaign  bool
	rand         *rand.Rand
	raftIdBuffer []byte
}

func (n *node) WaitForMinProposal(ctx context.Context, read *api.LinRead) error {
	if read == nil {
		return nil
	}
	if read.Sequencing == api.LinRead_SERVER_SIDE {
		return n.WaitLinearizableRead(ctx)
	}
	if read.Ids == nil {
		return nil
	}
	gid := n.RaftContext.Group
	min := read.Ids[gid]
	return n.Applied.WaitForMark(ctx, min)
}

type lockedSource struct {
	lk  sync.Mutex
	src rand.Source
}

func (r *lockedSource) Int63() int64 {
	r.lk.Lock()
	defer r.lk.Unlock()
	return r.src.Int63()
}

func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	defer r.lk.Unlock()
	r.src.Seed(seed)
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
		keys: make(map[string]*proposalCtx),
	}

	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, id)

	n := &node{
		Node:      m,
		requestCh: make(chan linReadReq),
		ctx:       context.Background(),
		gid:       gid,
		// processConfChange etc are not throttled so some extra delta, so that we don't
		// block tick when applyCh is full
		applyCh:      make(chan raftpb.Entry, Config.NumPendingProposals+1000),
		props:        props,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		rand:         rand.New(&lockedSource{src: rand.NewSource(time.Now().UnixNano())}),
		raftIdBuffer: b,
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

	che := make(chan error, 1)
	pctx := &proposalCtx{
		ch:  che,
		ctx: ctx,
	}

	key := uniqueKey()
	x.AssertTruef(n.props.Store(key, pctx), "Found existing proposal with key: [%v]", key)
	proposal.Key = key

	sz := proposal.Size()
	slice := make([]byte, sz)

	upto, err := proposal.MarshalTo(slice)
	if err != nil {
		return err
	}

	// Some proposals can be stuck if leader change happens. For e.g. MsgProp message from follower
	// to leader can be dropped/end up appearing with empty Data in CommittedEntries.
	// Having a timeout here prevents the mutation being stuck forever in case they don't have a
	// timeout.
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err = n.Raft().Propose(cctx, slice[:upto]); err != nil {
		return x.Wrapf(err, "While proposing")
	}

	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Waiting for the proposal.")
	}

	select {
	case err = <-che:
		if err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Raft Propose error: %v", err)
			}
		}
	case <-cctx.Done():
		return fmt.Errorf("While proposing to Raft group, err: %+v\n", cctx.Err())
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
	posting.Oracle().WaitForTs(context.Background(), txn.StartTs)
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
		return n.applyMutations(proposal, index)
	}

	defer posting.TxnMarks().Done(index)
	if len(proposal.Kv) > 0 {
		return n.processKeyValues(proposal.Key, proposal.Kv)

	} else if proposal.State != nil {
		// This state needn't be snapshotted in this group, on restart we would fetch
		// a state which is latest or equal to this.
		groups().applyState(proposal.State)
		return nil

	} else if len(proposal.CleanPredicate) > 0 {
		return n.deletePredicate(proposal.Key, proposal.CleanPredicate)

	} else if proposal.TxnContext != nil {
		return n.commitOrAbort(proposal.Key, proposal.TxnContext)
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
		n.props.Done(proposal.Key, err)
		n.Applied.Done(e.Index)
	}
}

func (n *node) commitOrAbort(pkey string, tctx *api.TxnContext) error {
	ctx, _ := n.props.CtxAndTxn(pkey)
	_, err := commitOrAbort(ctx, tctx)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Status of commitOrAbort %+v %v\n", tctx, err)
	}
	if err == nil {
		posting.Txns().Done(tctx.StartTs)
		posting.Oracle().Done(tctx.StartTs)
	}
	return err
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

type linReadReq struct {
	// A one-shot chan which we send a raft index upon
	indexCh chan<- uint64
}

func (n *node) readIndex(ctx context.Context) (chan uint64, error) {
	ch := make(chan uint64, 1)
	select {
	case n.requestCh <- linReadReq{ch}:
		return ch, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (n *node) runReadIndexLoop(closer *y.Closer, readStateCh <-chan raft.ReadState) {
	defer closer.Done()
	requests := []linReadReq{}
	// We maintain one linearizable ReadIndex request at a time.  Others wait queued behind
	// requestCh.
	for {
		select {
		case <-closer.HasBeenClosed():
			return
		case <-readStateCh:
			// Do nothing, discard ReadState as we don't have any pending ReadIndex requests.
		case req := <-n.requestCh:
		slurpLoop:
			for {
				requests = append(requests, req)
				select {
				case req = <-n.requestCh:
				default:
					break slurpLoop
				}
			}
			activeRctx := make([]byte, 8)
			x.Check2(n.rand.Read(activeRctx[:]))
			// To see if the ReadIndex request succeeds, we need to use a timeout and wait for a
			// successful response.  If we don't see one, the raft leader wasn't configured, or the
			// raft leader didn't respond.

			// This is supposed to use context.Background().  We don't want to cancel the timer
			// externally.  We want equivalent functionality to time.NewTimer.
			// TODO: Second is high, if a node gets partitioned we would have to throw error sooner.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := n.Raft().ReadIndex(ctx, activeRctx[:])
			if err != nil {
				for _, req := range requests {
					req.indexCh <- raft.None
				}
				continue
			}
		again:
			select {
			case <-closer.HasBeenClosed():
				cancel()
				return
			case rs := <-readStateCh:
				if 0 != bytes.Compare(activeRctx[:], rs.RequestCtx) {
					goto again
				}
				cancel()
				index := rs.Index
				for _, req := range requests {
					req.indexCh <- index
				}
			case <-ctx.Done():
				for _, req := range requests {
					req.indexCh <- raft.None
				}
			}
			requests = requests[:0]
		}
	}
}

func (n *node) Run() {
	firstRun := true
	var leader bool
	// See also our configuration of HeartbeatTick and ElectionTick.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	rcBytes, err := n.RaftContext.Marshal()
	x.Check(err)

	// Ensure we don't exit unless any snapshot in progress in done.
	closer := y.NewCloser(2)
	go n.snapshotPeriodically(closer)
	// This chan could have capacity zero, because runReadIndexLoop never blocks without selecting
	// on readStateCh.  It's 2 so that sending rarely blocks (so the Go runtime doesn't have to
	// switch threads as much.)
	readStateCh := make(chan raft.ReadState, 2)

	// We only stop runReadIndexLoop after the for loop below has finished interacting with it.
	// That way we know sending to readStateCh will not deadlock.
	go n.runReadIndexLoop(closer, readStateCh)

	elog := trace.NewEventLog("Dgraph", "RunLoop")
	defer elog.Finish()

	var timer x.Timer
	for {
		select {
		case <-ticker.C:
			n.Raft().Tick()

		case rd := <-n.Raft().Ready():
			timer.Start()
			for _, rs := range rd.ReadStates {
				readStateCh <- rs
			}

			if rd.SoftState != nil {
				groups().triggerMembershipSync()
				leader = rd.RaftState == raft.StateLeader
			}
			if leader {
				// Leader can send messages in parallel with writing to disk.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					msg.Context = rcBytes
					n.Send(msg)
				}
			}
			timer.Record() // Index 0.

			// Store the hardstate and entries.
			n.SaveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			timer.Record() // Index 1.

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
			timer.Record() // Index 2.

			lc := len(rd.CommittedEntries)
			if lc > 0 {
				if tr, ok := trace.FromContext(n.ctx); ok {
					tr.LazyPrintf("Found %d committed entries", len(rd.CommittedEntries))
				}
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
					n.Applied.Done(entry.Index)

				} else {
					// When applyCh fills up, this would automatically block.
					n.applyCh <- entry
				}

				// Move to debug log later.
				// Sometimes after restart there are too many entries to replay, so log so that we
				// know Run loop is replaying them.
				if lc > 1e5 && idx%5000 == 0 {
					x.Printf("In run loop applying committed entries, idx: [%v], pending: [%v]\n",
						idx, lc-idx)
				}
			}
			timer.Record() // Index 3.

			if lc > 1e5 {
				x.Println("All committed entries sent to applyCh.")
			}

			if !leader {
				// Followers should send messages later.
				for _, msg := range rd.Messages {
					// NOTE: We can do some optimizations here to drop messages.
					msg.Context = rcBytes
					n.Send(msg)
				}
			}
			n.Raft().Advance()
			if firstRun && n.canCampaign {
				go n.Raft().Campaign(n.ctx)
				firstRun = false
			}

			timer.Record() // Index 4.
			if total := timer.Total(); total >= 100*time.Millisecond {
				elog.Printf("Timer Total: %v. All: %+v.", total, timer.All())
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
				n.Connect(id, members[id].Addr)
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

var (
	errReadIndex = x.Errorf("cannot get linerized read (time expired or no configured leader)")
)

func (n *node) WaitLinearizableRead(ctx context.Context) error {
	replyCh, err := n.readIndex(ctx)
	if err != nil {
		return err
	}
	select {
	case index := <-replyCh:
		if index == raft.None {
			return errReadIndex
		}
		if err := n.Applied.WaitForMark(ctx, index); err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	return r.Status().Lead == r.Status().ID
}
