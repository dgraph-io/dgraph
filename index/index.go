package index

import (
	"sort"
	"sync"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// Posting list keys are prefixed with this rune if it is a mutation meant for
	// the index.
	indexRune = ':'
)

var (
	MutateChan   chan x.Mutations
	TokensTables map[string]*TokensTable
)

type TokensTable struct {
	sync.RWMutex
	key []string
}

func init() {
	MutateChan = make(chan x.Mutations, 100)
}

// InitIndex iterates through store to get the keys into memory.
func InitIndex(dataStore *store.Store) {
	indexedFields := schema.IndexedFields()
	x.Printf("~~~~~~~~index.InitIndex: %d", len(indexedFields))
	type resultStruct struct {
		attr  string
		table *TokensTable
	}
	results := make(chan resultStruct, len(indexedFields))

	for _, attr := range indexedFields {
		go func(attr string) {
			table := NewTokensTable()
			prefix := types.IndexKey(attr, "")
			x.Printf("~~~~index.InitIndex: seeking to prefix=[%s]", prefix)

			it := dataStore.NewIterator()
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				token := types.TokenFromKey(it.Key().Data())
				table.append(string(token))
				x.Printf("~~~index.InitIndex: attr=%s token=%s or %v", attr, token, []byte(token))
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

// append appends a key to the table. It assumes that this key is the largest
// and that order is preserved.
func (t *TokensTable) append(s string) {
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
	kt := GetTokensTable(attr)
	kt.RLock()
	defer kt.RUnlock()
	return kt.key
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
	x.Assert(len(t.key) > 0)
	return t.key[0]
}
