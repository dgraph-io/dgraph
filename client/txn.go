/*
 * Copyright 2016 Dgraph Labs, Inc.
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

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/y"
	"github.com/pkg/errors"
)

var (
	ErrAborted  = errors.New("Transaction has been aborted due to conflict")
	ErrFinished = errors.New("Transaction has already been committed or discarded")
)

// Txn is a single atomic transaction.
//
// A transaction lifecycle is as follows:
//
// 1. Created using NewTxn.
//
// 2. Various Query and Mutate calls made.
//
// 3. Commit or Discard used. If any mutations have been made, It's important
// that at least one of these methods is called to clean up resources. Discard
// is a no-op if Commit has already been called, so it's safe to defer a call
// to Discard immediately after NewTxn.
type Txn struct {
	context *protos.TxnContext

	finished bool
	mutated  bool

	dg *Dgraph
}

// NewTxn creates a new transaction.
func (d *Dgraph) NewTxn() *Txn {
	txn := &Txn{
		dg: d,
		context: &protos.TxnContext{
			LinRead: d.getLinRead(),
		},
	}
	return txn
}

// Query sends a query to one of the connected dgraph instances. If no
// mutations need to be made in the same transaction, it's convenient to chain
// the method, e.g. NewTxn().Query(ctx, "...").
func (txn *Txn) Query(ctx context.Context, q string) (*protos.Response, error) {
	return txn.QueryWithVars(ctx, q, nil)
}

// QueryWithVars is like Query, but allows a variable map to be used. This can
// provide safety against injection attacks.
func (txn *Txn) QueryWithVars(ctx context.Context, q string,
	vars map[string]string) (*protos.Response, error) {
	if txn.finished {
		return nil, ErrFinished
	}
	req := &protos.Request{
		Query:   q,
		Vars:    vars,
		StartTs: txn.context.StartTs,
		LinRead: txn.context.LinRead,
	}
	dc := txn.dg.anyClient()
	resp, err := dc.Query(ctx, req)
	if err == nil {
		if err := txn.mergeContext(resp.GetTxn()); err != nil {
			return nil, err
		}
	}
	return resp, err
}

func (txn *Txn) mergeContext(src *protos.TxnContext) error {
	if src == nil {
		return nil
	}

	y.MergeLinReads(txn.context.LinRead, src.LinRead)
	txn.dg.mergeLinRead(src.LinRead) // Also merge it with client.

	if txn.context.StartTs == 0 {
		txn.context.StartTs = src.StartTs
	}
	if txn.context.StartTs != src.StartTs {
		return errors.New("StartTs mismatch")
	}
	txn.context.Keys = append(txn.context.Keys, src.Keys...)
	return nil
}

// Mutate allows data stored on dgraph instances to be modified. The fields in
// protos.Mutation come in pairs, set and delete. Mutations can either be
// encoded as JSON or as RDFs.
//
// If CommitNow is set, then this call will result in the transaction
// being committed. In this case, an explicit call to Commit doesn't need to
// subsequently be made.
func (txn *Txn) Mutate(ctx context.Context, mu *protos.Mutation) (*protos.Assigned, error) {
	if txn.finished {
		return nil, ErrFinished
	}

	mu.StartTs = txn.context.StartTs
	dc := txn.dg.anyClient()
	ag, err := dc.Mutate(ctx, mu)
	if err != nil {
		return nil, err
	}
	txn.mutated = true
	if err := txn.mergeContext(ag.Context); err != nil {
		fmt.Printf("error while merging context: %v\n", err)
		return nil, err
	}
	if len(ag.Error) > 0 {
		return nil, errors.New(ag.Error)
	}
	return ag, nil
}

// Commit commits any mutations that have been made in the transaction. Once
// Commit has been called, the lifespan of the transaction is complete.
//
// Errors could be returned for various reasons. Notably, ErrAborted could be
// returned if transactions that modify the same data are being run
// concurrently. It's up to the user to decide if they wish to retry. In this
// case, the user should create a new transaction.
func (txn *Txn) Commit(ctx context.Context) error {
	if txn.finished {
		return ErrFinished
	}
	txn.finished = true

	if !txn.mutated {
		return nil
	}
	dc := txn.dg.anyClient()
	tctx, err := dc.CommitOrAbort(ctx, txn.context)
	if err != nil {
		return err
	}
	if tctx.Aborted {
		return ErrAborted
	}
	return nil
}

// Discard cleans up the resources associated with an uncommitted transaction
// that contains mutations. It is a no-op on transactions that have already
// been committed or don't contain mutations. Therefore it is safe (and
// recommended) to call as a deferred function immediately after a new
// transaction is created.
//
// In some cases, the transaction can't be discarded, e.g. the grpc connection
// is unavailable. In these cases, the server will eventually do the
// transaction clean up.
func (txn *Txn) Discard(ctx context.Context) error {
	if txn.finished {
		return nil
	}
	txn.finished = true

	if !txn.mutated {
		return nil
	}
	txn.context.Aborted = true
	dc := txn.dg.anyClient()
	_, err := dc.CommitOrAbort(ctx, txn.context)
	return err
}
