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

	"github.com/dgraph-io/badger/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	ErrConnected   = errors.New("Edge already connected to another node.")
	ErrValue       = errors.New("Edge already has a value.")
	ErrEmptyXid    = errors.New("Empty XID node.")
	ErrInvalidType = errors.New("Invalid value type")
	emptyEdge      Edge
)

type BatchMutationOptions struct {
	Size          int
	Pending       int
	PrintCounters bool
}

type allocator struct {
	x.SafeMutex

	kv *badger.KV
	dc protos.DgraphClient

	ids map[string]uint64

	startId uint64
	endId   uint64
}

func (a *allocator) getFromCache(id string) (uint64, error) {
	a.RLock()
	if uid, found := a.ids[id]; found && uid > 0 {
		a.RUnlock()
		return uid, nil
	}
	a.RUnlock()

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

func (a *allocator) assingOrGet(id string) (uint64, error) {
	uid, err := a.getFromCache(id)
	if err != nil || uid > 0 {
		return uid, err
	}

	uid, err = a.getFromCache(id)
	if err != nil || uid > 0 {
		return uid, err
	}

	a.Lock()
	defer a.Unlock()
	if a.startId == 0 || a.endId < a.startId {
		assignedIds, err := a.dc.AssignUids(context.Background(), &protos.Num{Val: 1000})
		if err != nil {
			return 0, err
		}
		a.startId = assignedIds.StartId
		a.endId = assignedIds.EndId
	}

	uid = a.startId
	a.startId++

	buf := make([]byte, 20)
	binary.PutUvarint(buf, uid)
	a.ids[id] = uid
	// Should be written to disk before mutation containing this node is persisted
	go a.kv.Set([]byte(id), buf)
	return uid, nil
}

type Dgraph struct {
	opts BatchMutationOptions

	schema chan protos.SchemaUpdate
	nquads chan nquadOp
	dc     protos.DgraphClient
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
}

// NewBatchMutation is used to create a new batch.
// size is the number of RDF's that are sent as part of one request to Dgraph.
// pending is the number of concurrent requests to make to Dgraph server.
func NewDgraphClient(conn *grpc.ClientConn, opts BatchMutationOptions,
	clientDir string) *Dgraph {
	client := protos.NewDgraphClient(conn)
	x.Check(os.MkdirAll(clientDir, 0700))
	opt := badger.DefaultOptions
	opt.SyncWrites = true
	opt.Dir = clientDir
	opt.ValueDir = clientDir
	kv, err := badger.NewKV(&opt)
	x.Checkf(err, "Error while creating badger KV posting store")

	alloc := &allocator{
		kv:  kv,
		dc:  client,
		ids: make(map[string]uint64),
	}
	d := &Dgraph{
		opts:   opts,
		dc:     client,
		start:  time.Now(),
		nquads: make(chan nquadOp, 2*opts.Size),
		schema: make(chan protos.SchemaUpdate, 2*opts.Size),
		alloc:  alloc,
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
	return d
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
RETRY:
	_, err := d.dc.Run(context.Background(), &req.gr)
	if err != nil {
		errString := err.Error()
		// Irrecoverable
		if strings.Contains(errString, "x509") || grpc.Code(err) == codes.Internal {
			log.Fatal(errString)
		}
		fmt.Printf("Retrying req: %d. Error: %v\n", counter, errString)
		time.Sleep(5 * time.Millisecond)
		goto RETRY
	}
	req.reset()
}

func (d *Dgraph) makeRequests() {
	req := new(Req)

	for n := range d.nquads {
		if n.op == SET {
			req.Set(n.e)
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

func (d *Dgraph) Set(e Edge) error {
	d.nquads <- nquadOp{
		e:  e,
		op: SET,
	}
	atomic.AddUint64(&d.rdfs, 1)
	return nil
}

func (d *Dgraph) Delete(e Edge) error {
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
func (d *Dgraph) Flush() {
	close(d.nquads)
	close(d.schema)
	d.wg.Wait()
	if d.ticker != nil {
		d.ticker.Stop()
	}
}

func (d *Dgraph) Run(ctx context.Context, req *Req) (*protos.Response, error) {
	return d.dc.Run(ctx, &req.gr)
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
	v, err := d.dc.CheckVersion(ctx, &protos.Check{})
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
	return Node(uid)
}

func (d *Dgraph) NodeBlank(varname string) (Node, error) {
	if len(varname) == 0 {
		varname = fmt.Sprintf("noreuse-%x", rand.Uint64())
	}
	uid, err := d.alloc.assingOrGet("_:" + varname)
	if err != nil {
		return 0, err
	}
	return Node(uid), nil
}

func (d *Dgraph) NodeXid(xid string, storeXid bool) (Node, error) {
	if len(xid) == 0 {
		return 0, ErrEmptyXid
	}
	if uid, err := d.alloc.getFromCache(xid); err == nil && uid > 0 {
		return Node(uid), nil
	}
	// TODO: return assigned or got from cache and add xid based on that
	uid, err := d.alloc.assingOrGet(xid)
	if err != nil {
		return 0, err
	}
	n := Node(uid)
	if storeXid {
		e, err := n.ScalarEdge("_xid_", xid, types.StringID)
		x.Check(err)
		d.Set(e)
	}
	return n, nil
}
