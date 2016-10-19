package index

import (
	"github.com/dgraph-io/dgraph/x"
)

var (
	MutateChan chan x.Mutations
)

func init() {
	MutateChan = make(chan x.Mutations, 100)
}
