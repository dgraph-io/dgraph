package worker

import (
	"log"

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
type Pool struct {
	conns chan *grpc.ClientConn
	Addr  string
}

// NewPool initializes an instance of Pool which is used to connect with other
// workers. The pool instance also has a buffered channel,conn with capacity
// maxCap that stores the connections.
func NewPool(addr string, maxCap int) *Pool {
	p := new(Pool)
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

func (p *Pool) dialNew() (*grpc.ClientConn, error) {
	return grpc.Dial(p.Addr, grpc.WithInsecure(), grpc.WithCodec(&PayloadCodec{}))
}

// Get returns a connection from the pool of connections or a new connection if
// the pool is empty.
func (p *Pool) Get() (*grpc.ClientConn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return p.dialNew()
	}
}

// Put returns a connection to the pool or closes and discards the connection
// incase the pool channel is at capacity.
func (p *Pool) Put(conn *grpc.ClientConn) error {
	select {
	case p.conns <- conn:
		return nil
	default:
		return conn.Close()
	}
}
