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

package posting

import (
	"context"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var (
	indexLog   trace.EventLog
	indexStore *store.Store
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
}

// InitIndex initializes the index with the given data store.
func InitIndex(ds *store.Store) {
	if ds == nil {
		return
	}
	indexStore = ds
}

func processIndexTerm(ctx context.Context, keygen schema.IndexKeyGenerator, attr string, uid uint64, term []byte, del bool) {
	x.Assert(uid != 0)
	keys, err := keygen.IndexKeys(attr, term)
	if err != nil {
		// This data is not indexable
		return
	}
	for _, key := range keys {
		edge := x.DirectedEdge{
			Timestamp: time.Now(),
			ValueId:   uid,
			Attribute: attr,
			Source:    "idx",
		}
		plist, decr := GetOrCreate([]byte(key), indexStore)
		defer decr()
		x.Assertf(plist != nil, "plist is nil [%s] %d %s", key, edge.ValueId, edge.Attribute)

		if del {
			_, err := plist.AddMutation(ctx, edge, Del)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error deleting %s for attr %s entity %d: %v",
					string(key), edge.Attribute, edge.Entity))
			}
			indexLog.Printf("DEL [%s] [%d] OldTerm [%s]", edge.Attribute, edge.Entity, string(key))
		} else {
			_, err := plist.AddMutation(ctx, edge, Set)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error adding %s for attr %s entity %d: %v",
					string(key), edge.Attribute, edge.Entity))
			}
			indexLog.Printf("SET [%s] [%d] NewTerm [%s]", edge.Attribute, edge.Entity, string(key))
		}
	}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	x.Assertf(len(t.Attribute) > 0 && t.Attribute[0] != schema.IndexRune,
		"[%s] [%d] [%s] %d %d\n", t.Attribute, t.Entity, string(t.Value), t.ValueId, op)

	var lastPost types.Posting
	var hasLastPost bool
	doUpdateIndex := indexStore != nil && (t.Value != nil) && schema.IsIndexed(t.Attribute)
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		if l.Length() >= 1 {
			x.Assert(l.Get(&lastPost, l.Length()-1))
			hasLastPost = true
		}
	}
	hasMutated, err := l.AddMutation(ctx, t, op)
	if err != nil {
		return err
	}
	if !hasMutated || !doUpdateIndex {
		return nil
	}
	keygen := schema.GetKeygen(t.Attribute)
	if hasLastPost && lastPost.ValueBytes() != nil {
		processIndexTerm(ctx, keygen, t.Attribute, t.Entity, lastPost.ValueBytes(), true)
	}
	if op == Set {
		processIndexTerm(ctx, keygen, t.Attribute, t.Entity, t.Value, false)
	}
	return nil
}
