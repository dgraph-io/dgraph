//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft/raftpb"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

const (
	defaultEventTopic = "dgraph-cdc"
)

// CDC struct is being used to send out change data capture events. There are two ways to do this:
// 1. Use Badger Subscribe.
// 2. Use Raft WAL.
// We chose to go with Raft WAL because in case we lose connection to the sink (say Kafka), we can
// resume from the last sent event and ensure there's continuity in event sending. Note the events
// would sent in the same order as they're being committed.
// With Badger Subscribe, if we lose the connection, we would have no way to send over the "missed"
// events. Even if we scan over Badger, we'd still not get those events in the right order, i.e.
// order of their commit timestamp. So, this approach would be tricky to get right.
type CDC struct {
	sync.Mutex
	sink             Sink
	closer           *z.Closer
	pendingTxnEvents map[uint64][]CDCEvent

	// dont use mutex, use atomic for the following.

	// seenIndex is the Raft index till which we have read the raft logs, and
	// put the events in our pendingTxnEvents. This does NOT mean that we have
	// sent them yet.
	seenIndex uint64
	sentTs    uint64 // max commit ts for which we have send the events.
}

func newCDC() *CDC {
	if Config.ChangeDataConf == "" || Config.ChangeDataConf == CDCDefaults {
		return nil
	}

	cdcFlag := z.NewSuperFlag(Config.ChangeDataConf).MergeAndCheckDefault(CDCDefaults)
	sink, err := GetSink(cdcFlag)
	x.Check(err)
	cdc := &CDC{
		sink:             sink,
		closer:           z.NewCloser(1),
		pendingTxnEvents: make(map[uint64][]CDCEvent),
	}
	return cdc
}

func (cdc *CDC) getSeenIndex() uint64 {
	if cdc == nil {
		return math.MaxUint64
	}
	return atomic.LoadUint64(&cdc.seenIndex)
}

func (cdc *CDC) getTs() uint64 {
	if cdc == nil {
		return math.MaxUint64
	}
	cdc.Lock()
	defer cdc.Unlock()
	min := uint64(math.MaxUint64)
	for startTs := range cdc.pendingTxnEvents {
		min = x.Min(min, startTs)
	}
	return min
}

func (cdc *CDC) resetPendingEvents() {
	if cdc == nil {
		return
	}
	cdc.Lock()
	defer cdc.Unlock()
	cdc.pendingTxnEvents = make(map[uint64][]CDCEvent)
}

func (cdc *CDC) resetPendingEventsForNs(ns uint64) {
	if cdc == nil {
		return
	}
	cdc.Lock()
	defer cdc.Unlock()
	for ts, events := range cdc.pendingTxnEvents {
		if len(events) > 0 && binary.BigEndian.Uint64(events[0].Meta.Namespace) == ns {
			delete(cdc.pendingTxnEvents, ts)
		}
	}
}

func (cdc *CDC) hasPending(attr string) bool {
	if cdc == nil {
		return false
	}
	cdc.Lock()
	defer cdc.Unlock()
	for _, events := range cdc.pendingTxnEvents {
		for _, e := range events {
			if me, ok := e.Event.(*MutationEvent); ok && me.Attr == attr {
				return true
			}
		}
	}
	return false
}

func (cdc *CDC) addToPending(ts uint64, events []CDCEvent) {
	if cdc == nil {
		return
	}
	cdc.Lock()
	defer cdc.Unlock()
	cdc.pendingTxnEvents[ts] = append(cdc.pendingTxnEvents[ts], events...)
}

func (cdc *CDC) removeFromPending(ts uint64) {
	if cdc == nil {
		return
	}
	cdc.Lock()
	defer cdc.Unlock()
	delete(cdc.pendingTxnEvents, ts)
}

func (cdc *CDC) updateSeenIndex(index uint64) {
	if cdc == nil {
		return
	}
	idx := atomic.LoadUint64(&cdc.seenIndex)
	if idx >= index {
		return
	}
	atomic.CompareAndSwapUint64(&cdc.seenIndex, idx, index)
}

func (cdc *CDC) updateCDCState(state *pb.CDCState) {
	if cdc == nil {
		return
	}

	// Dont try to update seen index in case of default mode else cdc job will not
	// be able to build the complete pending txns in case of membership changes.
	ts := atomic.LoadUint64(&cdc.sentTs)
	if ts >= state.SentTs {
		return
	}
	atomic.CompareAndSwapUint64(&cdc.sentTs, ts, state.SentTs)
}

func (cdc *CDC) Close() {
	if cdc == nil {
		return
	}
	glog.Infof("closing CDC events...")
	cdc.closer.SignalAndWait()
	err := cdc.sink.Close()
	glog.Errorf("error while closing sink %v", err)
}

func (cdc *CDC) processCDCEvents() {
	if cdc == nil {
		return
	}

	sendToSink := func(pending []CDCEvent, commitTs uint64) error {
		batch := make([]SinkMessage, len(pending))
		for i, e := range pending {
			e.Meta.CommitTs = commitTs
			b, err := json.Marshal(e)
			x.Check(err)
			batch[i] = SinkMessage{
				Meta: SinkMeta{
					Topic: defaultEventTopic,
				},
				Key:   e.Meta.Namespace,
				Value: b,
			}
		}
		if err := cdc.sink.Send(batch); err != nil {
			glog.Errorf("error while sending cdc event to sink %+v", err)
			return err
		}
		// We successfully sent messages to sink.
		atomic.StoreUint64(&cdc.sentTs, commitTs)
		return nil
	}

	handleEntry := func(entry raftpb.Entry) (rerr error) {
		defer func() {
			// Irrespective of whether we act on this entry or not, we should
			// always update the seenIndex. Otherwise, we'll loop over these
			// entries over and over again. However, if we encounter an error,
			// we should not update the index.
			if rerr == nil {
				cdc.updateSeenIndex(entry.Index)
			}
		}()

		if entry.Type != raftpb.EntryNormal || len(entry.Data) == 0 {
			return
		}

		var proposal pb.Proposal
		if err := proposal.Unmarshal(entry.Data[8:]); err != nil {
			glog.Warningf("CDC: unmarshal failed with error %v. Ignoring.", err)
			return
		}
		if proposal.Mutations != nil {
			events := toCDCEvent(entry.Index, proposal.Mutations)
			if len(events) == 0 {
				return
			}
			edges := proposal.Mutations.Edges
			switch {
			case proposal.Mutations.DropOp != pb.Mutations_NONE: // this means its a drop operation
				// if there is DROP ALL or DROP DATA operation, clear pending events also.
				if proposal.Mutations.DropOp == pb.Mutations_ALL {
					cdc.resetPendingEvents()
				} else if proposal.Mutations.DropOp == pb.Mutations_DATA {
					ns, err := strconv.ParseUint(proposal.Mutations.DropValue, 0, 64)
					if err != nil {
						glog.Warningf("CDC: parsing namespace failed with error %v. Ignoring.", err)
						return
					}
					cdc.resetPendingEventsForNs(ns)
				}
				if err := sendToSink(events, proposal.Mutations.StartTs); err != nil {
					rerr = errors.Wrapf(err, "unable to send messages to sink")
					return
				}
				// If drop predicate, then mutation only succeeds if there were no pending txn
				// This check ensures then event will only be send if there were no pending txns
			case len(edges) == 1 &&
				edges[0].Entity == 0 &&
				bytes.Equal(edges[0].Value, []byte(x.Star)):
				// If there are no pending txn send the events else
				// return as the mutation must have errored out in that case.
				if !cdc.hasPending(x.ParseAttr(edges[0].Attr)) {
					if err := sendToSink(events, proposal.Mutations.StartTs); err != nil {
						rerr = errors.Wrapf(err, "unable to send messages to sink")
					}
				}
				return
			default:
				cdc.addToPending(proposal.Mutations.StartTs, events)
			}
		}

		if proposal.Delta != nil {
			for _, ts := range proposal.Delta.Txns {
				// This ensures we dont send events again in case of membership changes.
				if ts.CommitTs > 0 && atomic.LoadUint64(&cdc.sentTs) < ts.CommitTs {
					events := cdc.pendingTxnEvents[ts.StartTs]
					if err := sendToSink(events, ts.CommitTs); err != nil {
						rerr = errors.Wrapf(err, "unable to send messages to sink")
						return
					}
				}
				// Delete from pending events once events are sent.
				cdc.removeFromPending(ts.StartTs)
			}
		}
		return
	}

	// This will always run on leader node only. For default mode, Leader will
	// check the Raft logs and keep in memory events that are pending.  Once
	// Txn is done, it will send events to sink, and update sentTs locally.
	sendEvents := func() error {
		first, err := groups().Node.Store.FirstIndex()
		x.Check(err)
		cdcIndex := x.Max(atomic.LoadUint64(&cdc.seenIndex)+1, first)

		last := groups().Node.Applied.DoneUntil()
		if cdcIndex > last {
			return nil
		}
		for batchFirst := cdcIndex; batchFirst <= last; {
			entries, err := groups().Node.Store.Entries(batchFirst, last+1, 256<<20)
			if err != nil {
				return errors.Wrapf(err,
					"CDC: failed to retrieve entries from Raft. Start: %d End: %d",
					batchFirst, last+1)
			}
			if len(entries) == 0 {
				return nil
			}
			batchFirst = entries[len(entries)-1].Index + 1
			for _, entry := range entries {
				if err := handleEntry(entry); err != nil {
					return errors.Wrapf(err, "CDC: unable to process raft entry")
				}
			}
		}
		return nil
	}

	jobTick := time.NewTicker(time.Second)
	proposalTick := time.NewTicker(3 * time.Minute)
	defer cdc.closer.Done()
	defer jobTick.Stop()
	defer proposalTick.Stop()
	var lastSent uint64
	for {
		select {
		case <-cdc.closer.HasBeenClosed():
			return
		case <-jobTick.C:
			if groups().Node.AmLeader() && EnterpriseEnabled() {
				if err := sendEvents(); err != nil {
					glog.Errorf("unable to send events %+v", err)
				}
			}
		case <-proposalTick.C:
			// The leader would propose the max sentTs over to the group.
			// So, in case of a crash or a leadership change, the new leader
			// would know where to send the cdc events from the Raft logs.
			if groups().Node.AmLeader() && EnterpriseEnabled() {
				sentTs := atomic.LoadUint64(&cdc.sentTs)
				if lastSent == sentTs {
					// No need to propose anything.
					continue
				}
				if err := groups().Node.proposeCDCState(atomic.LoadUint64(&cdc.sentTs)); err != nil {
					glog.Errorf("unable to propose cdc state %+v", err)
				} else {
					lastSent = sentTs
				}
			}
		}
	}
}

type CDCEvent struct {
	Meta  *EventMeta  `json:"meta"`
	Type  string      `json:"type"`
	Event interface{} `json:"event"`
}

type EventMeta struct {
	RaftIndex uint64 `json:"-"`
	Namespace []byte `json:"-"`
	CommitTs  uint64 `json:"commit_ts"`
}

type MutationEvent struct {
	Operation string      `json:"operation"`
	Uid       uint64      `json:"uid"`
	Attr      string      `json:"attr"`
	Value     interface{} `json:"value"`
	ValueType string      `json:"value_type"`
}

type DropEvent struct {
	Operation string `json:"operation"`
	Type      string `json:"type"`
	Pred      string `json:"pred"`
}

const (
	EventTypeDrop     = "drop"
	EventTypeMutation = "mutation"
	OpDropPred        = "predicate"
)

func toCDCEvent(index uint64, mutation *pb.Mutations) []CDCEvent {
	// todo(Aman): we are skipping schema updates for now. Fix this later.
	if len(mutation.Schema) > 0 || len(mutation.Types) > 0 {
		return nil
	}

	// If drop operation
	if mutation.DropOp != pb.Mutations_NONE {
		namespace := make([]byte, 8)
		var t string
		switch mutation.DropOp {
		case pb.Mutations_ALL:
			// Drop all is cluster wide.
			binary.BigEndian.PutUint64(namespace, x.GalaxyNamespace)
		case pb.Mutations_DATA:
			ns, err := strconv.ParseUint(mutation.DropValue, 0, 64)
			if err != nil {
				glog.Warningf("CDC: parsing namespace failed with error %v. Ignoring.", err)
				return nil
			}
			binary.BigEndian.PutUint64(namespace, ns)
		case pb.Mutations_TYPE:
			namespace, t = x.ParseNamespaceBytes(mutation.DropValue)
		default:
			glog.Error("CDC: got unhandled drop operation")
		}

		return []CDCEvent{
			{
				Type: EventTypeDrop,
				Event: &DropEvent{
					Operation: strings.ToLower(mutation.DropOp.String()),
					Type:      t,
				},
				Meta: &EventMeta{
					RaftIndex: index,
					Namespace: namespace,
				},
			},
		}
	}

	cdcEvents := make([]CDCEvent, 0)
	for _, edge := range mutation.Edges {
		if x.IsReservedPredicate(edge.Attr) {
			continue
		}
		ns, attr := x.ParseNamespaceBytes(edge.Attr)
		// Handle drop attr event.
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			return []CDCEvent{
				{
					Type: EventTypeDrop,
					Event: &DropEvent{
						Operation: OpDropPred,
						Pred:      attr,
					},
					Meta: &EventMeta{
						RaftIndex: index,
						Namespace: ns,
					},
				},
			}
		}

		var val interface{}
		switch {
		case posting.TypeID(edge) == types.UidID:
			val = edge.ValueId
		case posting.TypeID(edge) == types.PasswordID:
			val = "****"
		default:
			// convert to correct type
			src := types.Val{Tid: types.BinaryID, Value: edge.Value}
			if v, err := types.Convert(src, posting.TypeID(edge)); err == nil {
				val = v.Value
			} else {
				glog.Errorf("error while converting value %v", err)
			}
		}
		cdcEvents = append(cdcEvents, CDCEvent{
			Meta: &EventMeta{
				RaftIndex: index,
				Namespace: ns,
			},
			Type: EventTypeMutation,
			Event: &MutationEvent{
				Operation: strings.ToLower(edge.Op.String()),
				Uid:       edge.Entity,
				Attr:      attr,
				Value:     val,
				ValueType: posting.TypeID(edge).Name(),
			},
		})
	}

	return cdcEvents
}
