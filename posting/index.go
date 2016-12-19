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

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
)

// TokensTable tracks the keys / tokens / buckets for an indexed attribute.
type TokensTable struct {
	sync.RWMutex
	key []string
}

var (
	indexLog trace.EventLog
	tables   map[string]*TokensTable
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
}

// initIndex initializes the index with the given data store.
func initIndex() {
	x.AssertTrue(pstore != nil)

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
			pk := x.ParsedKey{
				Attr: attr,
			}
			prefix := pk.IndexPrefix()

			it := pstore.NewIterator()
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				pki := x.Parse(it.Key().Data())
				x.AssertTrue(pki.IsIndex())
				x.AssertTrue(len(pki.Term) > 0)
				table.push(pki.Term)
			}
			results <- resultStruct{attr, table}
		}(attr)
	}

	tables = make(map[string]*TokensTable)
	for i := 0; i < len(indexedFields); i++ {
		r := <-results
		tables[r.attr] = r.table
	}

}

// IndexTokens return tokens, without the predicate prefix and index rune.
func IndexTokens(attr string, pID types.TypeID, data []byte) ([]string, error) {
	schemaType := schema.TypeOf(attr)
	if !schemaType.IsScalar() {
		return nil, x.Errorf("Cannot index attribute %s of type object.", attr)
	}
	s := schemaType.(types.TypeID)
	sv := types.ValueForType(s)
	src := types.ValueForType(pID)
	src.Value = data
	err := types.Convert(src, &sv)
	if err != nil {
		return nil, err
	}
	switch v := sv.Value.(type) {
	case geom.T:
		return geo.IndexTokens(&v)
	case int32:
		return types.IntIndex(attr, &v)
	case float64:
		return types.FloatIndex(attr, &v)
	case time.Time:
		if s == types.DateID {
			return types.DateIndex(attr, &v)
		}
		return types.TimeIndex(attr, &v)
	case string:
		return types.DefaultIndexKeys(attr, &v), nil
	default:
		return nil, x.Errorf("Invalid type. Cannot be indexed")
	}
	return nil, nil
}

// addIndexMutations adds mutation(s) for a single term, to maintain index.
func addIndexMutations(ctx context.Context, attr string, uid uint64,
	typ types.TypeID, data []byte, del bool) {
	x.AssertTrue(uid != 0)
	dataType := types.TypeID(typ)
	tokens, err := IndexTokens(attr, dataType, data)
	if err != nil {
		// This data is not indexable
		return
	}
	edge := &task.DirectedEdge{
		ValueId: uid,
		Attr:    attr,
		Label:   "idx",
	}

	tokensTable := GetTokensTable(attr)
	x.AssertTruef(tokensTable != nil, "TokensTable missing for attr %s", attr)

	for _, token := range tokens {
		addIndexMutation(ctx, attr, token, tokensTable, edge, del)
	}
}

func addIndexMutation(ctx context.Context, attr, token string,
	tokensTable *TokensTable, edge *task.DirectedEdge, del bool) {
	key := x.IndexKey(attr, token)
	plist, decr := GetOrCreate(key)
	defer decr()

	x.AssertTruef(plist != nil, "plist is nil [%s] %d %s",
		token, edge.ValueId, edge.Attr)
	if del {
		_, err := plist.AddMutation(ctx, edge, Del)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err,
				"Error deleting %s for attr %s entity %d: %v",
				token, edge.Attr, edge.Entity))
		}
		indexLog.Printf("DEL [%s] [%d] OldTerm [%s]",
			edge.Attr, edge.Entity, token)

	} else {
		_, err := plist.AddMutation(ctx, edge, Set)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err,
				"Error adding %s for attr %s entity %d: %v",
				token, edge.Attr, edge.Entity))
		}
		indexLog.Printf("SET [%s] [%d] NewTerm [%s]",
			edge.Attr, edge.Entity, token)

		tokensTable.Add(token)
	}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t *task.DirectedEdge, op uint32) error {
	x.AssertTruef(len(t.Attr) > 0,
		"[%s] [%d] [%v] %d %d\n", t.Attr, t.Entity, t.Value, t.ValueId, op)

	var vbytes []byte
	var vtype types.TypeID
	var typ byte
	var verr error

	doUpdateIndex := pstore != nil && (t.Value != nil) &&
		schema.IsIndexed(t.Attr)
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		vbytes, typ, verr = l.Value()
		vtype = types.TypeID(typ)
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
		addIndexMutations(ctx, t.Attr, t.Entity, vtype, delTerm, true)
	}
	if op == Set {
		addIndexMutations(ctx, t.Attr, t.Entity, types.TypeID(t.ValueType), t.Value, false)
	}
	return nil
}

// GetTokensTable returns TokensTable for an indexed attribute.
func GetTokensTable(attr string) *TokensTable {
	x.AssertTruef(tables != nil,
		"TokensTable uninitialized. You need to call InitIndex.")
	return tables[attr]
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

// GetNext returns the next key after given key. If we reach the end, we
// return an empty string.
func (t *TokensTable) GetNext(key string) string {
	t.RLock()
	defer t.RUnlock()
	i := sort.Search(len(t.key),
		func(i int) bool { return t.key[i] > key })
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

// GetPrev returns the key just before the given key. If we reach the start,
// we return an empty string.
func (t *TokensTable) GetPrev(key string) string {
	t.RLock()
	defer t.RUnlock()
	n := len(t.key)
	i := sort.Search(len(t.key),
		func(i int) bool { return t.key[n-i-1] < key })
	if i < len(t.key) {
		return t.key[n-i-1]
	}
	return ""
}

// GetLast returns the first key in our list of keys. You could also call
// GetPrev("") but that is less efficient.
func (t *TokensTable) GetLast() string {
	t.RLock()
	defer t.RUnlock()
	if len(t.key) == 0 {
		// Assume all keys are nonempty. Returning empty string means there's no keys.
		return ""
	}
	return t.key[len(t.key)-1]
}

// GetNextOrEqual returns position of leftmost element that is greater or equal to s.
func (t *TokensTable) GetNextOrEqual(s string) string {
	t.RLock()
	defer t.RUnlock()
	n := len(t.key)
	i := sort.Search(n, func(i int) bool { return t.key[i] >= s })
	if i < n {
		return t.key[i]
	}
	return ""
}

// GetPrevOrEqual returns position of rightmost element that is smaller or equal to s.
func (t *TokensTable) GetPrevOrEqual(s string) string {
	t.RLock()
	defer t.RUnlock()
	n := len(t.key)
	i := sort.Search(n, func(i int) bool { return t.key[n-i-1] <= s })
	if i < n {
		return t.key[n-i-1]
	}
	return ""
}
