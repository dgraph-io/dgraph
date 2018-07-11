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
	"math"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

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

// uniqueKey is meant to be unique across all the replicas.
func uniqueKey() string {
	return fmt.Sprintf("%02d-%d", groups().Node.Id, groups().Node.Rand.Uint64())
}

type node struct {
	*conn.Node

	// Fields which are never changed after init.
	applyCh chan raftpb.Entry
	ctx     context.Context
	stop    chan struct{} // to send the stop signal to Run
	done    chan struct{} // to check whether node is running or not
	gid     uint32

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

	n := &node{
		Node: m,
		ctx:  context.Background(),
		gid:  gid,
		// processConfChange etc are not throttled so some extra delta, so that we don't
		// block tick when applyCh is full
		applyCh: make(chan raftpb.Entry, Config.NumPendingProposals+1000),
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
		pctx := &conn.ProposalCtx{
			Ch:  che,
			Ctx: cctx,
		}
		key := uniqueKey()
		x.AssertTruef(n.Proposals.Store(key, pctx), "Found existing proposal with key: [%v]", key)
		defer n.Proposals.Delete(key) // Ensure that it gets deleted on return.
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
			// We arrived here by a call to n.Proposals.Done().
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

func (n *node) Ctx(key string) context.Context {
	ctx := context.Background()
	if pctx := n.Proposals.Get(key); pctx != nil {
		ctx = pctx.Ctx
	}
	return ctx
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
		tctxs := posting.Oracle().IterateTxns(func(key []byte) bool {
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

// We don't support schema mutations across nodes in a transaction.
// Wait for all transactions to either abort or complete and all write transactions
// involving the predicate are aborted until schema mutations are done.
func (n *node) applyMutations(proposal *intern.Proposal, index uint64) error {
	if proposal.Mutations.DropAll {
		// Ensures nothing get written to disk due to commit proposals.
		posting.Oracle().ResetTxns()
		schema.State().DeleteAll()
		return posting.DeleteAll()
	}

	if proposal.Mutations.StartTs == 0 {
		return errors.New("StartTs must be provided.")
	}

	startTs := proposal.Mutations.StartTs
	ctx := n.Ctx(proposal.Key)

	if len(proposal.Mutations.Schema) > 0 {
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
			if err := runSchemaMutation(ctx, supdate, startTs); err != nil {
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
			return errPredicateMoving
		}
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			// We should only have one edge drop in one mutation call.
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
	defer x.ActiveMutations.Add(-int64(total))

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
	txn := posting.Oracle().RegisterStartTs(m.StartTs)
	if txn.ShouldAbort() {
		return dy.ErrConflict
	}
	for _, edge := range m.Edges {
		err := posting.ErrRetry
		for err == posting.ErrRetry {
			err = runMutation(ctx, edge, txn)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *node) applyCommitted(proposal *intern.Proposal, index uint64) error {
	if proposal.Mutations != nil {
		// syncmarks for this shouldn't be marked done until it's comitted.
		n.elog.Printf("Applying mutations for key: %s", proposal.Key)
		return n.applyMutations(proposal, index)
	}

	ctx := n.Ctx(proposal.Key)
	if len(proposal.Kv) > 0 {
		return populateKeyValues(ctx, proposal.Kv)

	} else if proposal.State != nil {
		n.elog.Printf("Applying state for key: %s", proposal.Key)
		// This state needn't be snapshotted in this group, on restart we would fetch
		// a state which is latest or equal to this.
		groups().applyState(proposal.State)
		return nil

	} else if len(proposal.CleanPredicate) > 0 {
		n.elog.Printf("Cleaning predicate: %s", proposal.CleanPredicate)
		return posting.DeletePredicate(ctx, proposal.CleanPredicate)

	} else if proposal.Delta != nil {
		n.elog.Printf("Applying Oracle Delta for key: %s", proposal.Key)
		return n.commitOrAbort(proposal.Key, proposal.Delta)

	} else if proposal.Snapshot != nil {
		snap := proposal.Snapshot
		n.elog.Printf("Creating snapshot: %+v", snap)
		x.Printf("Creating snapshot at index: %d. MinPendingStartTs: %d.\n",
			snap.Index, snap.MinPendingStartTs)
		data, err := snap.Marshal()
		x.Check(err)
		if err := n.Store.CreateSnapshot(snap.Index, n.ConfState(), data); err != nil {
			return err
		}
		// Now roll up all posting lists.
		return rollupPostingLists(snap.MinPendingStartTs - 1)

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
		n.Proposals.Done(proposal.Key, err)
		n.Applied.Done(e.Index)
	}
}

func (n *node) commitOrAbort(pkey string, delta *intern.OracleDelta) error {
	ctx := n.Ctx(pkey)

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
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Status of commitOrAbort startTs %d: %v\n", startTs, err)
		}
	}

	for _, txn := range delta.Txns {
		applyTxnStatus(txn.StartTs, txn.CommitTs)
	}
	// TODO: Use MaxPending to track the txn watermark. That's the only thing we need really.
	// delta.GetMaxPending
	posting.Oracle().ProcessDelta(delta)
	return nil
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
	//
	// We can safely evict posting lists from memory. Because, all the updates corresponding to txn
	// commits up until then have already been written to pstore. And the way we take snapshots, we
	// keep all the pre-writes for a pending transaction, so they will come back to memory, as Raft
	// logs are replayed.
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

	slowTicker := time.NewTicker(time.Minute)
	defer slowTicker.Stop()

	for {
		select {
		case <-slowTicker.C:
			n.elog.Printf("Size of applyCh: %d", len(n.applyCh))
			if leader {
				// We use disk based storage for Raft. So, we're not too concerned about
				// snapshotting.  We just need to do enough, so that we don't have a huge backlog of
				// entries to process on a restart.
				if err := n.calculateSnapshot(1000); err != nil {
					x.Errorf("While taking snapshot: %v\n", err)
				}
				go n.abortOldTransactions()
			}

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
				var snap intern.Snapshot
				x.Check(snap.Unmarshal(rd.Snapshot.Data))
				rc := snap.GetContext()
				x.AssertTrue(rc.GetGroup() == n.gid)
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
					close(n.done)
				}()
			} else {
				n.Raft().Stop()
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

// abortOldTransactions would find txns which have done pre-writes, but have been pending for a
// while. The time that is used is based on the last pre-write seen, so if a txn is doing a
// pre-write multiple times, we'll pick the timestamp of the last pre-write. Thus, this function
// would only act on the txns which have not been active in the last N minutes, and send them for
// abort. Note that only the leader runs this function.
// NOTE: We might need to get the results of TryAbort and propose them. But, it's unclear if we need
// to, because Zero should stream out the aborts anyway.
func (n *node) abortOldTransactions() {
	pl := groups().Leader(0)
	if pl == nil {
		return
	}
	zc := intern.NewZeroClient(pl.Get())
	// Aborts if not already committed.
	startTimestamps := posting.Oracle().TxnOlderThan(5 * time.Minute)
	if len(startTimestamps) == 0 {
		return
	}
	req := &intern.TxnTimestamps{Ts: startTimestamps}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := zc.TryAbort(ctx, req)
	x.Printf("Aborted txns with start ts: %v. Error: %v\n", startTimestamps, err)
}

// calculateSnapshot would calculate a snapshot index, considering these factors:
// - We still keep at least keepN number of Raft entries. If we cut too short,
// then the chances that a crashed follower needs to retrieve the entire state
// from leader increases. So, we keep a buffer to allow a follower to catch up.
// - We can discard at least half of keepN number of entries.
// - We are not overshooting the max applied entry. That is, we're not removing
// Raft entries before they get applied.
// - We are considering the minimum start ts that has yet to be committed or
// aborted. This way, we still keep all the mutations corresponding to this
// start ts in the Raft logs. This is important, because we don't persist
// pre-writes to disk in pstore.
// - Finally, this function would propose this snapshot index, so the entire
// group can apply it to their Raft stores.
func (n *node) calculateSnapshot(keepN int) error {
	tr := trace.New("Dgraph.Internal", "Propose.Snapshot")
	defer tr.Finish()

	// Each member of the Raft group is taking snapshots independently, as mentioned in Section 7 of
	// Raft paper. We want to avoid taking snapshots too close to the LastIndex, so that in case the
	// leader changes, and the followers haven't yet caught up to this index, they would need to
	// retrieve the entire state (snapshot) of the DB. So, we should always wait to accumulate skip
	// entries before we start taking a snapshot.
	count, err := n.Store.NumEntries()
	if err != nil {
		tr.LazyPrintf("Error: %v", err)
		tr.SetError()
		return err
	}
	tr.LazyPrintf("Found Raft entries: %d", count)
	if count < 2*keepN {
		// We wait to build up at least 2*keepN entries, and then discard keepN entries.
		tr.LazyPrintf("Skipping due to insufficient entries")
		return nil
	}
	discard := count - keepN

	first, err := n.Store.FirstIndex()
	if err != nil {
		tr.LazyPrintf("Error: %v", err)
		tr.SetError()
		return err
	}
	tr.LazyPrintf("First index: %d", first)
	last := first + uint64(discard)
	if last > n.Applied.DoneUntil() {
		tr.LazyPrintf("Skipping because last index: %d > applied", last)
		return nil
	}
	entries, err := n.Store.Entries(first, last, math.MaxUint64)
	if err != nil {
		tr.LazyPrintf("Error: %v", err)
		tr.SetError()
		return err
	}

	// We find the minimum start ts for which a decision to commit or abort is still pending. We
	// should not discard mutations corresponding to this start ts, because we don't persist
	// mutations until they are committed.
	minPending := posting.Oracle().MinPendingStartTs()
	tr.LazyPrintf("Found min pending start ts: %d", minPending)
	var snapshotIdx uint64
	for _, entry := range entries {
		if entry.Type != raftpb.EntryNormal {
			continue
		}
		var proposal intern.Proposal
		if err := proposal.Unmarshal(entry.Data); err != nil {
			tr.LazyPrintf("Error: %v", err)
			tr.SetError()
			return err
		}
		mu := proposal.Mutations
		if mu != nil && mu.StartTs >= minPending {
			break
		}
		snapshotIdx = entry.Index
	}
	tr.LazyPrintf("Got snapshotIdx: %d. Discarding: %d", snapshotIdx, snapshotIdx-first)
	// We should discard at least half of keepN. Otherwise, why bother.
	if snapshotIdx == 0 || int(snapshotIdx-first) < int(float64(keepN)*0.5) {
		tr.LazyPrintf("Skipping snapshot because insufficient discard entries")
		x.Printf("Skipping snapshot at index: %d. Insufficient discard entries: %d."+
			" MinPendingStartTs: %d\n", snapshotIdx, snapshotIdx-first, minPending)
		return nil
	}

	snap := &intern.Snapshot{
		Context:           n.RaftContext,
		Index:             snapshotIdx,
		MinPendingStartTs: minPending,
	}
	proposal := &intern.Proposal{
		Snapshot: snap,
	}
	tr.LazyPrintf("Proposing snapshot: %+v\n", snap)

	data, err := proposal.Marshal()
	x.Check(err)
	if err := n.Raft().Propose(context.Background(), data); err != nil {
		tr.LazyPrintf("Error: %v", err)
		tr.SetError()
		return err
	}
	tr.LazyPrintf("Done best-effort proposing.")
	return nil
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
