// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"go.etcd.io/etcd/raft/raftpb"
)

const defaultCDCConfig = "file=; kafka=; sasl_user=; sasl_password=; ca_cert=; client_cert=; client_key="
const defaultEventTopic = "dgraph-cdc"
const defaultEventKey = "dgraph-cdc-event"

type CDC struct {
	sync.Mutex
	sink             Sink
	closer           *z.Closer
	pendingTxnEvents map[uint64][]CDCEvent

	// dont use mutex, use atomic for these
	seenIndex uint64 // index till which we have read the raft logs.
	sentTs    uint64 // max commit ts for which we have send the events.
}

func newCDC() *CDC {
	if Config.ChangeDataConf == "" {
		return nil
	}

	cdcFlag := x.NewSuperFlag(Config.ChangeDataConf).MergeAndCheckDefault(defaultCDCConfig)
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

func (cdc *CDC) addEventsAndUpdateIndex(ts uint64, index uint64, events []CDCEvent) {
	if cdc == nil {
		return
	}
	cdc.Lock()
	cdc.pendingTxnEvents[ts] = append(cdc.pendingTxnEvents[ts], events...)
	cdc.Unlock()
	cdc.updateSeenIndex(index)
}

func (cdc *CDC) deleteEventsAndUpdateIndex(ts uint64, index uint64) {
	if cdc == nil {
		return
	}
	cdc.Lock()
	delete(cdc.pendingTxnEvents, ts)
	cdc.Unlock()
	cdc.updateSeenIndex(index)
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
	// events are sent as soon as we get mutation proposal in ludicrous mode.
	// hence we only update the sentIndex in ludicrous mode.
	// in normal mode sentTs will manage the state of CDC across cluster.
	// Therefore, sentIndex has same significance in ludicrous mode as sentTs in default mode.
	// Dont try to update seen index in case of default mode else cdc job will not be able to
	// build the complete pending txns in case of membership changes.
	if x.WorkerConfig.LudicrousMode {
		cdc.updateSeenIndex(state.SentIndex)
		return
	}
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

	var batch []SinkMessage

	flushEvents := func(commitTs uint64) error {
		if err := cdc.sink.Send(batch); err != nil {
			glog.Errorf("error while sending cdc event to sink %+v", err)
			batch = batch[:0]
			return err
		}
		// We successfully sent messages to sink.
		atomic.StoreUint64(&cdc.sentTs, commitTs)
		batch = batch[:0]
		return nil
	}

	sendEvents := func(pending []CDCEvent, commitTs uint64) error {
		for _, e := range pending {
			e.Meta.CommitTs = commitTs
			b, err := json.Marshal(e)
			x.Check(err)
			// todo(aman bansal): use namespace for key.
			batch = append(batch, SinkMessage{
				Meta: SinkMeta{
					Topic: defaultEventTopic,
				},
				Key:   []byte(defaultEventKey),
				Value: b,
			})
		}
		//if len(batch) > 1000 {
		//	return flushEvents()
		//}
		//return nil
		return flushEvents(commitTs)
	}

	// This will always run on leader node only. For default mode,
	// Leader will check the Raft logs and keep in memory events that are pending.
	// Once Txn is done, it will try to send events to sink.
	// Across the cluster, the process will manage sentTs
	// as the maximum commit timestamp CDC has sent to the sink.
	// In this way, even if leadership changes,
	// new leader will know which new events are to be sent.
	//
	// In case of ludicrous mode, this has been achieved using seenIndex.
	// We send the events to the sink as soon as we get the proposal.Mutation
	// in ludicrous mode. Hence, this job will manage seenIndex across the cluster
	// to manage from which index we have to send the events.
	checkAndSendCDCEvents := func() error {
		first, err := groups().Node.Store.FirstIndex()
		x.Check(err)
		cdcIndex := x.Max(atomic.LoadUint64(&cdc.seenIndex)+1, first)
		last := groups().Node.Applied.DoneUntil()
		if cdcIndex == last {
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
				if entry.Type != raftpb.EntryNormal || len(entry.Data) == 0 {
					continue
				}

				var proposal pb.Proposal
				if err := proposal.Unmarshal(entry.Data[8:]); err != nil {
					glog.Warningf("CDC: unmarshal failed with error %v. Ignoring.", err)
					continue
				}

				if proposal.Mutations != nil {
					events := toCDCEvent(entry.Index, proposal.Mutations)
					if len(events) == 0 {
						continue
					}
					// In ludicrous, we send the events as soon as we get it.
					// We won't wait for oracle delta in case of ludicrous mode
					// since all mutations will eventually succeed.
					if x.WorkerConfig.LudicrousMode {
						if err := sendEvents(events, 0); err != nil {
							return errors.Wrapf(err, "unable to send messages")
						}
						cdc.updateSeenIndex(entry.Index)
						continue
					}
					cdc.addEventsAndUpdateIndex(proposal.Mutations.StartTs, entry.Index, events)
				}

				if proposal.Delta != nil {
					for _, ts := range proposal.Delta.Txns {
						// This ensures we dont send events again in case of membership changes.
						if ts.CommitTs > 0 && atomic.LoadUint64(&cdc.sentTs) < ts.CommitTs {
							events := cdc.pendingTxnEvents[ts.StartTs]
							if err := sendEvents(events, ts.CommitTs); err != nil {
								return errors.Wrapf(err, "unable to send messages to sink")
							}
						}
						// Delete from pending events once events are sent.
						cdc.deleteEventsAndUpdateIndex(ts.StartTs, entry.Index)
					}
				}
			}
		}

		//if err := flushEvents(); err != nil {
		//	return errors.Wrapf(err, "unable to flush messages to sink")
		//}
		return nil
	}

	jobTick := time.NewTicker(time.Second)
	proposalTick := time.NewTicker(5 * time.Minute)
	defer cdc.closer.Done()
	defer jobTick.Stop()
	defer proposalTick.Stop()
	for {
		select {
		case <-cdc.closer.HasBeenClosed():
			return
		case <-jobTick.C:
			if groups().Node.AmLeader() && EnterpriseEnabled() {
				if err := checkAndSendCDCEvents(); err != nil {
					glog.Errorf("unable to send events %+v", err)
				}
			}
		case <-proposalTick.C:
			if groups().Node.AmLeader() && EnterpriseEnabled() {
				if err := groups().Node.proposeCDCState(atomic.LoadUint64(&cdc.seenIndex),
					atomic.LoadUint64(&cdc.sentTs)); err != nil {
					glog.Errorf("unable to propose cdc state %+v", err)
				}
			}
		}
	}
}

type CDCEvent struct {
	Meta      *EventMeta  `json:"meta"`
	EventType string      `json:"event_type"`
	Event     interface{} `json:"event"`
}

type EventMeta struct {
	RaftIndex uint64 `json:"-"`
	StartTs   uint64 `json:"-"`
	CommitTs  uint64 `json:"commit_ts"`
}

type MutationEvent struct {
	MutationType  string      `json:"mutation_type"`
	Uid           uint64      `json:"uid"`
	Attribute     string      `json:"attribute"`
	Value         interface{} `json:"value"`
	ValueDataType string      `json:"value_data_type"`
}

type DropEvent struct {
	Operation string `json:"operation"`
	Type      string `json:"type"`
	Pred      string `json:"pred"`
}

const (
	EventTypeDrop     = "DROP"
	EventTypeMutation = "MUTATION"
	OpDropPred        = "PREDICATE"
)

func toCDCEvent(index uint64, mutation *pb.Mutations) []CDCEvent {
	// todo(Aman): we are skipping schema updates for now. Fix this later.
	if len(mutation.Schema) > 0 || len(mutation.Types) > 0 {
		return nil
	}

	// If drop operation
	if mutation.DropOp != pb.Mutations_NONE {
		return []CDCEvent{
			{
				EventType: EventTypeDrop,
				Event: &DropEvent{
					Operation: mutation.DropOp.String(),
					Type:      mutation.DropValue,
				},
				Meta: &EventMeta{
					RaftIndex: index,
				},
			},
		}
	}

	cdcEvents := make([]CDCEvent, 0)
	for _, edge := range mutation.Edges {
		if x.IsReservedPredicate(edge.Attr) {
			continue
		}
		// Handle drop attr event.
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			return []CDCEvent{
				{
					EventType: EventTypeDrop,
					Event: &DropEvent{
						Operation: OpDropPred,
						Pred:      edge.Attr,
					},
					Meta: &EventMeta{
						RaftIndex: index,
					},
				},
			}
		}

		var val interface{}
		if posting.TypeID(edge) == types.UidID {
			val = edge.ValueId
		} else {
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
				StartTs:   mutation.StartTs,
			},
			EventType: EventTypeMutation,
			Event: &MutationEvent{
				MutationType:  edge.Op.String(),
				Uid:           edge.Entity,
				Attribute:     edge.Attr,
				Value:         val,
				ValueDataType: posting.TypeID(edge).Name(),
			},
		})
	}

	return cdcEvents
}
