package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/dgraph-io/badger/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

var (
	ErrConnected = errors.New("Edge already connected to another node.")
	ErrValue     = errors.New("Edge already has a value.")
	ErrEmptyXid  = errors.New("Empty XID node.")
)

type BatchMutationOptions struct {
	Size          int
	Pending       int
	PrintCounters bool
}

type allocations struct {
	sync.Mutex
	allocs map[string]*sync.Mutex
}

func (a *allocations) getOrBlock(id string) bool {
	a.Lock()
	defer a.Unlock()

	m, ok := a.allocs[id]
	if !ok {
		m = &sync.Mutex{}
		m.Lock()
		a.allocs[id] = m
		return true // I'm the one who created the lock.
	}
	// someone else created the lock. I'll just have to block.
	m.Lock()
	m.Unlock()
	return false
}

// release should be called only after writing the id to Badger.
func (a *allocations) release(id string) {
	a.Lock()
	defer a.Unlock()

	m, ok := a.allocs[id]
	if !ok {
		log.Fatalf("Id: %q should have been present", id)
	}
	m.Unlock()
	delete(m.allocs, id)
}

type BatchMutation2 struct {
	kv   *badger.KV
	opts BatchMutationOptions

	schema chan protos.SchemaUpdate
	nquads chan nquadOp
	dc     protos.DgraphClient
	wg     sync.WaitGroup

	// Miscellaneous information to print counters.
	// Num of RDF's sent
	rdfs uint64
	// Num of mutations sent
	mutations uint64
	// To get time elapsed.
	start time.Time
}

func NewBatchMutation2(client protos.DgraphClient, opts BatchMutationOptions) *BatchMutation2 {
	bm := BatchMutation2{
		opts: opts,
	}

	for i := 0; i < pending; i++ {
		bm.wg.Add(1)
		go bm.makeRequests()
	}
	bm.wg.Add(1)

	c := &DgraphClient{}
	rand.Seed(time.Now().Unix())
	return c
}

func (batch *BatchMutation) makeRequests() {
	var gr protos.Request

	for n := range batch.nquads {
		req.addMutation(n.nq, n.op)
		if req.size() == batch.size {
			batch.request(req)
		}
	}

	if req.size() > 0 {
		batch.request(req)
	}
	batch.wg.Done()
}

func checkSchema(schema protos.SchemaUpdate) error {
	typ := types.TypeID(schema.ValueType)
	if typ == types.UidID && schema.Directive == protos.SchemaUpdate_INDEX {
		// index on uid type
		return x.Errorf("Index not allowed on predicate of type uid on predicate %s",
			schema.Predicate)
	} else if typ != types.UidID && schema.Directive == protos.SchemaUpdate_REVERSE {
		// reverse on non-uid type
		return x.Errorf("Cannot reverse for non-uid type on predicate %s", schema.Predicate)
	}
	return nil
}

func (batch *BatchMutation) AddSchema(s protos.SchemaUpdate) error {
	if err := checkSchema(s); err != nil {
		return err
	}
	batch.schema <- s
	return nil
}

type Node struct {
	uid   uint64
	blank string
	xid   string
}

func (c *DgraphClient) NodeUid(uid uint64) *Node {
	return &Node{uid: uid}
}

func (c *DgraphClient) getFromCache(key string) (*Node, error) {
	var item badger.KVItem
	if err := c.kv.Get([]byte(key), &item); err != nil {
		return nil, err
	}
	val := item.Value()
	if len(val) == 0 {
		return nil, nil
	}
	uid, n := binary.Uvarint(val)
	if n <= 0 {
		return nil, fmt.Errorf("Unable to parse val %q to uint64 for key %q", val, key)
	}
	return &Node{uid: uid}, nil
}

func (c *DgraphClient) NodeBlank(varname string) (*Node, error) {
	if len(varname) == 0 {
		return &Node{blank: fmt.Sprintf("noreuse-%x", rand.Uint64())}, nil
	}
	node, err := c.getFromCache("_:" + varname)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return &Node{blank: varname}, nil
	}
	return node, nil
}

func (c *DgraphClient) NodeXid(xid string) (*Node, error) {
	if len(xid) == 0 {
		return nil, ErrEmptyXid
	}
	node, err := c.getFromCache("xid:" + xid)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return &Node{xid: xid}, nil
	}
	return node, nil
}

func (n *Node) String() string {
	if n.uid > 0 {
		return fmt.Sprintf("<%#x>", n.uid)
	}
	if len(n.blank) > 0 {
		return "_:" + n.blank
	}
	return fmt.Sprintf("<%s>", n.xid)
}

type Edge struct {
	nq protos.NQuad
}

func (n *Node) Edge(pred string) *Edge {
	e := &Edge{}
	e.nq.Subject = n.String()
	e.nq.Predicate = pred
	return e
}

func (e *Edge) ConnectTo(n *Node) error {
	if e.nq.ObjectType > 0 {
		return ErrValue
	}
	e.nq.ObjectId = n.String()
	return nil
}

func validateStr(val string) error {
	for idx, c := range val {
		if c == '"' && (idx == 0 || val[idx-1] != '\\') {
			return fmt.Errorf(`" must be preceded by a \ in object value`)
		}
	}
	return nil
}

func (e *Edge) SetValueString(val string) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	if err := validateStr(val); err != nil {
		return err
	}

	v, err := types.ObjectValue(types.StringID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.StringID)
	return nil
}

func (e *Edge) SetValueInt(val int64) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.IntID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.IntID)
	return nil
}

func (e *Edge) SetValueFloat(val float64) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.FloatID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.FloatID)
	return nil
}

func (e *Edge) SetValueBool(val bool) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.BoolID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.BoolID)
	return nil
}

func (e *Edge) SetValuePassword(val string) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.PasswordID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.PasswordID)
	return nil
}

func (e *Edge) SetValueDatetime(dateTime time.Time) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	d, err := types.ObjectValue(types.DateTimeID, dateTime)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = d
	e.nq.ObjectType = int32(types.DateTimeID)
	return nil
}

func (e *Edge) SetValueGeoJson(json string) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	var g geom.T
	// Parse the json
	err := geojson.Unmarshal([]byte(json), &g)
	if err != nil {
		return err
	}

	geo, err := types.ObjectValue(types.GeoID, g)
	if err != nil {
		return err
	}

	e.nq.ObjectValue = geo
	e.nq.ObjectType = int32(types.GeoID)
	return nil
}

func (e *Edge) AddFacet(key, val string) {
	e.nq.Facets = append(e.nq.Facets, &protos.Facet{
		Key: key,
		Val: val,
	})
}
