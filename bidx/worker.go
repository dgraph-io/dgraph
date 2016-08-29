// TODO(jiawei)
// This is supposed to send out RPC calls if server instance does not handle
// given attribute / predicate.
package bidx

import (
	"log"
	"os"
)

var (
	globalIndices *Indices
)

// InitWorker initializes Bleve indices for this instance.
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

// WorkerLookup looks up the indices and push the result to the given channel.
func WorkerLookup(li *LookupSpec, results chan *LookupResult) {
	// Will check instance here next time.
	results <- globalIndices.Lookup(li)
}
