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
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

var (
	ErrAborted  = x.Errorf("Transaction has been aborted due to conflict")
	ErrFinished = x.Errorf("Transaction has already been committed or discarded")
)

type Txn struct {
	context *protos.TxnContext

	finished bool
	mutated  bool

	dg *Dgraph
}

func (d *Dgraph) NewTxn() *Txn {
	txn := &Txn{
		dg: d,
		context: &protos.TxnContext{
			LinRead: d.getLinRead(),
		},
	}
	return txn
}

func (txn *Txn) Query(ctx context.Context, q string) (*protos.Response, error) {
	if txn.finished {
		return nil, ErrFinished
	}
	req := &protos.Request{
		Query:   q,
		StartTs: txn.context.StartTs,
		LinRead: txn.context.LinRead,
	}
	resp, err := txn.dg.query(ctx, req)
	if err == nil {
		if err := txn.mergeContext(resp.GetTxn()); err != nil {
			return nil, err
		}
	}
	return resp, err
}

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
	resp, err := txn.dg.query(ctx, req)
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

	x.MergeLinReads(txn.context.LinRead, src.LinRead)
	txn.dg.mergeLinRead(src.LinRead) // Also merge it with client.

	if txn.context.StartTs == 0 {
		txn.context.StartTs = src.StartTs
	}
	if txn.context.StartTs != src.StartTs {
		return x.Errorf("StartTs mismatch")
	}
	txn.context.Keys = append(txn.context.Keys, src.Keys...)
	return nil
}

func (txn *Txn) Mutate(ctx context.Context, mu *protos.Mutation) (*protos.Assigned, error) {
	if txn.finished {
		return nil, ErrFinished
	}

	mu.StartTs = txn.context.StartTs
	ag, err := txn.dg.mutate(ctx, mu)
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

func (txn *Txn) Commit(ctx context.Context) error {
	if txn.finished {
		return ErrFinished
	}
	txn.finished = true

	if !txn.mutated {
		return nil
	}
	tctx, err := txn.dg.commitOrAbort(ctx, txn.context)
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
	_, err := txn.dg.commitOrAbort(ctx, txn.context)
	return err
}
