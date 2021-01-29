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
	"encoding/binary"
	"encoding/json"
	"math"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/protos/pb"
	"go.etcd.io/etcd/raft/raftpb"

	"github.com/dgraph-io/dgraph/x"
)

// TODO: (aman bansal) verify things if cdc is not enabled
// TODO: (aman bansal) see if we can send some monitoring events or not
type ChangeData struct {
	sink     SinkHandler
	cdcIndex uint64
}

// If Enterprise is not enabled return
// Todo: (aman bansal) Make the Sink Configurable
func initChangeDataCapture(idx uint64) *ChangeData {
	if !EnterpriseEnabled() {
		return nil
	}
	path, err := filepath.Abs(filepath.Join("cdc", "cdc.event.log"))
	x.Check(err)
	sink, err := NewFileBasedSink(path)
	x.Check(err)
	cd := &ChangeData{
		sink:     sink,
		cdcIndex: idx,
	}
	return cd
}

// if cdc is not enabled return the max possible value
// This is done so that it will not effect with the default behaviour
func (cd *ChangeData) getCDCIndex() uint64 {
	if cd == nil {
		return math.MaxUint64
	}
	return atomic.LoadUint64(&cd.cdcIndex)
}

func (cd *ChangeData) UpdateCDCIndex(idx uint64) {
	if cd == nil {
		return
	}
	atomic.StoreUint64(&cd.cdcIndex, idx)
}

// 1. Old cluster start (data already has ) -> // ask kafka gives you 0.
// this is solved with snpshtIdx
// 2. Live loader
func (cd *ChangeData) processCDCEvents() {
	if cd == nil {
		return
	}

	sendCDCEvents := func() {
		cdcIndex := cd.getCDCIndex() + 1
		first, err := groups().Node.Store.FirstIndex()
		x.Check(err)
		if cdcIndex < first {
			glog.Error("there is mismatch in cdc index and snapshot index, " +
				"we might have missed some events.")
			cdcIndex = first
		}

		last := groups().Node.Applied.DoneUntil()
		if cdcIndex == last {
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
					x.Check(err)
				}

				if proposal.Mutations != nil {
					b, _ := json.Marshal(proposal.Mutations)
					if proposal.Mutations.Edges != nil {
						for _, r := range proposal.Mutations.Edges {
							if r.ValueType == 2 {
								u := binary.LittleEndian.Uint64(r.Value)
								b, _ = json.Marshal(u)
							}

							//p := types.Val{Tid: types.BinaryID, Value: r.Value}
							//p1 := types.ValueForType(types.TypeID(r.ValueType))
							//err := types.Marshal(p, &p1)
							//fmt.Println("type marshal", err)
						}
					}

					err := cd.sink.SendMessage(nil, b)
					x.Check(err)
				}
				cd.UpdateCDCIndex(entry.Index)
			}
			if err := groups().Node.proposeSnapshot(0); err != nil {
				glog.Errorf("not able to propose snapshot %v", err)
			}
		}
	}

	for {
		if groups().Node.AmLeader() {
			sendCDCEvents()
		}
		time.Sleep(time.Second)
	}
}
