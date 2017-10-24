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

type Txn struct {
	startTs uint64
	context *protos.TxnContext

	dg *Dgraph
}

func (d *Dgraph) NewTxn() *Txn {
	ts := d.getTimestamp()
	txn := &Txn{
		startTs: ts,
		dg:      d,
	}
	return txn
}

func (txn *Txn) Query(ctx context.Context, q string,
	vars map[string]string) (*protos.Response, error) {
	req := &protos.Request{
		Query:   q,
		Vars:    vars,
		StartTs: txn.startTs,
	}
	return txn.dg.run(ctx, req)
}

func (txn *Txn) mergeContext(src *protos.TxnContext) error {
	if src == nil {
		return nil
	}
	if txn.context == nil {
		txn.context = src
		return nil
	}
	if txn.context.Primary != src.Primary {
		return x.Errorf("Primary key mismatch")
	}
	if txn.context.StartTs != src.StartTs {
		return x.Errorf("StartTs mismatch")
	}
	txn.context.Keys = append(txn.context.Keys, src.Keys...)
	return nil
}

func (txn *Txn) Mutate(ctx context.Context, mu *protos.Mutation) (*protos.Assigned, error) {
	mu.StartTs = txn.startTs
	if txn.context != nil {
		mu.Primary = txn.context.Primary
	}
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
		txn.context = &protos.TxnContext{StartTs: txn.startTs}
	}
	txn.context.CommitTs = 0
	_, err := txn.dg.commitOrAbort(ctx, txn.context)
	return err
}

func (txn *Txn) Commit(ctx context.Context) error {
	if txn.context == nil || len(txn.context.Primary) == 0 {
		// If there were no mutations
		return nil
	}
	txn.context.CommitTs = txn.dg.getTimestamp()
	_, err := txn.dg.commitOrAbort(ctx, txn.context)
	return err
}
