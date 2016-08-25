// Given schema and our graphDB data, we want to build and write the indices.
package bidx

import (
	"bytes"
	"fmt"
	"log"

	"github.com/dgraph-io/dgraph/store"
)

func Backfill(ps *store.Store, schema *Schema) {
	it := ps.GetIterator()
	var count int
	var lastPred string
	for it.SeekToFirst(); it.Valid(); it.Next() {
		b := it.Key().Data()
		buf := bytes.NewBuffer(b)
		a, err := buf.ReadString('|')
		if err != nil {
			log.Fatalf("Error backfill: %v", b)
		}
		pred := string(a[:len(a)-1]) // omit the trailing '|'
		if pred != lastPred {
			fmt.Printf("%d [%s]\n", count, pred)
		}
		lastPred = pred

		// To remove later.
		{
			count++
			if count >= 1000000 {
				break
			}
		}
	}
	fmt.Printf("%d rows processed\n", count)
}
