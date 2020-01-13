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
	"math"
	"os"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/badger/v2/options"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

// ServerState holds the state of the Dgraph server.
type ServerState struct {
	FinishCh   chan struct{} // channel to wait for all pending reqs to finish.
	ShutdownCh chan struct{} // channel to signal shutdown.

	Pstore   *badger.DB
	WALstore *badger.DB

	vlogTicker          *time.Ticker // runs every 1m, check size of vlog and run GC conditionally.
	mandatoryVlogTicker *time.Ticker // runs every 10m, we always run vlog GC.

	needTs chan tsReq
}

// State is the instance of ServerState used by the current server.
var State ServerState

// InitServerState initializes this server's state.
func InitServerState() {
	Config.validate()

	State.FinishCh = make(chan struct{})
	State.ShutdownCh = make(chan struct{})
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

func (s *ServerState) runVlogGC(store *badger.DB) {
	// Get initial size on start.
	_, lastVlogSize := store.Size()
	const GB = int64(1 << 30)

	runGC := func() {
		var err error
		for err == nil {
			// If a GC is successful, immediately run it again.
			err = store.RunValueLogGC(0.7)
		}
		_, lastVlogSize = store.Size()
	}

	for {
		select {
		case <-s.vlogTicker.C:
			_, currentVlogSize := store.Size()
			if currentVlogSize < lastVlogSize+GB {
				continue
			}
			runGC()
		case <-s.mandatoryVlogTicker.C:
			runGC()
		}
	}
}

func setBadgerOptions(opt badger.Options) badger.Options {
	opt = opt.WithSyncWrites(false).WithTruncate(true).WithLogger(&x.ToGlog{}).
		WithEncryptionKey(enc.ReadEncryptionKeyFile(Config.BadgerKeyFile))

	glog.Infof("Setting Badger table load option: %s", Config.BadgerTables)
	switch Config.BadgerTables {
	case "mmap":
		opt.TableLoadingMode = options.MemoryMap
	case "ram":
		opt.TableLoadingMode = options.LoadToRAM
	case "disk":
		opt.TableLoadingMode = options.FileIO
	default:
		x.Fatalf("Invalid Badger Tables options")
	}

	glog.Infof("Setting Badger value log load option: %s", Config.BadgerVlog)
	switch Config.BadgerVlog {
	case "mmap":
		opt.ValueLogLoadingMode = options.MemoryMap
	case "disk":
		opt.ValueLogLoadingMode = options.FileIO
	default:
		x.Fatalf("Invalid Badger Value log options")
	}
	return opt
}

func (s *ServerState) initStorage() {
	var err error

	if Config.BadgerKeyFile != "" {
		// non-nil key file
		if !EnterpriseEnabled() {
			// not licensed --> crash.
			glog.Fatal("Valid Enterprise License needed for the Encryption feature.")
		} else {
			// licensed --> OK.
			glog.Infof("Encryption feature enabled. Using encryption key file: %v", Config.BadgerKeyFile)
		}
	}

	{
		// Write Ahead Log directory
		x.Checkf(os.MkdirAll(Config.WALDir, 0700), "Error while creating WAL dir.")
		opt := badger.LSMOnlyOptions(Config.WALDir)
		opt = setBadgerOptions(opt)
		opt.ValueLogMaxEntries = 10000 // Allow for easy space reclamation.
		opt.MaxCacheSize = 10 << 20    // 10 mb of cache size for WAL.

		// We should always force load LSM tables to memory, disregarding user settings, because
		// Raft.Advance hits the WAL many times. If the tables are not in memory, retrieval slows
		// down way too much, causing cluster membership issues. Because of prefix compression and
		// value separation provided by Badger, this is still better than using the memory based WAL
		// storage provided by the Raft library.
		opt.TableLoadingMode = options.LoadToRAM

		// Print the options w/o exposing key.
		// TODO: Build a stringify interface in Badger options, which is used to print nicely here.
		key := opt.EncryptionKey
		opt.EncryptionKey = nil
		glog.Infof("Opening write-ahead log BadgerDB with options: %+v\n", opt)
		opt.EncryptionKey = key
		opt.EventLogging = false
		opt.Compression = 0

		s.WALstore, err = badger.Open(opt)
		x.Checkf(err, "Error while creating badger KV WAL store")
	}
	{
		// Postings directory
		// All the writes to posting store should be synchronous. We use batched writers
		// for posting lists, so the cost of sync writes is amortized.
		x.Check(os.MkdirAll(Config.PostingDir, 0700))
		opt := badger.DefaultOptions(Config.PostingDir).WithValueThreshold(1 << 10 /* 1KB */).
			WithNumVersionsToKeep(math.MaxInt32).WithMaxCacheSize(1 << 30)
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

	s.vlogTicker = time.NewTicker(1 * time.Minute)
	s.mandatoryVlogTicker = time.NewTicker(10 * time.Minute)
	go s.runVlogGC(s.Pstore)
	go s.runVlogGC(s.WALstore)
}

// Dispose stops and closes all the resources inside the server state.
func (s *ServerState) Dispose() {
	if err := s.Pstore.Close(); err != nil {
		glog.Errorf("Error while closing postings store: %v", err)
	}
	if err := s.WALstore.Close(); err != nil {
		glog.Errorf("Error while closing WAL store: %v", err)
	}
	s.vlogTicker.Stop()
	s.mandatoryVlogTicker.Stop()
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

	var reqs []tsReq
	for {
		// Reset variables.
		reqs = reqs[:0]
		delay := initDelay

		req := <-s.needTs
	slurpLoop:
		for {
			reqs = append(reqs, req)
			select {
			case req = <-s.needTs:
			default:
				break slurpLoop
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
