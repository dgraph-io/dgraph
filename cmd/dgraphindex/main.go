// Backfill indices. For now, we assume a full reconstruction of the indices.
// In the future, we may expose ways to modify only parts of the index.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dgraph-io/dgraph/bidx"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var (
	configFilename = flag.String("config", "", "Filename of JSON config file.")
	postingDir     = flag.String("postings", "p", "Directory to store posting lists")
	indicesDir     = flag.String("indices", "i", "Directory to store indices")
)

func main() {
	x.Init()

	// Open posting store as read-only.
	ps := new(store.Store)
	x.Check(ps.InitReadOnly(*postingDir))
	defer ps.Close()

	// Read in the config file.
	f, err := os.Open(*configFilename)
	x.Check(err)
	defer f.Close()
	config, err := bidx.NewIndicesConfig(f)
	x.Check(err)

	// Try writing to index directory.
	x.Check(bidx.CreateIndices(config, *indicesDir))

	indices, err := bidx.NewIndices(*indicesDir)
	x.Check(err)

	start := time.Now()
	indices.Backfill(ps)
	fmt.Printf("Elapsed %s\n", time.Since(start))
}
