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

package dgo

import (
	"context"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrFinished = errors.New("Transaction has already been committed or discarded")
	ErrReadOnly = errors.New("Readonly transaction cannot run mutations or be committed")
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
	context *api.TxnContext

	finished   bool
	mutated    bool
	readOnly   bool
	bestEffort bool

	dg *Dgraph
	dc api.DgraphClient
}

func (txn *Txn) Sequencing(sequencing api.LinRead_Sequencing) {
	// Sequencing is no longer used.
}

// BestEffort enables best effort in read-only queries. Using this flag will ask the
// Dgraph Alpha to try to get timestamps from memory in a best effort to reduce the number of
// outbound requests to Zero. This may yield improved latencies in read-bound datasets.
//
// This method will panic if the transaction is not read-only.
// Returns the transaction itself.
func (txn *Txn) BestEffort() *Txn {
	if !txn.readOnly {
		panic("Best effort only works for read-only queries.")
	}
	txn.bestEffort = true
	return txn
}

// NewTxn creates a new transaction.
func (d *Dgraph) NewTxn() *Txn {
	return &Txn{
		dg:      d,
		dc:      d.anyClient(),
		context: &api.TxnContext{},
	}
}

func (d *Dgraph) NewReadOnlyTxn() *Txn {
	txn := d.NewTxn()
	txn.readOnly = true
	return txn
}

// Query sends a query to one of the connected dgraph instances. If no
// mutations need to be made in the same transaction, it's convenient to chain
// the method, e.g. NewTxn().Query(ctx, "...").
func (txn *Txn) Query(ctx context.Context, q string) (*api.Response, error) {
	return txn.QueryWithVars(ctx, q, nil)
}

// QueryWithVars is like Query, but allows a variable map to be used. This can
// provide safety against injection attacks.
func (txn *Txn) QueryWithVars(ctx context.Context, q string,
	vars map[string]string) (*api.Response, error) {
	if txn.finished {
		return nil, ErrFinished
	}
	req := &api.Request{
		Query:      q,
		Vars:       vars,
		StartTs:    txn.context.StartTs,
		ReadOnly:   txn.readOnly,
		BestEffort: txn.bestEffort,
	}
	ctx = txn.dg.getContext(ctx)
	resp, err := txn.dc.Query(ctx, req)
	if isJwtExpired(err) {
		err = txn.dg.retryLogin(ctx)
		if err != nil {
			return nil, err
		}
		ctx = txn.dg.getContext(ctx)
		resp, err = txn.dc.Query(ctx, req)
	}

	if err == nil {
		if err := txn.mergeContext(resp.GetTxn()); err != nil {
			return nil, err
		}
	}
	return resp, err
}

func (txn *Txn) mergeContext(src *api.TxnContext) error {
	if src == nil {
		return nil
	}

	if txn.context.StartTs == 0 {
		txn.context.StartTs = src.StartTs
	}
	if txn.context.StartTs != src.StartTs {
		return errors.New("StartTs mismatch")
	}
	txn.context.Keys = append(txn.context.Keys, src.Keys...)
	txn.context.Preds = append(txn.context.Preds, src.Preds...)
	return nil
}

// Mutate allows data stored on dgraph instances to be modified. The fields in
// api.Mutation come in pairs, set and delete. Mutations can either be
// encoded as JSON or as RDFs.
//
// If CommitNow is set, then this call will result in the transaction
// being committed. In this case, an explicit call to Commit doesn't need to
// subsequently be made.
//
// If the mutation fails, then the transaction is discarded and all future
// operations on it will fail.
func (txn *Txn) Mutate(ctx context.Context, mu *api.Mutation) (*api.Assigned, error) {
	switch {
	case txn.readOnly:
		return nil, ErrReadOnly
	case txn.finished:
		return nil, ErrFinished
	}

	txn.mutated = true
	mu.StartTs = txn.context.StartTs
	ctx = txn.dg.getContext(ctx)
	ag, err := txn.dc.Mutate(ctx, mu)
	if isJwtExpired(err) {
		err = txn.dg.retryLogin(ctx)
		if err != nil {
			return nil, err
		}
		ctx = txn.dg.getContext(ctx)
		ag, err = txn.dc.Mutate(ctx, mu)
	}

	if err != nil {
		_ = txn.Discard(ctx) // Ignore error - user should see the original error.
		return nil, err
	}

	if mu.CommitNow {
		txn.finished = true
	}
	err = txn.mergeContext(ag.Context)
	return ag, err
}

// Commit commits any mutations that have been made in the transaction. Once
// Commit has been called, the lifespan of the transaction is complete.
//
// Errors could be returned for various reasons. Notably, ErrAborted could be
// returned if transactions that modify the same data are being run
// concurrently. It's up to the user to decide if they wish to retry. In this
// case, the user should create a new transaction.
func (txn *Txn) Commit(ctx context.Context) error {
	switch {
	case txn.readOnly:
		return ErrReadOnly
	case txn.finished:
		return ErrFinished
	}

	err := txn.commitOrAbort(ctx)
	if s, ok := status.FromError(err); ok && s.Code() == codes.Aborted {
		err = y.ErrAborted
	}
	return err
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
	err := txn.commitOrAbort(ctx)
	if err != nil {
		txn.context.Aborted = true
	}
	return err
}

func (txn *Txn) commitOrAbort(ctx context.Context) error {
	if txn.finished {
		return nil
	}
	txn.finished = true

	if !txn.mutated {
		return nil
	}

	ctx = txn.dg.getContext(ctx)
	_, err := txn.dc.CommitOrAbort(ctx, txn.context)
	if isJwtExpired(err) {
		err = txn.dg.retryLogin(ctx)
		if err != nil {
			return err
		}
		ctx = txn.dg.getContext(ctx)
		_, err = txn.dc.CommitOrAbort(ctx, txn.context)
	}

	return err
}
