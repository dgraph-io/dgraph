/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package index

// TODO(jiawei)
// This is supposed to send out RPC calls if server instance does not handle
// given attribute / predicate.

import (
	"context"

	"github.com/dgraph-io/dgraph/x"
)

var (
	workerIndices *Indices
)

// InitWorker sets workerIndices to the given Indices object.
func InitWorker(indices *Indices) {
	x.Assert(indices != nil)
	workerIndices = indices
}

// WorkerLookup looks up the indices and push the result to the given channel.
func WorkerLookup(li *LookupSpec, results chan *LookupResult) {
	// Will check instance here next time.
	results <- workerIndices.Lookup(li)
}

// FrontfillAdd inserts with overwrite (replace) key, value into our indices.
func FrontfillAdd(ctx context.Context, attr string, uid uint64, val string) {
	if err := workerIndices.FrontfillAdd(ctx, attr, uid, val); err != nil {
		x.TraceError(ctx, err)
	}
}

// FrontfillDel deletes a key, value from our indices.
func FrontfillDel(ctx context.Context, attr string, uid uint64) {
	if err := workerIndices.FrontfillDel(ctx, attr, uid); err != nil {
		x.TraceError(ctx, err)
	}
}
