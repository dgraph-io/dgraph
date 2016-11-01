package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"log"
	"sync"

	"github.com/dgraph-io/dgraph/x"

	"google.golang.org/grpc"
)

// PayloadCodec is a custom codec that is that is used for internal worker
// communication.
type PayloadCodec struct{}

// Marshal marshals v into a Payload instance. v contains serialised data
// for a flatbuffer Query object.
func (cb *PayloadCodec) Marshal(v interface{}) ([]byte, error) {
	p, ok := v.(*Payload)
	if !ok {
		log.Fatalf("Invalid type of struct: %+v", v)
	}
	return p.Data, nil
}

// Unmarshal unmarshals byte slice data into v.
func (cb *PayloadCodec) Unmarshal(data []byte, v interface{}) error {
	p, ok := v.(*Payload)
	if !ok {
		log.Fatalf("Invalid type of struct: %+v", v)
	}
	p.Data = data
	return nil
}

func (cb *PayloadCodec) String() string {
	return "worker.PayloadCodec"
}

// Pool is used to manage the grpc client connections for communicating with
// other worker instances.
type pool struct {
	conns chan *grpc.ClientConn
	Addr  string
}

type poolsi struct {
	sync.RWMutex
	all map[string]*pool
}

var pi *poolsi

func init() {
	pi = new(poolsi)
	pi.all = make(map[string]*pool)
}

func pools() *poolsi {
	return pi
}

func (p *poolsi) get(addr string) *pool {
	p.RLock()
	defer p.RUnlock()
	pool, _ := p.all[addr]
	return pool
}

func (p *poolsi) connect(addr string) {
	p.RLock()
	_, has := p.all[addr]
	p.RUnlock()
	if has {
		return
	}

	pool := newPool(addr, 5)
	query := new(Payload)
	query.Data = make([]byte, 10)
	x.Check2(rand.Read(query.Data))

	conn, err := pool.Get()
	if err != nil {
		log.Fatalf("Unable to connect: %v", err)
	}

	c := NewWorkerClient(conn)
	resp, err := c.Echo(context.Background(), query)
	if err != nil {
		log.Fatalf("Unable to connect: %v", err)
	}
	x.AssertTrue(bytes.Equal(resp.Data, query.Data))
	x.Check(pool.Put(conn))

	p.Lock()
	defer p.Unlock()
	_, has = p.all[addr]
	if has {
		return
	}
	p.all[addr] = pool
}

// NewPool initializes an instance of Pool which is used to connect with other
// workers. The pool instance also has a buffered channel,conn with capacity
// maxCap that stores the connections.
func newPool(addr string, maxCap int) *pool {
	p := new(pool)
	p.Addr = addr
	p.conns = make(chan *grpc.ClientConn, maxCap)
	conn, err := p.dialNew()
	if err != nil {
		log.Fatal(err)
		return nil
	}
	p.conns <- conn
	return p
}

func (p *pool) dialNew() (*grpc.ClientConn, error) {
	return grpc.Dial(p.Addr, grpc.WithInsecure(), grpc.WithCodec(&PayloadCodec{}))
}

// Get returns a connection from the pool of connections or a new connection if
// the pool is empty.
func (p *pool) Get() (*grpc.ClientConn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return p.dialNew()
	}
}

// Put returns a connection to the pool or closes and discards the connection
// incase the pool channel is at capacity.
func (p *pool) Put(conn *grpc.ClientConn) error {
	select {
	case p.conns <- conn:
		return nil
	default:
		return conn.Close()
	}
}
