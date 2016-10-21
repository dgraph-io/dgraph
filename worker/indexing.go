package worker

import (
	"golang.org/x/net/context" // Need this for the time being.

	"github.com/dgraph-io/dgraph/index"
	"github.com/dgraph-io/dgraph/x"
)

func InitIndex() {
	go processIndexMutations()
}

func processIndexMutations() {
	ctx := context.Background()
	for m := range index.MutateChan {
		x.Check(MutateOverNetwork(ctx, m))
	}
}
