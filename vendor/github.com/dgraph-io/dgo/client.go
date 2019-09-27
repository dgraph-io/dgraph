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

package dgo

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Dgraph is a transaction aware client to a set of Dgraph server instances.
type Dgraph struct {
	jwtMutex sync.RWMutex
	jwt      api.Jwt
	dc       []api.DgraphClient
}

// NewDgraphClient creates a new Dgraph (client) for interacting with Alphas.
// The client is backed by multiple connections to the same or different
// servers in a cluster.
//
// A single Dgraph (client) is thread safe for sharing with multiple goroutines.
func NewDgraphClient(clients ...api.DgraphClient) *Dgraph {
	dg := &Dgraph{
		dc: clients,
	}

	return dg
}

// Login logs in the current client using the provided credentials.
// Valid for the duration the client is alive.
func (d *Dgraph) Login(ctx context.Context, userid string, password string) error {
	d.jwtMutex.Lock()
	defer d.jwtMutex.Unlock()

	dc := d.anyClient()
	loginRequest := &api.LoginRequest{
		Userid:   userid,
		Password: password,
	}
	resp, err := dc.Login(ctx, loginRequest)
	if err != nil {
		return err
	}

	return d.jwt.Unmarshal(resp.Json)
}

// Alter can be used to do the following by setting various fields of api.Operation:
//   1. Modify the schema.
//   2. Drop a predicate.
//   3. Drop the database.
func (d *Dgraph) Alter(ctx context.Context, op *api.Operation) error {
	dc := d.anyClient()

	ctx = d.getContext(ctx)
	_, err := dc.Alter(ctx, op)

	if isJwtExpired(err) {
		err = d.retryLogin(ctx)
		if err != nil {
			return err
		}

		ctx = d.getContext(ctx)
		_, err = dc.Alter(ctx, op)
	}

	return err
}

func (d *Dgraph) retryLogin(ctx context.Context) error {
	d.jwtMutex.Lock()
	defer d.jwtMutex.Unlock()

	if len(d.jwt.RefreshJwt) == 0 {
		return fmt.Errorf("refresh jwt should not be empty")
	}

	dc := d.anyClient()
	loginRequest := &api.LoginRequest{
		RefreshToken: d.jwt.RefreshJwt,
	}
	resp, err := dc.Login(ctx, loginRequest)
	if err != nil {
		return err
	}

	return d.jwt.Unmarshal(resp.Json)
}

func (d *Dgraph) getContext(ctx context.Context) context.Context {
	d.jwtMutex.RLock()
	defer d.jwtMutex.RUnlock()

	if len(d.jwt.AccessJwt) > 0 {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			// no metadata key is in the context, add one
			md = metadata.New(nil)
		}

		md.Set("accessJwt", d.jwt.AccessJwt)
		return metadata.NewOutgoingContext(ctx, md)
	}

	return ctx
}

// isJwtExpired returns true if the error indicates that the jwt has expired.
func isJwtExpired(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	return ok && st.Code() == codes.Unauthenticated &&
		strings.Contains(err.Error(), "Token is expired")
}

func (d *Dgraph) anyClient() api.DgraphClient {
	return d.dc[rand.Intn(len(d.dc))]
}

// DeleteEdges sets the edges corresponding to predicates
// on the node with the given uid for deletion.
// This helper function doesn't run the mutation on the server.
// Txn needs to be committed in order to execute the mutation.
func DeleteEdges(mu *api.Mutation, uid string, predicates ...string) {
	for _, predicate := range predicates {
		mu.Del = append(mu.Del, &api.NQuad{
			Subject:   uid,
			Predicate: predicate,
			// _STAR_ALL is defined as x.Star in x package.
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "_STAR_ALL"}},
		})
	}
}
