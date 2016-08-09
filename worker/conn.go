package worker

import (
	"log"

	"google.golang.org/grpc"
)

type PayloadCodec struct{}

func (cb *PayloadCodec) Marshal(v interface{}) ([]byte, error) {
	p, ok := v.(*Payload)
	if !ok {
		log.Fatalf("Invalid type of struct: %+v", v)
	}
	return p.Data, nil
}

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

type Pool struct {
	conns chan *grpc.ClientConn
	Addr  string
}

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

func (p *Pool) Get() (*grpc.ClientConn, error) {
	select {
	case conn := <-p.conns:
		return conn, nil
	default:
		return p.dialNew()
	}
}

func (p *Pool) Put(conn *grpc.ClientConn) error {
	select {
	case p.conns <- conn:
		return nil
	default:
		return conn.Close()
	}
}
