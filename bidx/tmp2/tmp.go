package main

import (
	"fmt"
	"log"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
)

func main() {
	ps := new(store.Store)
	postingDir := "/home/jchiu/dgraph/p"
	if err := ps.InitReadOnly(postingDir); err != nil {
		log.Fatalf("error initializing postings store: %s", err)
	}
	defer ps.Close()
	fmt.Println("store opened")

	var lastPred string
	var count int
	it := ps.GetIterator()
	for it.SeekToFirst(); it.Valid(); it.Next() {
		_, pred := posting.DecodeKeyPartial(it.Key().Data())
		if pred != lastPred {
			fmt.Printf("%d [%s]\n", count, pred)
		}
		lastPred = pred
		count++
	}
	fmt.Printf("%d rows processed\n", count)
}
