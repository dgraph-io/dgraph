package index

import (
	"sort"
	"sync"

	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	MutateChan chan x.Mutations
	keysTables map[string]*keysTable
)

type keysTable struct {
	sync.RWMutex
	key []string
}

func init() {
	MutateChan = make(chan x.Mutations, 100)
}

// InitIndex iterates through store to get the keys into memory.
func InitIndex(dataStore *store.Store) {
	indexedFields := schema.IndexedFields()
	type resultStruct struct {
		attr  string
		table *keysTable
	}
	results := make(chan resultStruct, len(indexedFields))

	for _, attr := range indexedFields {
		go func(attr string) {
			kt := NewKeysTable()
			prefix := types.IndexKey(attr, nil)
			it := dataStore.NewIterator()
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				token := types.TokenFromKey(it.Key().Data())
				kt.Add(string(token))
			}
			results <- resultStruct{attr, kt}
		}(attr)
	}
}

// NewKeysTable returns a new keysTable.
func NewKeysTable() *keysTable {
	return &keysTable{
		key: make([]string, 0, 50),
	}
}

// Get returns position of element. If not found, it returns -1.
func (t *keysTable) Get(s string) int {
	t.RLock()
	defer t.RUnlock()
	i := sort.SearchStrings(t.key, s)
	if i < len(t.key) && t.key[i] == s {
		return i
	}
	return -1
}

// Add increments counter for a given key. If it doesn't exist, we create a
// new entry in keysTable. We don't support delete yet.
func (t *keysTable) Add(s string) {
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

// Size returns size of keysTable.
func (t *keysTable) Size() int {
	t.RLock()
	defer t.RUnlock()
	return len(t.key)
}
