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
	"strings"
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

const defaultCDCConfig = "enabled=false; max_recovery=10000; file=; kafka=; sasl_user=; sasl_password=; ca_cert=; client_cert=; client_key="

type CDC struct {
	sink               Sink
	maxRecoveryEntries uint64
	closer             *z.Closer
	pendingTxnEvents   map[uint64][]CDCEvent

	seenIndex uint64
	// startTs is the timestamp till which we have send the event of txns.
	// There will be no event below this timestamp for which we need to send the events
	startTs uint64
}

func newCDC() *CDC {
	if Config.ChangeDataConf == "" {
		return nil
	}

	cdcFlag := x.NewSuperFlag(Config.ChangeDataConf).MergeAndCheckDefault(defaultCDCConfig)
	sink, err := GetSink(cdcFlag)
	x.Check(err)
	cdc := &CDC{
		sink:               sink,
		maxRecoveryEntries: cdcFlag.GetUint64("max-recovery"),
		closer:             z.NewCloser(1),
		pendingTxnEvents:   make(map[uint64][]CDCEvent),
	}
	return cdc
}

func (cdc *CDC) getSeenIndex() uint64 {
	// TODO: Fill this in.
	return math.MaxUint64
}

func (cdc *CDC) getTs() uint64 {
	if cdc == nil {
		return math.MaxUint64
	}
	// TODO: Check if this would work with ludicrous mode.
	min := uint64(math.MaxUint64)
	for startTs := range cdc.pendingTxnEvents {
		min = x.Min(min, startTs)
	}
	return min
	// return atomic.LoadUint64(&cdc.startTs)
}

func (cdc *CDC) updateTs(newTs uint64) {
	if cdc == nil {
		return
	}
	// current cdc read ts is larger than the proposed newts. Skip this
	ts := cdc.getTs()
	if ts >= newTs {
		return
	}
	atomic.CompareAndSwapUint64(&cdc.startTs, ts, newTs)
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

	var startTs, startIndex uint64
	var batch []SinkMessage

	flushEvents := func() error {
		if err := cdc.sink.Send(batch); err != nil {
			glog.Errorf("error while sending cdc event to sink %+v", err)
			return err
		}
		// We successfully sent messages to sink.
		batch = batch[:0]
		cdc.updateTs(startTs)
		cdc.seenIndex = x.Max(cdc.seenIndex, startIndex)
		return nil
	}

	sendEvents := func(pending []CDCEvent, ts *pb.TxnStatus) error {
		var commitTs uint64 = 0
		if ts != nil {
			commitTs = ts.CommitTs
		}

		for _, e := range pending {
			// add commit timestamp here
			startTs = x.Max(startTs, e.Meta.StartTs)
			startIndex = x.Max(startIndex, e.Meta.RaftIndex)
			e.Meta.CommitTs = commitTs
			b, err := json.Marshal(e)
			x.Check(err)
			// todo(aman bansal): use namespace for key.
			batch = append(batch, SinkMessage{
				Meta: SinkMeta{
					Topic: "dgraph-cdc",
				},
				Key:   []byte("dgraph-cdc-event"),
				Value: b,
			})
		}
		if len(batch) > 1000 {
			return flushEvents()
		}
		return nil
	}

	// This will always run on leader node only.
	// Leader will check the Raft logs and keep in memory events that are pending.
	// Once Txn is done, it will try to send events to sink.
	// index helps to define from which point we need to start reading from the raft logs
	// clear map whenever we have send the events
	checkAndSendCDCEvents := func() error {
		first, err := groups().Node.Store.FirstIndex()
		x.Check(err)
		cdcIndex := x.Max(cdc.seenIndex, first)
		last := groups().Node.Applied.DoneUntil()
		if cdcIndex == last {
			return nil
		}

		// if cdc is lagging behind the current via maxRecoveryEntries,
		// skip ahead the index to prevent uncontrolled growth of raft logs.
		if uint64(len(cdc.pendingTxnEvents)) > cdc.maxRecoveryEntries {
			cdc.updateTs(posting.Oracle().MaxAssigned())
			cdc.seenIndex = last
			cdc.pendingTxnEvents = make(map[uint64][]CDCEvent)
			return errors.New("too many pending CDC events. Dropping events.")
		}

		startTs := cdc.getTs()
		for batchFirst := cdcIndex; batchFirst <= last; {
			entries, err := groups().Node.Store.Entries(batchFirst, last+1, 256<<20)
			if err != nil {
				return errors.Wrapf(err, "while retrieving entries from Raft")
			}
			x.Check(err)
			// Exit early from the loop if no entries were found.
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
				// this is to ensure that cdcTs will be monotonically increasing
				// In this way no min pending txn in case we skip some entries can affect
				// the sentTs to decrease. This way we will be able to provide guarantees
				// across the cluster in case of failures.
				if proposal.Mutations != nil && proposal.Mutations.StartTs > startTs {
					events := toCDCEvent(entry.Index, proposal.Mutations)
					if len(events) == 0 {
						continue
					}
					// In ludicrous events send the events as soon as you get it.
					// We wont wait for oracle delta in case of ludicrous mode.
					// Since all mutations will eventually succeed.
					// We can set the read ts here only.
					if x.WorkerConfig.LudicrousMode {
						if err := sendEvents(events, nil); err != nil {
							return errors.Wrapf(err, "unable to send messages")
						}
						continue
					}
					cdc.pendingTxnEvents[proposal.Mutations.StartTs] =
						append(cdc.pendingTxnEvents[proposal.Mutations.StartTs], events...)
				}

				if proposal.Delta != nil {
					for _, ts := range proposal.Delta.Txns {
						if ts.CommitTs > 0 {
							events := cdc.pendingTxnEvents[ts.StartTs]
							if err := sendEvents(events, ts); err != nil {
								return errors.Wrapf(err, "unable to send messages to sink")
							}
						}
						// delete from pending events once events are sent
						delete(cdc.pendingTxnEvents, ts.StartTs)
						cdc.evaluateAndSetTs()
					}
				}
				cdc.seenIndex = entry.Index
			}
		}
		return nil
	}

	eventTick := time.NewTicker(time.Second)
	proposalTick := time.NewTicker(time.Minute)
	defer cdc.closer.Done()
	defer eventTick.Stop()
	defer proposalTick.Stop()
	for {
		select {
		case <-cdc.closer.HasBeenClosed():
			return
		case <-eventTick.C:
			if groups().Node.AmLeader() && EnterpriseEnabled() {
				checkAndSendCDCEvents()
			}
		case <-proposalTick.C:
			if groups().Node.AmLeader() && EnterpriseEnabled() {
				ts := cdc.getTs()
				glog.V(2).Infof("proposing CDC sentTs %d", ts)
				if err := groups().Node.proposeCDCTs(ts); err != nil {
					glog.Errorf("not able to propose cdc sentTs %v", err)
				}
			}
		}
	}
}

// evaluateAndSetTs finds the sentTs we have pending events for.
// we can't send MaxUint64 as the response because when proposed,
// it will nullify the state of cdc in the cluster,
// thus making followers to clear raft logs between next proposal.
// In this way we can loose some events.
func (cdc *CDC) evaluateAndSetTs() {
	if cdc == nil || x.WorkerConfig.LudicrousMode {
		return
	}
	min := cdc.maxSentTs
	for ts := range cdc.pendingTxnEvents {
		if ts < min {
			min = ts
		}
	}
	cdc.updateTs(min)
}

type CDCEvent struct {
	Meta      *EventMeta  `json:"meta"`
	EventType string      `json:"event_type"`
	Event     interface{} `json:"event"`
}

type EventMeta struct {
	RaftIndex uint64 `json:"raft_index"`
	StartTs   uint64 `json:"start_ts"`
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

func toCDCEvent(index uint64, mutation *pb.Mutations) []CDCEvent {
	// we are skipping schema updates for now.
	if len(mutation.Schema) > 0 || len(mutation.Types) > 0 {
		return nil
	}

	// if drop operation
	if mutation.DropOp != pb.Mutations_NONE {
		return []CDCEvent{
			{
				EventType: "DROP",
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
		if skipAttribute(edge.Attr) {
			continue
		}
		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			return []CDCEvent{
				{
					EventType: "DROP",
					Event: &DropEvent{
						Operation: "PREDICATE",
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
			EventType: "MUTATION",
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

func skipAttribute(attr string) bool {
	if strings.HasPrefix(attr, "dgraph") {
		return true
	}
	return false
}
