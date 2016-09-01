// Backfill indices. For now, we assume a full reconstruction of the indices.
// In the future, we may expose ways to modify only parts of the index.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dgraph-io/dgraph/index"
	_ "github.com/dgraph-io/dgraph/index/indexer/memtable"
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
	x.Checkf(err, "Cannot open config file: %s", *configFilename)
	defer f.Close()
	config, err := index.ReadConfigs(f)
	x.Check(err)

	// Try writing to index directory.
	indices, err := index.CreateIndices(config, *indicesDir)
	x.Check(err)

	start := time.Now()
	err = indices.Backfill(context.Background(), ps)
	if err != nil {
		x.TraceError(context.Background(), err)
	}
	fmt.Printf("Elapsed %s\n", time.Since(start))
}
