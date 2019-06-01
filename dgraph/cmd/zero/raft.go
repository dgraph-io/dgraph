/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package zero

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
	"github.com/golang/glog"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"
	"golang.org/x/net/context"
)

type node struct {
	*conn.Node
	server      *Server
	ctx         context.Context
	reads       map[uint64]chan uint64
	subscribers map[uint32]chan struct{}
	closer      *y.Closer // to stop Run.

	// The last timestamp when this Zero was able to reach quorum.
	mu         sync.RWMutex
	lastQuorum time.Time
}

func (n *node) AmLeader() bool {
	if n.Raft() == nil {
		return false
	}
	r := n.Raft()
	if r.Status().Lead != r.Status().ID {
		return false
	}

	// This node must be the leader, but must also be an active member of the cluster, and not
	// hidden behind a partition. Basically, if this node was the leader and goes behind a
	// partition, it would still think that it is indeed the leader for the duration mentioned
	// below.
	n.mu.RLock()
	defer n.mu.RUnlock()
	return time.Since(n.lastQuorum) <= 5*time.Second
}

func (n *node) uniqueKey() string {
	return fmt.Sprintf("z%x-%d", n.Id, n.Rand.Uint64())
}

var errInternalRetry = errors.New("Retry Raft proposal internally")

func (n *node) proposeAndWait(ctx context.Context, proposal *pb.ZeroProposal) error {
	switch {
	case n.Raft() == nil:
		return errors.Errorf("Raft isn't initialized yet.")
	case ctx.Err() != nil:
		return ctx.Err()
	case !n.AmLeader():
		// Do this check upfront. Don't do this inside propose for reasons explained below.
		return errors.Errorf("Not Zero leader. Aborting proposal: %+v", proposal)
	}

	// We could consider adding a wrapper around the user proposal, so we can access any key-values.
	// Something like this:
	// https://github.com/golang/go/commit/5d39260079b5170e6b4263adb4022cc4b54153c4
	span := otrace.FromContext(ctx)
	// Overwrite ctx, so we no longer enforce the timeouts or cancels from ctx.
	ctx = otrace.NewContext(context.Background(), span)

	stop := x.SpanTimer(span, "n.proposeAndWait")
	defer stop()

	// propose runs in a loop. So, we should not do any checks inside, including n.AmLeader. This is
	// to avoid the scenario where the first proposal times out and the second one gets returned
	// due to node no longer being the leader. In this scenario, the first proposal can still get
	// accepted by Raft, causing a txn violation later for us, because we assumed that the proposal
	// did not go through.
	propose := func(timeout time.Duration) error {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		errCh := make(chan error, 1)
		pctx := &conn.ProposalCtx{
			ErrCh: errCh,
			// Don't use the original context, because that's not what we're passing to Raft.
			Ctx: cctx,
		}
		key := n.uniqueKey()
		x.AssertTruef(n.Proposals.Store(key, pctx), "Found existing proposal with key: [%v]", key)
		defer n.Proposals.Delete(key)
		proposal.Key = key
		span.Annotatef(nil, "Proposing with key: %s. Timeout: %v", key, timeout)

		data, err := proposal.Marshal()
		if err != nil {
			return err
		}
		// Propose the change.
		if err := n.Raft().Propose(cctx, data); err != nil {
			span.Annotatef(nil, "Error while proposing via Raft: %v", err)
			return errors.Wrapf(err, "While proposing")
		}

		// Wait for proposal to be applied or timeout.
		select {
		case err := <-errCh:
			// We arrived here by a call to n.props.Done().
			return err
		case <-cctx.Done():
			span.Annotatef(nil, "Internal context timeout %s. Will retry...", timeout)
			return errInternalRetry
		}
	}

	// Some proposals can be stuck if leader change happens. For e.g. MsgProp message from follower
	// to leader can be dropped/end up appearing with empty Data in CommittedEntries.
	// Having a timeout here prevents the mutation being stuck forever in case they don't have a
	// timeout. We should always try with a timeout and optionally retry.
	err := errInternalRetry
	timeout := 4 * time.Second
	for err == errInternalRetry {
		err = propose(timeout)
		timeout *= 2 // Exponential backoff
		if timeout > time.Minute {
			timeout = 32 * time.Second
		}
	}
	return err
}

var (
	errInvalidProposal     = errors.New("Invalid group proposal")
	errTabletAlreadyServed = errors.New("Tablet is already being served")
)

func newGroup() *pb.Group {
	return &pb.Group{
		Members: make(map[uint64]*pb.Member),
		Tablets: make(map[string]*pb.Tablet),
	}
}

func (n *node) handleMemberProposal(member *pb.Member) error {
	n.server.AssertLock()
	state := n.server.state

	m := n.server.member(member.Addr)
	// Ensures that different nodes don't have same address.
	if m != nil && (m.Id != member.Id || m.GroupId != member.GroupId) {
		return errors.Errorf("Found another member %d with same address: %v", m.Id, m.Addr)
	}
	if member.GroupId == 0 {
		state.Zeros[member.Id] = member
		if member.Leader {
			// Unset leader flag for other nodes, there can be only one
			// leader at a time.
			for _, m := range state.Zeros {
				if m.Id != member.Id {
					m.Leader = false
				}
			}
		}
		return nil
	}
	group := state.Groups[member.GroupId]
	if group == nil {
		group = newGroup()
		state.Groups[member.GroupId] = group
	}
	m, has := group.Members[member.Id]
	if member.AmDead {
		if has {
			delete(group.Members, member.Id)
			state.Removed = append(state.Removed, m)
		}
		// else already removed.
		if len(group.Members) == 0 {
			glog.V(3).Infof("Deleting group Id %d (no members) ...", member.GroupId)
			delete(state.Groups, member.GroupId)
		}
		return nil
	}
	if !has && len(group.Members) >= n.server.NumReplicas {
		// We shouldn't allow more members than the number of replicas.
		return errors.Errorf("Group reached replication level. Can't add another member: %+v", member)
	}

	// Create a connection to this server.
	go conn.GetPools().Connect(member.Addr)

	group.Members[member.Id] = member
	// Increment nextGroup when we have enough replicas
	if member.GroupId == n.server.nextGroup &&
		len(group.Members) >= n.server.NumReplicas {
		n.server.nextGroup++
	}
	if member.Leader {
		// Unset leader flag for other nodes, there can be only one
		// leader at a time.
		for _, m := range group.Members {
			if m.Id != member.Id {
				m.Leader = false
			}
		}
	}
	// On replay of logs on restart we need to set nextGroup.
	if n.server.nextGroup <= member.GroupId {
		n.server.nextGroup = member.GroupId + 1
	}
	return nil
}

func (n *node) handleTabletProposal(tablet *pb.Tablet) error {
	n.server.AssertLock()
	state := n.server.state
	defer func() {
		// Regenerate group checksums. These checksums are solely based on which tablets are being
		// served by the group. If the tablets that a group is serving changes, and the Alpha does
		// not know about these changes, then the read request must fail.
		for _, g := range state.GetGroups() {
			preds := make([]string, 0, len(g.GetTablets()))
			for pred := range g.GetTablets() {
				preds = append(preds, pred)
			}
			sort.Strings(preds)
			g.Checksum = farm.Fingerprint64([]byte(strings.Join(preds, "")))
		}
		if n.AmLeader() {
			// It is important to push something to Oracle updates channel, so the subscribers would
			// get the latest checksum that we calculated above. Otherwise, if all the queries are
			// best effort queries which don't create any transaction, then the OracleDelta never
			// gets sent to Alphas, causing their group checksum to mismatch and never converge.
			n.server.orc.updates <- &pb.OracleDelta{}
		}
	}()

	if tablet.GroupId == 0 {
		return errors.Errorf("Tablet group id is zero: %+v", tablet)
	}
	group := state.Groups[tablet.GroupId]
	if tablet.Remove {
		glog.Infof("Removing tablet for attr: [%v], gid: [%v]\n", tablet.Predicate, tablet.GroupId)
		if group != nil {
			delete(group.Tablets, tablet.Predicate)
		}
		return nil
	}
	if group == nil {
		group = newGroup()
		state.Groups[tablet.GroupId] = group
	}

	// There's a edge case that we're handling.
	// Two servers ask to serve the same tablet, then we need to ensure that
	// only the first one succeeds.
	if prev := n.server.servingTablet(tablet.Predicate); prev != nil {
		if tablet.Force {
			originalGroup := state.Groups[prev.GroupId]
			delete(originalGroup.Tablets, tablet.Predicate)
		} else {
			if prev.GroupId != tablet.GroupId {
				glog.Infof(
					"Tablet for attr: [%s], gid: [%d] already served by group: [%d]\n",
					prev.Predicate, tablet.GroupId, prev.GroupId)
				return errTabletAlreadyServed
			}
		}
	}
	tablet.Force = false
	group.Tablets[tablet.Predicate] = tablet
	return nil
}

func (n *node) applyProposal(e raftpb.Entry) (string, error) {
	var p pb.ZeroProposal
	// Raft commits empty entry on becoming a leader.
	if len(e.Data) == 0 {
		return p.Key, nil
	}
	if err := p.Unmarshal(e.Data); err != nil {
		return p.Key, err
	}
	if len(p.Key) == 0 {
		return p.Key, errInvalidProposal
	}
	span := otrace.FromContext(n.Proposals.Ctx(p.Key))

	n.server.Lock()
	defer n.server.Unlock()

	state := n.server.state
	state.Counter = e.Index
	if len(p.Cid) > 0 {
		if len(state.Cid) > 0 {
			return p.Key, errInvalidProposal
		}
		state.Cid = p.Cid
	}
	if p.MaxRaftId > 0 {
		if p.MaxRaftId <= state.MaxRaftId {
			return p.Key, errInvalidProposal
		}
		state.MaxRaftId = p.MaxRaftId
	}
	if p.SnapshotTs != nil {
		for gid, ts := range p.SnapshotTs {
			if group, ok := state.Groups[gid]; ok {
				group.SnapshotTs = x.Max(group.SnapshotTs, ts)
			}
		}
		purgeTs := uint64(math.MaxUint64)
		for _, group := range state.Groups {
			purgeTs = x.Min(purgeTs, group.SnapshotTs)
		}
		if purgeTs < math.MaxUint64 {
			n.server.orc.purgeBelow(purgeTs)
		}
	}
	if p.Member != nil {
		if err := n.handleMemberProposal(p.Member); err != nil {
			span.Annotatef(nil, "While applying membership proposal: %+v", err)
			glog.Errorf("While applying membership proposal: %+v", err)
			return p.Key, err
		}
	}
	if p.Tablet != nil {
		if err := n.handleTabletProposal(p.Tablet); err != nil {
			span.Annotatef(nil, "While applying tablet proposal: %+v", err)
			glog.Errorf("While applying tablet proposal: %+v", err)
			return p.Key, err
		}
	}

	if p.MaxLeaseId > state.MaxLeaseId {
		state.MaxLeaseId = p.MaxLeaseId
	} else if p.MaxTxnTs > state.MaxTxnTs {
		state.MaxTxnTs = p.MaxTxnTs
	} else if p.MaxLeaseId != 0 || p.MaxTxnTs != 0 {
		// Could happen after restart when some entries were there in WAL and did not get
		// snapshotted.
		glog.Infof("Could not apply proposal, ignoring: p.MaxLeaseId=%v, p.MaxTxnTs=%v maxLeaseId=%d"+
			" maxTxnTs=%d\n", p.MaxLeaseId, p.MaxTxnTs, state.MaxLeaseId, state.MaxTxnTs)
	}
	if p.Txn != nil {
		n.server.orc.updateCommitStatus(e.Index, p.Txn)
	}

	return p.Key, nil
}

func (n *node) applyConfChange(e raftpb.Entry) {
	var cc raftpb.ConfChange
	cc.Unmarshal(e.Data)

	if cc.Type == raftpb.ConfChangeRemoveNode {
		if cc.NodeID == n.Id {
			glog.Fatalf("I [id:%#x group:0] have been removed. Goodbye!", n.Id)
		}
		n.DeletePeer(cc.NodeID)
		n.server.removeZero(cc.NodeID)

	} else if len(cc.Context) > 0 {
		var rc pb.RaftContext
		x.Check(rc.Unmarshal(cc.Context))
		go n.Connect(rc.Id, rc.Addr)

		m := &pb.Member{Id: rc.Id, Addr: rc.Addr, GroupId: 0}
		for _, member := range n.server.membershipState().Removed {
			// It is not recommended to reuse RAFT ids.
			if member.GroupId == 0 && m.Id == member.Id {
				err := errors.Errorf("REUSE_RAFTID: Reusing removed id: %d.\n", m.Id)
				n.DoneConfChange(cc.ID, err)
				// Cancel configuration change.
				cc.NodeID = raft.None
				n.Raft().ApplyConfChange(cc)
				return
			}
		}

		n.server.storeZero(m)
	}

	cs := n.Raft().ApplyConfChange(cc)
	n.SetConfState(cs)
	n.DoneConfChange(cc.ID, nil)

	// The following doesn't really trigger leader change. It's just capturing a leader change
	// event. The naming is poor. TODO: Fix naming, and see if we can simplify this leader change
	// logic.
	n.triggerLeaderChange()
}

func (n *node) triggerLeaderChange() {
	n.server.triggerLeaderChange()
	// We update leader information on each node without proposal. This
	// function is called on all nodes on leader change.
	n.server.updateZeroLeader()
}

func (n *node) initAndStartNode() error {
	_, restart, err := n.PastLife()
	x.Check(err)

	if restart {
		glog.Infoln("Restarting node for dgraphzero")
		sp, err := n.Store.Snapshot()
		x.Checkf(err, "Unable to get existing snapshot")
		if !raft.IsEmptySnap(sp) {
			// It is important that we pick up the conf state here.
			n.SetConfState(&sp.Metadata.ConfState)

			var state pb.MembershipState
			x.Check(state.Unmarshal(sp.Data))
			n.server.SetMembershipState(&state)
			for _, id := range sp.Metadata.ConfState.Nodes {
				n.Connect(id, state.Zeros[id].Addr)
			}
		}

		n.SetRaft(raft.RestartNode(n.Cfg))

	} else if len(opts.peer) > 0 {
		p := conn.GetPools().Connect(opts.peer)
		if p == nil {
			return errors.Errorf("Unhealthy connection to %v", opts.peer)
		}

		gconn := p.Get()
		c := pb.NewRaftClient(gconn)
		timeout := 8 * time.Second
		for {
			ctx, cancel := context.WithTimeout(n.ctx, timeout)
			defer cancel()
			// JoinCluster can block indefinitely, raft ignores conf change proposal
			// if it has pending configuration.
			_, err := c.JoinCluster(ctx, n.RaftContext)
			if err == nil {
				break
			}
			if x.ShouldCrash(err) {
				log.Fatalf("Error while joining cluster: %v", err)
			}
			glog.Errorf("Error while joining cluster: %v\n", err)
			timeout *= 2
			if timeout > 32*time.Second {
				timeout = 32 * time.Second
			}
			time.Sleep(timeout) // This is useful because JoinCluster can exit immediately.
		}
		glog.Infof("[%#x] Starting node\n", n.Id)
		n.SetRaft(raft.StartNode(n.Cfg, nil))

	} else {
		data, err := n.RaftContext.Marshal()
		x.Check(err)
		peers := []raft.Peer{{ID: n.Id, Context: data}}
		n.SetRaft(raft.StartNode(n.Cfg, peers))

		go func() {
			// This is a new cluster. So, propose a new ID for the cluster.
			for {
				id := uuid.New().String()
				err := n.proposeAndWait(context.Background(), &pb.ZeroProposal{Cid: id})
				if err == nil {
					glog.Infof("CID set for cluster: %v", id)
					return
				}
				if err == errInvalidProposal {
					return
				}
				glog.Errorf("While proposing CID: %v. Retrying...", err)
				time.Sleep(3 * time.Second)
			}
		}()
	}

	go n.Run()
	go n.BatchAndSendMessages()
	return nil
}

func (n *node) updateZeroMembershipPeriodically(closer *y.Closer) {
	defer closer.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.server.updateZeroLeader()

		case <-closer.HasBeenClosed():
			return
		}
	}
}

var startOption = otrace.WithSampler(otrace.ProbabilitySampler(0.01))

func (n *node) checkQuorum(closer *y.Closer) {
	defer closer.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	quorum := func() {
		// Make this timeout 1.5x the timeout on RunReadIndexLoop.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		ctx, span := otrace.StartSpan(ctx, "Zero.checkQuorum", startOption)
		defer span.End()
		span.Annotatef(nil, "Node id: %d", n.Id)

		if state, err := n.server.latestMembershipState(ctx); err == nil {
			n.mu.Lock()
			n.lastQuorum = time.Now()
			n.mu.Unlock()
			// Also do some connection cleanup.
			conn.GetPools().RemoveInvalid(state)
			span.Annotate(nil, "Updated lastQuorum")

		} else if glog.V(1) {
			span.Annotatef(nil, "Got error: %v", err)
			glog.Warningf("Zero node: %#x unable to reach quorum. Error: %v", n.Id, err)
		}
	}

	for {
		select {
		case <-ticker.C:
			quorum()
		case <-closer.HasBeenClosed():
			return
		}
	}
}

func (n *node) snapshotPeriodically(closer *y.Closer) {
	defer closer.Done()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.trySnapshot(1000)

		case <-closer.HasBeenClosed():
			return
		}
	}
}

func (n *node) trySnapshot(skip uint64) {
	existing, err := n.Store.Snapshot()
	x.Checkf(err, "Unable to get existing snapshot")
	si := existing.Metadata.Index
	idx := n.server.SyncedUntil()
	if idx <= si+skip {
		return
	}

	data, err := n.server.MarshalMembershipState()
	x.Check(err)

	err = n.Store.CreateSnapshot(idx, n.ConfState(), data)
	x.Checkf(err, "While creating snapshot")
	glog.Infof("Writing snapshot at index: %d, applied mark: %d\n", idx, n.Applied.DoneUntil())
}

func (n *node) Run() {
	var leader bool
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	// snapshot can cause select loop to block while deleting entries, so run
	// it in goroutine
	readStateCh := make(chan raft.ReadState, 100)
	closer := y.NewCloser(4)
	defer func() {
		closer.SignalAndWait()
		n.closer.Done()
		glog.Infof("Zero Node.Run finished.")
	}()

	go n.snapshotPeriodically(closer)
	go n.updateZeroMembershipPeriodically(closer)
	go n.checkQuorum(closer)
	go n.RunReadIndexLoop(closer, readStateCh)
	// We only stop runReadIndexLoop after the for loop below has finished interacting with it.
	// That way we know sending to readStateCh will not deadlock.

	var timer x.Timer
	for {
		select {
		case <-n.closer.HasBeenClosed():
			n.Raft().Stop()
			return
		case <-ticker.C:
			n.Raft().Tick()
		case rd := <-n.Raft().Ready():
			timer.Start()
			_, span := otrace.StartSpan(n.ctx, "Zero.RunLoop",
				otrace.WithSampler(otrace.ProbabilitySampler(0.001)))
			for _, rs := range rd.ReadStates {
				// No need to use select-case-default on pushing to readStateCh. It is typically
				// empty.
				readStateCh <- rs
			}
			span.Annotatef(nil, "Pushed %d readstates", len(rd.ReadStates))

			if rd.SoftState != nil {
				if rd.RaftState == raft.StateLeader && !leader {
					glog.Infoln("I've become the leader, updating leases.")
					n.server.updateLeases()
				}
				leader = rd.RaftState == raft.StateLeader
				// Oracle stream would close the stream once it steps down as leader
				// predicate move would cancel any in progress move on stepping down.
				n.triggerLeaderChange()
			}
			if leader {
				// Leader can send messages in parallel with writing to disk.
				for _, msg := range rd.Messages {
					n.Send(msg)
				}
			}
			n.SaveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			timer.Record("disk")
			if rd.MustSync {
				if err := n.Store.Sync(); err != nil {
					glog.Errorf("Error while calling Store.Sync: %v", err)
				}
				timer.Record("sync")
			}
			span.Annotatef(nil, "Saved to storage")

			if !raft.IsEmptySnap(rd.Snapshot) {
				var state pb.MembershipState
				x.Check(state.Unmarshal(rd.Snapshot.Data))
				n.server.SetMembershipState(&state)
			}

			for _, entry := range rd.CommittedEntries {
				n.Applied.Begin(entry.Index)
				if entry.Type == raftpb.EntryConfChange {
					n.applyConfChange(entry)
					glog.Infof("Done applying conf change at %#x", n.Id)

				} else if entry.Type == raftpb.EntryNormal {
					key, err := n.applyProposal(entry)
					if err != nil {
						glog.Errorf("While applying proposal: %v\n", err)
					}
					n.Proposals.Done(key, err)

				} else {
					glog.Infof("Unhandled entry: %+v\n", entry)
				}
				n.Applied.Done(entry.Index)
			}
			span.Annotatef(nil, "Applied %d CommittedEntries", len(rd.CommittedEntries))

			if !leader {
				// Followers should send messages later.
				for _, msg := range rd.Messages {
					n.Send(msg)
				}
			}
			span.Annotate(nil, "Sent messages")
			timer.Record("proposals")

			n.Raft().Advance()
			span.Annotate(nil, "Advanced Raft")
			timer.Record("advance")

			span.End()
			if timer.Total() > 100*time.Millisecond {
				glog.Warningf(
					"Raft.Ready took too long to process: %s."+
						" Num entries: %d. MustSync: %v",
					timer.String(), len(rd.Entries), rd.MustSync)
			}
		}
	}
}
