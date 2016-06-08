package worker

import (
	"fmt"
	"log"

	"google.golang.org/grpc"
)

type PayloadCodec struct{}

func (cb *PayloadCodec) Marshal(v interface{}) ([]byte, error) {
	fmt.Println("Marshal")
	p, ok := v.(*Payload)
	if !ok {
		log.Fatal("Invalid type of struct")
	}
	return p.Data, nil
}

func (cb *PayloadCodec) Unmarshal(data []byte, v interface{}) error {
	fmt.Println("Unmarshal")
	p, ok := v.(*Payload)
	if !ok {
		log.Fatal("Invalid type of struct")
	}
	p.Data = data
	return nil
}

func (cb *PayloadCodec) String() string {
	fmt.Println("String")
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
		glog.Fatal(err)
		return nil
	}
	p.conns <- conn
	return p
}

func (p *Pool) dialNew() (*grpc.ClientConn, error) {
	return grpc.Dial(p.Addr, grpc.WithInsecure(), grpc.WithInsecure(),
		grpc.WithCodec(&PayloadCodec{}))
}
