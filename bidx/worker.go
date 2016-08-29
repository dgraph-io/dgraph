// TODO(jiawei)
// This is supposed to send out RPC calls if server instance does not handle
// given attribute / predicate.
package bidx

import (
	"log"
	"os"

	"github.com/dgraph-io/dgraph/x"
)

var (
	globalIndices *Indices
)

// InitWorker initializes Bleve indices for this instance. Returns the Indices
// constructed. You may need this in tests. Most of the time, you can ignore.
func InitWorker(indicesDir string) *Indices {
	// Create indices directory if it is missing.
	err := os.MkdirAll(indicesDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for indices: %v", err)
	}

	// Read in the indices.
	globalIndices, err = NewIndices(indicesDir)
	x.Check(err)

	globalIndices.initFrontfill()
	return globalIndices
}

// WorkerLookup looks up the indices and push the result to the given channel.
func WorkerLookup(li *LookupSpec, results chan *LookupResult) {
	// Will check instance here next time.
	results <- globalIndices.Lookup(li)
}
