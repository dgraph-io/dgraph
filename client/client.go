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
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

type Dgraph struct {
	zero protos.ZeroClient
	dc   []protos.DgraphClient

	mu     sync.Mutex
	needTs []chan uint64
	notify chan struct{}

	linRead *protos.LinRead
	state   *protos.MembershipState
}

func (d *Dgraph) ZeroClient() protos.ZeroClient {
	return d.zero
}

// TODO(tzdybal) - hide this function from users
func NewClient(clients []protos.DgraphClient) *Dgraph {
	d := &Dgraph{
		dc: clients,
	}

	return d
}

// NewDgraphClient creates a new Dgraph for interacting with the Dgraph store connected to in
// conns.
// The client can be backed by multiple connections (to the same server, or multiple servers in a
// cluster).
//
// A single client is thread safe for sharing with multiple go routines.
func NewDgraphClient(zero protos.ZeroClient, dc protos.DgraphClient) *Dgraph {
	dg := &Dgraph{
		zero:    zero,
		dc:      []protos.DgraphClient{dc},
		notify:  make(chan struct{}, 1),
		linRead: &protos.LinRead{},
	}

	go dg.fillTimestampRequests()
	return dg
}

func (d *Dgraph) getTimestamp() uint64 {
	ch := make(chan uint64)
	d.mu.Lock()
	d.needTs = append(d.needTs, ch)
	d.mu.Unlock()

	select {
	case d.notify <- struct{}{}:
	default:
	}
	return <-ch
}

func (d *Dgraph) mergeLinRead(src *protos.LinRead) {
	d.mu.Lock()
	defer d.mu.Unlock()
	x.MergeLinReads(d.linRead, src)
}

func (d *Dgraph) getLinRead() *protos.LinRead {
	d.mu.Lock()
	defer d.mu.Unlock()
	return proto.Clone(d.linRead).(*protos.LinRead)
}

func (d *Dgraph) fillTimestampRequests() {
	var chs []chan uint64
	for range d.notify {
	RETRY:
		d.mu.Lock()
		chs = append(chs, d.needTs...)
		d.needTs = d.needTs[:0]
		d.mu.Unlock()

		if len(chs) == 0 {
			continue
		}
		num := &protos.Num{Val: uint64(len(chs))}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ts, err := d.zero.Timestamps(ctx, num)
		cancel()
		if err != nil {
			log.Printf("Error while retrieving timestamps: %v. Will retry...\n", err)
			goto RETRY
		}
		x.AssertTrue(ts.EndId-ts.StartId+1 == uint64(len(chs)))
		for i, ch := range chs {
			ch <- ts.StartId + uint64(i)
		}
		chs = chs[:0]
	}
}

// DropAll deletes all edges and schema from Dgraph.
func (d *Dgraph) Alter(ctx context.Context, op *protos.Operation) error {
	dc := d.anyClient()
	if len(op.Schema) > 0 {
		op.StartTs = d.getTimestamp()
	}
	_, err := dc.Alter(ctx, op)
	return err
}

func (d *Dgraph) CheckSchema(schema *protos.SchemaUpdate) error {
	if len(schema.Predicate) == 0 {
		return x.Errorf("No predicate specified for schemaUpdate")
	}
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

func (d *Dgraph) query(ctx context.Context, req *protos.Request) (*protos.Response, error) {
	dc := d.anyClient()
	return dc.Query(ctx, req)
}

func (d *Dgraph) mutate(ctx context.Context, mu *protos.Mutation) (*protos.Assigned, error) {
	dc := d.anyClient()
	return dc.Mutate(ctx, mu)
}

func (d *Dgraph) commitOrAbort(ctx context.Context, txn *protos.TxnContext) (*protos.TxnContext,
	error) {
	return d.zero.CommitOrAbort(ctx, txn)
}

// CheckVersion checks if the version of dgraph and dgraph-live-loader are the same.  If either the
// versions don't match or the version information could not be obtained an error message is
// printed.
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

func (d *Dgraph) anyClient() protos.DgraphClient {
	return d.dc[rand.Intn(len(d.dc))]
}
