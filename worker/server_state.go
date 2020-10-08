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

package worker

import (
	"context"
	"math"
	"os"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

// ServerState holds the state of the Dgraph server.
type ServerState struct {
	FinishCh chan struct{} // channel to wait for all pending reqs to finish.

	Pstore   *badger.DB
	WALstore *raftwal.DiskStorage
	gcCloser *z.Closer // closer for valueLogGC

	needTs chan tsReq
}

// State is the instance of ServerState used by the current server.
var State ServerState

// InitServerState initializes this server's state.
func InitServerState() {
	Config.validate()

	State.FinishCh = make(chan struct{})
	State.needTs = make(chan tsReq, 100)

	State.initStorage()
	go State.fillTimestampRequests()

	groupId, err := x.ReadGroupIdFile(Config.PostingDir)
	if err != nil {
		glog.Warningf("Could not read %s file inside posting directory %s.", x.GroupIdFileName,
			Config.PostingDir)
	}
	x.WorkerConfig.ProposedGroupId = groupId
}

func setBadgerOptions(opt badger.Options) badger.Options {
	opt = opt.WithSyncWrites(false).
		WithLogger(&x.ToGlog{}).
		WithEncryptionKey(x.WorkerConfig.EncryptionKey)

	// Disable conflict detection in badger. Alpha runs in managed mode and
	// perform its own conflict detection so we don't need badger's conflict
	// detection. Using badger's conflict detection uses memory which can be
	// saved by disabling it.
	opt.DetectConflicts = false

	glog.Infof("Setting Posting Dir Compression Level: %d", Config.PostingDirCompressionLevel)
	opt.Compression = Config.PostingDirCompression
	opt.ZSTDCompressionLevel = Config.PostingDirCompressionLevel

	// Settings for the data directory.
	return opt
}

func (s *ServerState) initStorage() {
	var err error

	if x.WorkerConfig.EncryptionKey != nil {
		// non-nil key file
		if !EnterpriseEnabled() {
			// not licensed --> crash.
			glog.Fatal("Valid Enterprise License needed for the Encryption feature.")
		} else {
			// licensed --> OK.
			glog.Infof("Encryption feature enabled.")
		}
	}

	{
		// Write Ahead Log directory
		x.Checkf(os.MkdirAll(Config.WALDir, 0700), "Error while creating WAL dir.")
		s.WALstore = raftwal.Init(Config.WALDir)
		// TODO: Add encryption back to WALStore.
	}
	{
		// Postings directory
		// All the writes to posting store should be synchronous. We use batched writers
		// for posting lists, so the cost of sync writes is amortized.
		x.Check(os.MkdirAll(Config.PostingDir, 0700))
		opt := badger.DefaultOptions(Config.PostingDir).
			WithValueThreshold(1 << 10 /* 1KB */).
			WithNumVersionsToKeep(math.MaxInt32).
			WithBlockCacheSize(Config.PBlockCacheSize).
			WithIndexCacheSize(Config.PIndexCacheSize)
		opt = setBadgerOptions(opt)

		// Print the options w/o exposing key.
		// TODO: Build a stringify interface in Badger options, which is used to print nicely here.
		key := opt.EncryptionKey
		opt.EncryptionKey = nil
		glog.Infof("Opening postings BadgerDB with options: %+v\n", opt)
		opt.EncryptionKey = key

		s.Pstore, err = badger.OpenManaged(opt)
		x.Checkf(err, "Error while creating badger KV posting store")

		// zero out from memory
		opt.EncryptionKey = nil
	}

	s.gcCloser = z.NewCloser(2)
	go x.RunVlogGC(s.Pstore, s.gcCloser)
	// Commenting this out because Badger is doing its own cache checks.
	go x.MonitorCacheHealth(s.Pstore, s.gcCloser)
}

// Dispose stops and closes all the resources inside the server state.
func (s *ServerState) Dispose() {
	s.gcCloser.SignalAndWait()
	if err := s.Pstore.Close(); err != nil {
		glog.Errorf("Error while closing postings store: %v", err)
	}
	if err := s.WALstore.Close(); err != nil {
		glog.Errorf("Error while closing WAL store: %v", err)
	}
}

func (s *ServerState) GetTimestamp(readOnly bool) uint64 {
	tr := tsReq{readOnly: readOnly, ch: make(chan uint64)}
	s.needTs <- tr
	return <-tr.ch
}

func (s *ServerState) fillTimestampRequests() {
	const (
		initDelay = 10 * time.Millisecond
		maxDelay  = time.Second
	)

	defer func() {
		glog.Infoln("Exiting fillTimestampRequests")
	}()

	var reqs []tsReq
	for {
		// Reset variables.
		reqs = reqs[:0]
		delay := initDelay

		select {
		case <-s.gcCloser.HasBeenClosed():
			return
		case req := <-s.needTs:
		slurpLoop:
			for {
				reqs = append(reqs, req)
				select {
				case req = <-s.needTs:
				default:
					break slurpLoop
				}
			}
		}

		// Generate the request.
		num := &pb.Num{}
		for _, r := range reqs {
			if r.readOnly {
				num.ReadOnly = true
			} else {
				num.Val++
			}
		}

		// Execute the request with infinite retries.
	retry:
		if s.gcCloser.Ctx().Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.gcCloser.Ctx(), 10*time.Second)
		ts, err := Timestamps(ctx, num)
		cancel()
		if err != nil {
			glog.Warningf("Error while retrieving timestamps: %v with delay: %v."+
				" Will retry...\n", err, delay)
			time.Sleep(delay)
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
			goto retry
		}
		var offset uint64
		for _, req := range reqs {
			if req.readOnly {
				req.ch <- ts.ReadOnly
			} else {
				req.ch <- ts.StartId + offset
				offset++
			}
		}
		x.AssertTrue(ts.StartId == 0 || ts.StartId+offset-1 == ts.EndId)
	}
}

type tsReq struct {
	readOnly bool
	// A one-shot chan which we can send a txn timestamp upon.
	ch chan uint64
}
