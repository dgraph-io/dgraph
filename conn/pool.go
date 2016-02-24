package conn

import (
	"net"
	"net/rpc"

	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("conn")

type Pool struct {
	clients chan *rpc.Client
	addr    string
}

func NewPool(addr string, maxCap int) *Pool {
	p := new(Pool)
	p.addr = addr
	p.clients = make(chan *rpc.Client, maxCap)
	client, err := p.dialNew()
	if err != nil {
		glog.Fatal(err)
		return nil
	}
	p.clients <- client
	return p
}

func (p *Pool) dialNew() (*rpc.Client, error) {
	nconn, err := net.Dial("tcp", p.addr)
	if err != nil {
		return nil, err
	}
	cc := &ClientCodec{
		Rwc: nconn,
	}
	return rpc.NewClientWithCodec(cc), nil
}

func (p *Pool) Get() (*rpc.Client, error) {
	select {
	case client := <-p.clients:
		return client, nil
	default:
		return p.dialNew()
	}
}

func (p *Pool) Put(client *rpc.Client) error {
	select {
	case p.clients <- client:
		return nil
	default:
		return client.Close()
	}
}

func (p *Pool) Close() error {
	// We're not doing a clean exit here. A clean exit here would require
	// mutex locks around conns; which seems unnecessary just to shut down
	// the server.
	return nil
}
