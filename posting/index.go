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
	"sort"
	"sync"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/schema"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// TokensTable tracks the keys / tokens / buckets for an indexed attribute.
type TokensTable struct {
	sync.RWMutex
	key []string
}

var (
	indexLog     trace.EventLog
	TokensTables map[string]*TokensTable
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
}

// initIndex initializes the index with the given data store.
func initIndex() {
	x.Assert(pstore != nil)

	// Initialize TokensTables.
	indexedFields := schema.IndexedFields()
	type resultStruct struct {
		attr  string
		table *TokensTable
	}
	results := make(chan resultStruct, len(indexedFields))

	for _, attr := range indexedFields {
		go func(attr string) {
			table := &TokensTable{
				key: make([]string, 0, 50),
			}
			prefix := stype.IndexKey(attr, "")

			it := pstore.NewIterator()
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				table.push(stype.TokenFromKey(it.Key().Data()))
			}
			results <- resultStruct{attr, table}
		}(attr)
	}

	TokensTables = make(map[string]*TokensTable)
	for i := 0; i < len(indexedFields); i++ {
		r := <-results
		TokensTables[r.attr] = r.table
	}

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
	// TODO: Geo is currently disabled. We like tokenizers to not mess with
	// keys. However, Geo seems to require that.
	//	case *stype.Geo:
	//		return geo.IndexTokens(v)
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

// addIndexMutations adds mutation(s) for a single term, to maintain index.
func addIndexMutations(ctx context.Context, attr string, uid uint64,
	p stype.Value, del bool) {
	x.Assert(uid != 0)
	tokens, err := indexTokens(attr, p)
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

	tokensTable := GetTokensTable(attr)
	x.Assertf(tokensTable != nil, "TokensTable missing for attr %s", attr)

	for _, token := range tokens {
		addIndexMutation(ctx, attr, token, tokensTable, &edge, del)
	}
}

func addIndexMutation(ctx context.Context, attr, token string,
	tokensTable *TokensTable, edge *x.DirectedEdge, del bool) {
	plist, decr := GetOrCreate(stype.IndexKey(attr, token))
	defer decr()

	x.Assertf(plist != nil, "plist is nil [%s] %d %s",
		token, edge.ValueId, edge.Attribute)
	if del {
		_, err := plist.AddMutation(ctx, *edge, Del)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err,
				"Error deleting %s for attr %s entity %d: %v",
				token, edge.Attribute, edge.Entity))
		}
		indexLog.Printf("DEL [%s] [%d] OldTerm [%s]",
			edge.Attribute, edge.Entity, token)
	} else {
		_, err := plist.AddMutation(ctx, *edge, Set)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err,
				"Error adding %s for attr %s entity %d: %v",
				token, edge.Attribute, edge.Entity))
		}
		indexLog.Printf("SET [%s] [%d] NewTerm [%s]",
			edge.Attribute, edge.Entity, token)

		tokensTable.Add(token)
	}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	x.Assertf(len(t.Attribute) > 0 && t.Attribute[0] != ':',
		"[%s] [%d] [%s] %d %d\n", t.Attribute, t.Entity, string(t.Value), t.ValueId, op)

	var vbytes []byte
	var vtype byte
	var verr error

	doUpdateIndex := pstore != nil && (t.Value != nil) &&
		schema.IsIndexed(t.Attribute)
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		vbytes, vtype, verr = l.Value()
	}
	hasMutated, err := l.AddMutation(ctx, t, op)
	if err != nil {
		return err
	}
	if !hasMutated || !doUpdateIndex {
		return nil
	}

	// Exact matches.
	if verr == nil && len(vbytes) > 0 {
		delTerm := vbytes
		delType := vtype
		p := stype.ValueForType(stype.TypeID(delType))
		err = p.UnmarshalBinary(delTerm)
		if err != nil {
			return err
		}
		addIndexMutations(ctx, t.Attribute, t.Entity, p, true)
	}
	if op == Set {
		p := stype.ValueForType(stype.TypeID(t.ValueType))
		err := p.UnmarshalBinary(t.Value)
		if err != nil {
			return err
		}
		addIndexMutations(ctx, t.Attribute, t.Entity, p, false)
	}
	return nil
}

// GetTokensTable returns TokensTable for an indexed attribute.
func GetTokensTable(attr string) *TokensTable {
	x.Assertf(TokensTables != nil,
		"TokensTable uninitialized. You need to call InitIndex.")
	return TokensTables[attr]
}

// NewTokensTable returns a new TokensTable.
func NewTokensTable() *TokensTable {
	return &TokensTable{
		key: make([]string, 0, 50),
	}
}

// Get returns position of element. If not found, it returns -1.
func (t *TokensTable) Get(s string) int {
	t.RLock()
	defer t.RUnlock()
	i := sort.SearchStrings(t.key, s)
	if i < len(t.key) && t.key[i] == s {
		return i
	}
	return -1
}

// Add increments counter for a given key. If it doesn't exist, we create a
// new entry in TokensTable. We don't support delete yet. We are using a very
// simple implementation. In the future, as balanced trees / skip lists
// implementations become standardized for Go, we may consider using them.
// We also didn't support Delete operations yet. For that, we need to store
// the counts for each key.
func (t *TokensTable) Add(s string) {
	t.Lock()
	defer t.Unlock()
	i := sort.SearchStrings(t.key, s)
	if i < len(t.key) && t.key[i] == s {
		return
	}
	t.key = append(t.key, "")
	for j := len(t.key) - 1; j > i; j-- {
		t.key[j] = t.key[j-1]
	}
	t.key[i] = s
}

// push appends a key to the table. It assumes that this key is the largest
// and that order is preserved.
func (t *TokensTable) push(s string) {
	t.Lock()
	defer t.Unlock()
	t.key = append(t.key, s)
}

// Size returns size of TokensTable.
func (t *TokensTable) Size() int {
	t.RLock()
	defer t.RUnlock()
	return len(t.key)
}

// KeysForTest returns keys for a table. This is just for testing / debugging.
func KeysForTest(attr string) []string {
	t := GetTokensTable(attr)
	t.RLock()
	defer t.RUnlock()
	return t.key
}

// GetNextKey returns the next key after given key. If we reach the end, we
// return an empty string.
func (t *TokensTable) GetNext(key string) string {
	t.RLock()
	defer t.RUnlock()
	i := sort.Search(len(t.key),
		func(i int) bool {
			return t.key[i] > key
		})
	if i < len(t.key) {
		return t.key[i]
	}
	return ""
}

// GetFirst returns the first key in our list of keys. You could also call
// GetNext("") but that is less efficient.
func (t *TokensTable) GetFirst() string {
	t.RLock()
	defer t.RUnlock()
	if len(t.key) == 0 {
		// Assume all keys are nonempty. Returning empty string means there's no keys.
		return ""
	}
	return t.key[0]
}
