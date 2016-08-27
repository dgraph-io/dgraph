// Backfill indices. For now, we assume a full reconstruction of the indices.
// In the future, we may expose ways to modify only parts of the index.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/dgraph-io/dgraph/bidx"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var (
	indicesDir     = flag.String("indices", "i", "Directory to store indices.")
	configFilename = flag.String("config", "", "Filename of JSON config file.")
	postingDir     = flag.String("postings", "p", "Directory to store posting lists")
)

func main() {
	log.SetFlags(log.Lshortfile | log.Flags())
	flag.Parse()
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}
	if ok := x.PrintVersionOnly(); ok {
		return
	}

	// Open posting store as read-only.
	ps := new(store.Store)
	if err := ps.InitReadOnly(*postingDir); err != nil {
		log.Fatalf("Error initializing postings store: %s", err)
	}
	defer ps.Close()

	// Read in the config file.
	config, err := bidx.NewIndicesConfig(*configFilename)
	if err != nil {
		log.Fatalf("Error opening JSON config: %s", err)
	}

	// Try writing to index directory.
	err = bidx.CreateIndices(config, *indicesDir)
	if err != nil {
		log.Fatal(err)
	}

	indices, err := bidx.NewIndices(*indicesDir)
	if err != nil {
		log.Fatalf("Failed to open indices: %s", err)
	}

	start := time.Now()
	indices.Backfill(ps)
	fmt.Printf("Elapsed %s\n", time.Since(start))
}
