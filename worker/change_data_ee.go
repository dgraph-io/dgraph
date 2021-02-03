// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/ristretto/z"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/types"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/protos/pb"
	"go.etcd.io/etcd/raft/raftpb"

	"github.com/dgraph-io/dgraph/x"
)

const defaultCDCConfig = "enabled=false; max_recovery=10000"

// TODO: see if we need to send some monitoring events or not
type ChangeData struct {
	sink               SinkHandler
	cdcIndex           uint64
	maxRecoveryEntries uint64
	closer             *z.Closer
	pendingEvents      map[uint64][]CDCEvent
	// maximum commit timestamp till which we have send the CDC event
	maxCommitTs      uint64
	lastProposeMaxTs uint64
}

func initChangeDataCapture(idx uint64) *ChangeData {
	if Config.ChangeDataConf == "" {
		return nil
	}

	cdcFlag := x.NewSuperFlag(Config.ChangeDataConf).MergeAndCheckDefault(defaultCDCConfig)
	if !cdcFlag.GetBool("enabled") {
		return nil
	}
	sink, err := GetSinkHandler()
	x.Check(err)
	cd := &ChangeData{
		sink:               sink,
		cdcIndex:           idx,
		maxRecoveryEntries: cdcFlag.GetUint64("max-recovery"),
		closer:             z.NewCloser(1),
		pendingEvents:      make(map[uint64][]CDCEvent),
	}
	return cd
}

func (cd *ChangeData) getCDCMaxTs() uint64 {
	if cd == nil {
		// return max value that will help callers to nullify cdc effect if not enabled.
		return math.MaxUint64
	}
	return atomic.LoadUint64(&cd.maxCommitTs)
}

func (cd *ChangeData) updateCDCMaxTs(ts uint64) {
	if cd == nil {
		return
	}
	atomic.StoreUint64(&cd.maxCommitTs, ts)
}

func (cd *ChangeData) proposeCDCMaxCommitTs() error {
	if cd == nil {
		return nil
	}

	maxTs := atomic.LoadUint64(&cd.maxCommitTs)
	if cd.lastProposeMaxTs < maxTs {
		err := groups().Node.proposeCDCMaxCommitTs(maxTs)
		if err != nil {
			return err
		}
		cd.lastProposeMaxTs = maxTs
	}
	return nil
}

func (cd *ChangeData) Close() {
	if cd == nil {
		return
	}
	glog.Infof("closing CDC events...")
	cd.closer.SignalAndWait()
	err := cd.sink.Close()
	glog.Errorf("error while closing sink %v", err)
}

// todo: test cases old cluster restart, live loader, bulk loader, backup restore etc
func (cd *ChangeData) processCDCEvents() {
	if cd == nil {
		return
	}

	sendCDCEvents := func() {
		first, err := groups().Node.Store.FirstIndex()
		x.Check(err)

		cdcIndex := x.Max(cd.cdcIndex, first)
		last := groups().Node.Applied.DoneUntil()
		if cdcIndex == last {
			return
		}
		// if cdc is lagging behind the current raft index,
		// skip ahead the cdcIndex to prevent uncontrolled growth of raft logs.
		//if last-cdcIndex > cd.maxRecoveryEntries {
		//	cd.UpdateCDCIndex(last)
		//	return
		//}
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
					cd.cdcIndex = entry.Index
					continue
				}

				var proposal pb.Proposal
				if err := proposal.Unmarshal(entry.Data[8:]); err != nil {
					glog.Errorf("CDC: not able to marshal the proposal %v", err)
					cd.cdcIndex = entry.Index
					continue
				}
				// todo(aman bansal): use namespace for key.
				if proposal.Mutations != nil {
					events := transformMutationToCDCEvent(entry.Index, proposal.Mutations)
					if cd.pendingEvents[proposal.Mutations.StartTs] == nil {
						cd.pendingEvents[proposal.Mutations.StartTs] = make([]CDCEvent, 0)
					}
					cd.pendingEvents[proposal.Mutations.StartTs] = append(cd.pendingEvents[proposal.Mutations.StartTs], events...)
				}

				if proposal.Delta != nil {
					for _, ts := range proposal.Delta.Txns {
						pending := cd.pendingEvents[ts.StartTs]
						if ts.CommitTs > 0 && len(pending) > 0 {
							msgs := make([]SinkMessage, len(pending))
							for i, e := range pending {
								// add commit timestamp here
								e.Meta.Timestamp = ts.CommitTs
								b, err := json.Marshal(e)
								x.Check(err)
								msgs[i] = SinkMessage{
									Meta: SinkMeta{
										Topic: "dgraph_cdc",
									},
									Key:   []byte("dgraph-cdc-event"),
									Value: b,
								}
							}
							if err := cd.sink.SendMessages(msgs); err != nil {
								glog.Errorf("error while sending cdc event to sink %+v", err)
								return
							}
							// delete from pending events once events are sent
							delete(cd.pendingEvents, ts.StartTs)
							if cd.maxCommitTs < ts.CommitTs {
								cd.maxCommitTs = ts.CommitTs
							}
						} else {
							delete(cd.pendingEvents, ts.StartTs)
						}
					}
				}

				cd.cdcIndex = entry.Index
			}
		}
		return
	}

	tick := time.NewTicker(time.Second)
	defer cd.closer.Done()
	defer tick.Stop()
	iter := 0
	for {
		select {
		case <-cd.closer.HasBeenClosed():
			return
		case <-tick.C:
			if groups().Node.AmLeader() && EnterpriseEnabled() {
				sendCDCEvents()
				iter = iter + 1
				if iter == 5 {
					iter = 0
					if err := cd.proposeCDCMaxCommitTs(); err != nil {
						glog.Errorf("not able to propose snapshot %v", err)
					}
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
	CDCIndex  uint64 `json:"cdc_index"`
	Timestamp uint64 `json:"timestamp"`
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

func transformMutationToCDCEvent(index uint64, mutation *pb.Mutations) []CDCEvent {
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
		// todo (aman bansal): send password fields encrypted
		cdcEvents = append(cdcEvents, CDCEvent{
			Meta: &EventMeta{
				CDCIndex:  index,
				Timestamp: mutation.StartTs,
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
