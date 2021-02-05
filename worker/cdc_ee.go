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
	"go.etcd.io/etcd/raft/raftpb"
)

const defaultCDCConfig = "enabled=false; max_recovery=10000; file=; kafka=; sasl_user=; sasl_password=; ca_cert=; client_cert=; client_key="

type CDC struct {
	sink               SinkHandler
	index              uint64
	maxRecoveryEntries uint64
	closer             *z.Closer
	pendingTxnEvents   map[uint64][]CDCEvent
	// sentTs is the timestamp till which we have send the event of txns.
	// There will be no event below this timestamp for which we need to send the events
	sentTs uint64
	// maxSentTs is the maximum timestamp till which we have sent the events of txns.
	// this is helpful to maintain the state of the CDC till which we can clear the raft logs
	maxSentTs uint64
}

func newCDC() *CDC {
	if Config.ChangeDataConf == "" {
		return nil
	}

	cdcFlag := x.NewSuperFlag(Config.ChangeDataConf).MergeAndCheckDefault(defaultCDCConfig)
	sink, err := GetSinkHandler(cdcFlag)
	x.Check(err)
	cdc := &CDC{
		sink:               sink,
		maxRecoveryEntries: cdcFlag.GetUint64("max-recovery"),
		closer:             z.NewCloser(1),
		pendingTxnEvents:   make(map[uint64][]CDCEvent),
	}
	return cdc
}

func (cdc *CDC) getTs() uint64 {
	if cdc == nil {
		return math.MaxUint64
	}
	return atomic.LoadUint64(&cdc.sentTs)
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
	atomic.CompareAndSwapUint64(&cdc.sentTs, ts, newTs)
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

	sendEvents := func(ts *pb.TxnStatus, pending []CDCEvent) error {
		var commitTs uint64 = 0
		if ts != nil {
			commitTs = ts.CommitTs
		}
		msgs := make([]SinkMessage, len(pending))
		for i, e := range pending {
			// add commit timestamp here
			e.Meta.CommitTimestamp = commitTs
			b, err := json.Marshal(e)
			x.Check(err)
			// todo(aman bansal): use namespace for key.
			msgs[i] = SinkMessage{
				Meta: SinkMeta{
					Topic: "dgraph-cdc",
				},
				Key:   []byte("dgraph-cdc-event"),
				Value: b,
			}
		}
		if err := cdc.sink.SendMessages(msgs); err != nil {
			glog.Errorf("error while sending cdc event to sink %+v", err)
			return err
		}
		return nil
	}

	// This will always run on leader node only.
	// Leader will check the Raft logs and keep in memory events that are pending.
	// Once Txn is done, it will try to send events to sink.
	// index helps to define from which point we need to start reading from the raft logs
	// clear map whenever we have send the events
	checkAndSendCDCEvents := func() {
		first, err := groups().Node.Store.FirstIndex()
		x.Check(err)
		cdcIndex := x.Max(cdc.index, first)
		last := groups().Node.Applied.DoneUntil()
		if cdcIndex == last {
			return
		}
		// if cdc is lagging behind the current via maxRecoveryEntries,
		// skip ahead the index to prevent uncontrolled growth of raft logs.
		if uint64(len(cdc.pendingTxnEvents)) > cdc.maxRecoveryEntries {
			glog.Info("too many pending cdc events. Skipping for now.")
			cdc.updateTs(posting.Oracle().MaxAssigned())
			cdc.index = last
			cdc.pendingTxnEvents = make(map[uint64][]CDCEvent)
			return
		}
		for batchFirst := cdcIndex; batchFirst <= last; {
			entries, err := groups().Node.Store.Entries(batchFirst, last+1, 256<<20)
			x.Check(err)
			// Exit early from the loop if no entries were found.
			if len(entries) == 0 {
				break
			}

			batchFirst = entries[len(entries)-1].Index + 1
			for _, entry := range entries {
				if entry.Type != raftpb.EntryNormal || len(entry.Data) == 0 {
					continue
				}

				var proposal pb.Proposal
				if err := proposal.Unmarshal(entry.Data[8:]); err != nil {
					glog.Errorf("CDC: not able to marshal the proposal %v", err)
					continue
				}
				// this is to ensure that cdcMinReadTs will be monotonically increasing
				// In this way no min pending txn in case we skip some entries can affect
				// the sentTs to decrease. This way we will be able to provide guarantees
				// across the cluster in case of failures.
				if proposal.Mutations != nil && proposal.Mutations.StartTs > cdc.getTs() {
					events := toCDCEvent(entry.Index, proposal.Mutations)
					if len(events) == 0 {
						continue
					}
					// In ludicrous events send the events as soon as you get it.
					// We wont wait for oracle delta in case of ludicrous mode.
					// Since all mutations will eventually succeed.
					// We can set the read ts here only.
					if x.WorkerConfig.LudicrousMode {
						if err := sendEvents(nil, events); err != nil {
							glog.Errorf("not able to send messages to the sink %v", err)
							return
						}
						cdc.index = entry.Index
						cdc.updateTs(proposal.Mutations.StartTs)
						continue
					}
					cdc.pendingTxnEvents[proposal.Mutations.StartTs] =
						append(cdc.pendingTxnEvents[proposal.Mutations.StartTs], events...)
				}

				if proposal.Delta != nil {
					for _, ts := range proposal.Delta.Txns {
						pending := cdc.pendingTxnEvents[ts.StartTs]
						if ts.CommitTs > 0 && len(pending) > 0 {
							if err := sendEvents(ts, pending); err != nil {
								glog.Errorf("not able to send messages to the sink %v", err)
								return
							}
						}
						// delete from pending events once events are sent
						delete(cdc.pendingTxnEvents, ts.StartTs)
						cdc.maxSentTs = x.Max(cdc.maxSentTs, ts.StartTs)
						cdc.evaluateAndSetTs()
					}
				}
				cdc.index = entry.Index
			}
		}
		return
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
	CDCIndex        uint64 `json:"cdc_index"`
	ReadTimestamp   uint64 `json:"read_timestamp"`
	CommitTimestamp uint64 `json:"commit_timestamp"`
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
					CDCIndex: index,
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
						CDCIndex: index,
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
				CDCIndex:      index,
				ReadTimestamp: mutation.StartTs,
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
