// TODO: Expand to multiserver. Use grpc.
package bidx

import (
	"log"
	"os"
)

var (
	globalIndices *Indices
)

func InitWorker(indicesDir string) *Indices {
	err := os.MkdirAll(indicesDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for indices: %v", err)
	}

	globalIndices, err = NewIndices(indicesDir)
	if err != nil {
		log.Fatalf("Error initializing indices store: %s", err)
	}
	return globalIndices
}

func WorkerLookup(li *LookupSpec, results chan *LookupResult) {
	// Will check instance here next time.
	results <- globalIndices.Lookup(li)
}
