/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package conn

import (
	"net"
	"net/rpc"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("conn")

type Pool struct {
	clients chan *rpc.Client
	Addr    string
}

func NewPool(addr string, maxCap int) *Pool {
	p := new(Pool)
	p.Addr = addr
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
	d := &net.Dialer{
		Timeout: 3 * time.Minute,
	}
	var nconn net.Conn
	var err error
	// This loop will retry for 10 minutes before giving up.
	for i := 0; i < 60; i++ {
		nconn, err = d.Dial("tcp", p.Addr)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "refused") {
			break
		}

		glog.WithField("error", err).WithField("addr", p.Addr).
			Info("Retrying connection...")
		time.Sleep(10 * time.Second)
	}
	if err != nil {
		return nil, err
	}
	cc := &ClientCodec{
		Rwc: nconn,
	}
	return rpc.NewClientWithCodec(cc), nil
}

func (p *Pool) Call(serviceMethod string, args interface{},
	reply interface{}) error {

	client, err := p.get()
	if err != nil {
		return err
	}
	if err = client.Call(serviceMethod, args, reply); err != nil {
		return err
	}

	select {
	case p.clients <- client:
		return nil
	default:
		return client.Close()
	}
}

func (p *Pool) get() (*rpc.Client, error) {
	select {
	case client := <-p.clients:
		return client, nil
	default:
		return p.dialNew()
	}
}

func (p *Pool) Close() error {
	// We're not doing a clean exit here. A clean exit here would require
	// synchronization, which seems unnecessary for now. But, we should
	// add one if required later.
	return nil
}
