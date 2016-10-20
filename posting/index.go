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

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/index"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	stype "github.com/dgraph-io/dgraph/types"
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

// indexTokens return tokens, without the predicate prefix and index rune.
func indexTokens(attr string, p stype.Value) ([]string, error) {
	schemaType := schema.TypeOf(attr)
	if !schemaType.IsScalar() {
		return nil, x.Errorf("Cannot index attribute %s of type object.", attr)
	}
	s := schemaType.(stype.Scalar)
	schemaVal, err := s.Convert(p)
	if err != nil {
		return nil, err
	}
	switch v := schemaVal.(type) {
	case *stype.Geo: // Not supported now.
		return geo.IndexKeys(v)
	case *stype.Int32:
		return stype.IntIndex(attr, v)
	case *stype.Float:
		return stype.FloatIndex(attr, v)
	case *stype.Date:
		return stype.DateIndex(attr, v)
	case *stype.Time:
		return stype.TimeIndex(attr, v)
	case *stype.String:
		return stype.DefaultIndexKeys(attr, v), nil
	}
	return nil, nil
}

// spawnIndexMutations adds mutations for a single term, to maintain index.
func spawnIndexMutations(ctx context.Context, attr string, uid uint64, p stype.Value, del bool) {
	x.Assert(uid != 0)
	tokens, err := indexTokens(attr, p)
	//	x.Printf("~~~~~~~processIndexTerm numTokens=%d", len(tokens))
	if err != nil {
		// This data is not indexable
		return
	}
	edge := x.DirectedEdge{
		Timestamp: time.Now(),
		ValueId:   uid,
		Attribute: attr,
		Source:    "idx",
	}

	for _, token := range tokens {
		//		x.Printf("~~~~~~~processIndexTerm attr=%s token=[%s] length=%d %d", attr, string(token), len(token), uid)
		edge.IndexToken = token

		//		key := KeyFromEdge(&edge)
		//		plist, decr := GetOrCreate(key, indexStore)
		//		defer decr()
		//		x.Assertf(plist != nil, "plist is nil [%s] %d %s", key, edge.ValueId, edge.Attribute)

		//		if del {
		//			_, err := plist.AddMutation(ctx, edge, Del)
		//			if err != nil {
		//				x.TraceError(ctx, x.Wrapf(err, "Error deleting %s for attr %s entity %d: %v",
		//					string(key), edge.Attribute, edge.Entity))
		//			}
		//			indexLog.Printf("DEL [%s] [%d] OldTerm [%s]", edge.Attribute, edge.Entity, string(key))
		//		} else {
		//			_, err := plist.AddMutation(ctx, edge, Set)
		//			if err != nil {
		//				x.TraceError(ctx, x.Wrapf(err, "Error adding %s for attr %s entity %d: %v",
		//					string(key), edge.Attribute, edge.Entity))
		//			}
		//			indexLog.Printf("SET [%s] [%d] NewTerm [%s]", edge.Attribute, edge.Entity, string(key))
		//		}
		if del {
			index.MutateChan <- x.Mutations{
				Del: []x.DirectedEdge{edge},
			}
			indexLog.Printf("processIndexTerm DEL [%s] [%d] OldTerm [%s]", edge.Attribute, edge.Entity, edge.IndexToken)
			x.Printf("~~~processIndexTerm DEL [%s] [%d] OldTerm [%s]", edge.Attribute, edge.ValueId, edge.IndexToken)
		} else {
			index.MutateChan <- x.Mutations{
				Set: []x.DirectedEdge{edge},
			}
			indexLog.Printf("processIndexTerm SET [%s] [%d] NewTerm [%s]", edge.Attribute, edge.Entity, edge.IndexToken)
			x.Printf("~~~processIndexTerm SET [%s] [%d] NewTerm [%s]", edge.Attribute, edge.ValueId, edge.IndexToken)
		}
	}
}

// AddMutationWithIndex is AddMutation with support for indexing. Note that
// index mutations will also come through here.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	x.Assertf(len(t.Attribute) > 0, "Attribute should always be nonempty")

	// Temporary check. Remove later on.
	x.Assertf(!stype.IsIndexKey(t.Attribute),
		"Attribute should never begin with indexRune: %s", t.Attribute)

	var lastPost types.Posting
	var hasLastPost bool

	isIndexedAttr := schema.IsIndexed(t.Attribute)

	// Update list of tokens / buckets.
	if len(t.IndexToken) > 0 && op == Set {
		x.Assertf(isIndexedAttr, "Attribute should be indexed: %s", t.Attribute)
		index.GetTokensTable(t.Attribute).Add(t.IndexToken)
	}

	doUpdateIndex := indexStore != nil && (t.Value != nil) && isIndexedAttr &&
		(len(t.IndexToken) == 0)
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

	// Exact matches.
	if hasLastPost && lastPost.ValueBytes() != nil {
		delTerm := lastPost.ValueBytes()
		delType := lastPost.ValType()
		p := stype.ValueForType(stype.TypeID(delType))
		err = p.UnmarshalBinary(delTerm)
		if err != nil {
			return err
		}
		spawnIndexMutations(ctx, t.Attribute, t.Entity, p, true)
	}
	if op == Set {
		p := stype.ValueForType(stype.TypeID(t.ValueType))
		err := p.UnmarshalBinary(t.Value)
		if err != nil {
			return err
		}
		spawnIndexMutations(ctx, t.Attribute, t.Entity, p, false)
	}
	return nil
}
