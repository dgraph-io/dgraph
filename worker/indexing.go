package worker

import (
	"golang.org/x/net/context" // Need this for the time being.

	"github.com/dgraph-io/dgraph/index"
	"github.com/dgraph-io/dgraph/x"
)

func InitIndex() {
	go processIndexMutations()
}

func printMutations(m x.Mutations) {
	if len(m.Set) > 0 {
		a := m.Set[0]
		x.Printf("~~SET [%s] [%s] [%d] [%s]", a.Attribute, string(a.Value), a.ValueId, string(a.Key))
	} else {
		a := m.Del[0]
		x.Printf("~~DEL [%s] [%s] [%d] [%s]", a.Attribute, string(a.Value), a.ValueId, string(a.Key))
	}
}

func processIndexMutations() {
	ctx := context.Background()
	for m := range index.MutateChan {
		printMutations(m)
		x.Check(MutateOverNetwork(ctx, m))
	}
}
