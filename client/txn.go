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

var ErrAborted = x.Errorf("Transaction has been aborted due to conflict")

type Txn struct {
	context *protos.TxnContext

	dg *Dgraph
}

func (d *Dgraph) NewTxn() (*Txn, error) {
	txn := &Txn{
		dg: d,
		context: &protos.TxnContext{
			LinRead: d.getLinRead(),
		},
	}
	startTs, err := txn.dg.startTs(context.Background())
	if err != nil {
		return nil, err
	}
	txn.context.StartTs = startTs
	return txn, nil
}

func (txn *Txn) Query(ctx context.Context, q string,
	vars map[string]string) (*protos.Response, error) {
	req := &protos.Request{
		Query:   q,
		Vars:    vars,
		StartTs: txn.context.StartTs,
		LinRead: txn.context.LinRead,
	}
	resp, err := txn.dg.query(ctx, req)
	if err == nil {
		x.MergeLinReads(txn.context.LinRead, resp.Txn.LinRead)
		txn.dg.mergeLinRead(resp.Txn.LinRead)
	}
	return resp, err
}

func (txn *Txn) mergeContext(src *protos.TxnContext) error {
	if src == nil {
		return nil
	}

	x.MergeLinReads(txn.context.LinRead, src.LinRead)
	txn.dg.mergeLinRead(src.LinRead) // Also merge it with client.

	if txn.context.StartTs != src.StartTs {
		return x.Errorf("StartTs mismatch")
	}
	txn.context.Keys = append(txn.context.Keys, src.Keys...)
	x.MergeLinReads(txn.context.LinRead, src.LinRead)
	return nil
}

func (txn *Txn) Mutate(ctx context.Context, mu *protos.Mutation) (*protos.Assigned, error) {
	mu.StartTs = txn.context.StartTs
	ag, err := txn.dg.mutate(ctx, mu)
	if ag != nil {
		if err := txn.mergeContext(ag.Context); err != nil {
			fmt.Printf("error while merging context: %v\n", err)
		}
		if len(ag.Error) > 0 {
			// fmt.Printf("Mutate failed. start=%d ag= %+v\n", txn.startTs, ag)
			return ag, errors.New(ag.Error)
		}
	}
	return ag, err
}

func (txn *Txn) Abort(ctx context.Context) error {
	if txn.context == nil {
		txn.context = &protos.TxnContext{StartTs: txn.context.StartTs}
	}
	txn.context.Aborted = true
	_, err := txn.dg.commitOrAbort(ctx, txn.context)
	return err
}

func (txn *Txn) Commit(ctx context.Context) error {
	if txn.context == nil {
		// If there were no mutations
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
