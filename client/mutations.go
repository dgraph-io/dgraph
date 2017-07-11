/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	ErrConnected   = errors.New("Edge already connected to another node.")
	ErrValue       = errors.New("Edge already has a value.")
	ErrEmptyXid    = errors.New("Empty XID node.")
	ErrInvalidType = errors.New("Invalid value type")
	ErrEmptyVar    = errors.New("Empty variable name.")
	emptyEdge      Edge
)

var DefaultOptions = BatchMutationOptions{
	Size:          100,
	Pending:       100,
	PrintCounters: false,
}

type BatchMutationOptions struct {
	Size          int
	Pending       int
	PrintCounters bool
}

type allocator struct {
	x.SafeMutex

	dc protos.DgraphClient

	kv  *badger.KV
	ids *Cache

	syncCh chan entry

	startId uint64
	endId   uint64
}

// TODO: Add for n later
func (a *allocator) fetchOne() (uint64, error) {
	a.AssertLock()
	if a.startId == 0 || a.endId < a.startId {
		factor := time.Second
		for {
			assignedIds, err := a.dc.AssignUids(context.Background(), &protos.Num{Val: 1000})
			if err != nil {
				time.Sleep(factor)
				if factor < 256*time.Second {
					factor = factor * 2
				}
			} else {
				a.startId = assignedIds.StartId
				a.endId = assignedIds.EndId
				break
			}
		}
	}

	uid := a.startId
	a.startId++
	return uid, nil
}

func (a *allocator) getFromKV(id string) (uint64, error) {
	var item badger.KVItem
	if err := a.kv.Get([]byte(id), &item); err != nil {
		return 0, err
	}
	val := item.Value()
	if len(val) > 0 {
		uid, n := binary.Uvarint(val)
		if n <= 0 {
			return 0, fmt.Errorf("Unable to parse val %q to uint64 for %q", val, id)
		}
		return uid, nil
	}
	return 0, nil
}

func (a *allocator) assignOrGet(id string) (uid uint64, isNew bool, err error) {
	a.Lock()
	defer a.Unlock()
	uid, _ = a.ids.Get(id)
	if uid > 0 {
		return
	}

	// check in kv
	// get in outside lock and do put if missing ???
	uid, err = a.getFromKV(id)
	if err != nil {
		return
	}
	// if not found in kv
	if uid == 0 {
		// assign one
		uid, err = a.fetchOne()
		if err != nil {
			return
		}
		// Entry would be comitted to disc by the time it's evicted
		// TODO: Better to delete after it's persisted, can cause race
		// may be persist it during eviction and delete after it's synced
		// to disk
		a.syncCh <- entry{key: id, value: uid}
	}
	a.ids.Add(id, uid)
	isNew = true
	err = nil
	return
}

// Counter keeps a track of various parameters about a batch mutation.
type Counter struct {
	// Number of RDF's processed by server.
	Rdfs uint64
	// Number of mutations processed by the server.
	Mutations uint64
	// Time elapsed sinze the batch started.
	Elapsed time.Duration
}

type Dgraph struct {
	opts BatchMutationOptions

	schema chan protos.SchemaUpdate
	nquads chan nquadOp
	dc     []protos.DgraphClient
	wg     sync.WaitGroup
	alloc  *allocator
	ticker *time.Ticker

	// Miscellaneous information to print counters.
	// Num of RDF's sent
	rdfs uint64
	// Num of mutations sent
	mutations uint64
	// To get time elapsed.
	start time.Time

	marks *syncMarks
}

func NewDgraphClient(conns []*grpc.ClientConn, opts BatchMutationOptions, clientDir string) *Dgraph {
	var clients []protos.DgraphClient
	for _, conn := range conns {
		client := protos.NewDgraphClient(conn)
		clients = append(clients, client)
	}
	return NewClient(clients, opts, clientDir)
}

func (d *Dgraph) printDoneUntil() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		fmt.Println(d.marks.Get("21million.rdf.gz").DoneUntil())
	}
}

// TODO(tzdybal) - hide this function from users
func NewClient(clients []protos.DgraphClient, opts BatchMutationOptions, clientDir string) *Dgraph {
	x.Check(os.MkdirAll(clientDir, 0700))
	opt := badger.DefaultOptions
	opt.SyncWrites = false
	opt.MapTablesTo = table.MemoryMap
	opt.Dir = clientDir
	opt.ValueDir = clientDir

	kv, err := badger.NewKV(&opt)
	x.Checkf(err, "Error while creating badger KV posting store")

	alloc := &allocator{
		dc:     clients[0],
		ids:    NewCache(100000),
		kv:     kv,
		syncCh: make(chan entry, 10000),
	}

	d := &Dgraph{
		opts:   opts,
		dc:     clients,
		start:  time.Now(),
		nquads: make(chan nquadOp, opts.Pending*opts.Size),
		schema: make(chan protos.SchemaUpdate, opts.Pending*opts.Size),
		alloc:  alloc,
		marks:  new(syncMarks),
	}

	for i := 0; i < opts.Pending; i++ {
		d.wg.Add(1)
		go d.makeRequests()
	}
	d.wg.Add(1)
	go d.makeSchemaRequests()

	rand.Seed(time.Now().Unix())
	if opts.PrintCounters {
		go d.printCounters()
	}
	go d.batchSync()
	go d.printDoneUntil()
	return d
}

func (d *Dgraph) batchSync() {
	var entries []entry
	var loop uint64
	wb := make([]*badger.Entry, 0, 1000)

	for {
		ent := <-d.alloc.syncCh
	slurpLoop:
		for {
			entries = append(entries, ent)
			if len(entries) == 1000 {
				// Avoid making infinite batch, push back against syncCh.
				break
			}
			select {
			case ent = <-d.alloc.syncCh:
			default:
				break slurpLoop
			}
		}
		loop++
		//fmt.Printf("Writing batch of size: %v\n", len(entries))

		for _, e := range entries {
			// Atmost 10 bytes are needed for uvarint encoding
			buf := make([]byte, 10)
			n := binary.PutUvarint(buf[:], e.value)
			wb = badger.EntriesSet(wb, []byte(e.key), buf[:n])
		}
		if err := d.alloc.kv.BatchSet(wb); err != nil {
			fmt.Printf("Error while writing to disc %v\n", err)
		}
		for _, wbe := range wb {
			if err := wbe.Error; err != nil {
				fmt.Printf("Error while writing to disc %v\n", err)
			}
		}
		wb = wb[:0]
		entries = entries[:0]
	}
}

func (d *Dgraph) printCounters() {
	d.ticker = time.NewTicker(2 * time.Second)
	start := time.Now()

	for range d.ticker.C {
		counter := d.Counter()
		rate := float64(counter.Rdfs) / counter.Elapsed.Seconds()
		elapsed := ((time.Since(start) / time.Second) * time.Second).String()
		fmt.Printf("[Request: %6d] Total RDFs done: %8d RDFs per second: %7.0f Time Elapsed: %v \r",
			counter.Mutations, counter.Rdfs, rate, elapsed)

	}
}

func (d *Dgraph) request(req *Req) {
	counter := atomic.AddUint64(&d.mutations, 1)
	factor := time.Second
RETRY:
	_, err := d.dc[rand.Intn(len(d.dc))].Run(context.Background(), &req.gr)
	if err != nil {
		errString := err.Error()
		// Irrecoverable
		if strings.Contains(errString, "x509") || grpc.Code(err) == codes.Internal {
			log.Fatal(errString)
		}
		fmt.Printf("Retrying req: %d. Error: %v\n", counter, errString)
		time.Sleep(factor)
		if factor < 256*time.Second {
			factor = factor * 2
		}
		goto RETRY
	}

	for _, m := range req.rm {
		d.marks.Get(m.file).Ch <- x.Mark{Index: m.line, Done: true}
	}
	req.rm = req.rm[:0]
	req.reset()
}

func (d *Dgraph) makeRequests() {
	req := new(Req)

	for n := range d.nquads {
		if n.op == SET {
			req.Set(n.e)
			req.rm = append(req.rm, rdfMeta{n.e.file, n.e.line})
		} else if n.op == DEL {
			req.Delete(n.e)
		}
		if req.size() == d.opts.Size {
			d.request(req)
		}
	}

	if req.size() > 0 {
		d.request(req)
	}
	d.wg.Done()
}

func (d *Dgraph) makeSchemaRequests() {
	req := new(Req)
LOOP:
	for {
		select {
		case s, ok := <-d.schema:
			if !ok {
				break LOOP
			}
			req.addSchema(s)
		default:
			start := time.Now()
			if req.size() > 0 {
				d.request(req)
			}
			elapsedMillis := time.Since(start).Seconds() * 1e3
			if elapsedMillis < 10 {
				time.Sleep(time.Duration(int64(10-elapsedMillis)) * time.Millisecond)
			}
		}
	}

	if req.size() > 0 {
		d.request(req)
	}
	d.wg.Done()
}

func (d *Dgraph) BatchSet(e Edge) error {
	d.nquads <- nquadOp{
		e:  e,
		op: SET,
	}
	if len(e.file) > 0 && e.line != 0 {
		d.SyncMarkFor(e.file).Ch <- x.Mark{Index: e.line}
	}
	atomic.AddUint64(&d.rdfs, 1)
	return nil
}

func (d *Dgraph) BatchDelete(e Edge) error {
	d.nquads <- nquadOp{
		e:  e,
		op: DEL,
	}
	atomic.AddUint64(&d.rdfs, 1)
	return nil
}

func (d *Dgraph) AddSchema(s protos.SchemaUpdate) error {
	if err := checkSchema(s); err != nil {
		return err
	}
	d.schema <- s
	return nil
}

// Flush waits for all pending requests to complete. It should always be called
// after adding all the NQuads using batch.AddMutation().
func (d *Dgraph) BatchFlush() {
	close(d.nquads)
	close(d.schema)
	d.wg.Wait()
	if d.ticker != nil {
		d.ticker.Stop()
	}
}

func (d *Dgraph) Run(ctx context.Context, req *Req) (*protos.Response, error) {
	return d.dc[rand.Intn(len(d.dc))].Run(ctx, &req.gr)
}

// Counter returns the current state of the BatchMutation.
func (d *Dgraph) Counter() Counter {
	return Counter{
		Rdfs:      atomic.LoadUint64(&d.rdfs),
		Mutations: atomic.LoadUint64(&d.mutations),
		Elapsed:   time.Since(d.start),
	}
}

func (d *Dgraph) CheckVersion(ctx context.Context) {
	v, err := d.dc[rand.Intn(len(d.dc))].CheckVersion(ctx, &protos.Check{})
	if err != nil {
		fmt.Printf(`Could not fetch version information from Dgraph. Got err: %v.`, err)
	} else {
		version := x.Version()
		if version != "" && v.Tag != "" && version != v.Tag {
			fmt.Printf(`
Dgraph server: %v, loader: %v dont match.
You can get the latest version from https://docs.dgraph.io
`, v.Tag, version)
		}
	}
}

func (d *Dgraph) NodeUid(uid uint64) Node {
	return Node{uid: uid}
}

func (d *Dgraph) NodeBlank(varname string) (Node, error) {
	if len(varname) == 0 {
		d.alloc.Lock()
		defer d.alloc.Unlock()
		uid, err := d.alloc.fetchOne()
		if err != nil {
			return Node{}, err
		}
		return Node{uid: uid}, nil
	}
	uid, _, err := d.alloc.assignOrGet("_:" + varname)
	if err != nil {
		return Node{}, err
	}
	return Node{uid: uid}, nil
}

func (d *Dgraph) NodeXid(xid string, storeXid bool) (Node, error) {
	if len(xid) == 0 {
		return Node{}, ErrEmptyXid
	}
	uid, isNew, err := d.alloc.assignOrGet(xid)
	if err != nil {
		return Node{}, err
	}
	n := Node{uid: uid}
	if storeXid && isNew {
		e := n.Edge("_xid_")
		x.Check(e.SetValueString(xid))
		d.BatchSet(e)
	}
	return n, nil
}

func (d *Dgraph) NodeUidVar(name string) (Node, error) {
	if len(name) == 0 {
		return Node{}, ErrEmptyVar
	}

	return Node{varName: name}, nil
}
