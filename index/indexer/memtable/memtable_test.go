package memtable

import (
	"testing"

	"github.com/dgraph-io/dgraph/index/indexer"
)

func TestAll(t *testing.T) {
	indexer.TestBasic(New(), t)
}
