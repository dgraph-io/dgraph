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
	"math/rand"
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

// Dgraph is a transaction aware client to a set of dgraph server instances.
type Dgraph struct {
	dc []protos.DgraphClient

	mu      sync.Mutex
	linRead *protos.LinRead
}

// NewDgraphClient creates a new Dgraph for interacting with the Dgraph store connected to in
// conns.
// The client can be backed by multiple connections (to the same server, or multiple servers in a
// cluster).
//
// A single client is thread safe for sharing with multiple go routines.
func NewDgraphClient(clients ...protos.DgraphClient) *Dgraph {
	dg := &Dgraph{
		dc:      clients,
		linRead: &protos.LinRead{},
	}

	return dg
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

// By setting various fields of protos.Operation, Alter can be used to do the
// following:
//
// 1. Modify the schema.
//
// 2. Drop a predicate.
//
// 3. Drop the database.
func (d *Dgraph) Alter(ctx context.Context, op *protos.Operation) error {
	dc := d.anyClient()
	_, err := dc.Alter(ctx, op)
	return err
}

func (d *Dgraph) anyClient() protos.DgraphClient {
	return d.dc[rand.Intn(len(d.dc))]
}
