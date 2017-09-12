package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"

	"github.com/dgraph-io/dgraph/x"
)

var (
	blockRate = flag.Int("block", 0, "Block profiling rate")
)

func main() {

	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

	var opt options
	flag.StringVar(&opt.rdfFiles, "r", "", "Location of rdf files to load (comma separated)")
	flag.StringVar(&opt.schemaFile, "s", "", "Location of schema file to load")
	flag.StringVar(&opt.badgerDir, "p", "p", "Location of the final Dgraph directory")
	flag.StringVar(&opt.leaseFile, "l", "LEASE", "Location to write the lease file")
	flag.StringVar(&opt.tmpDir, "tmp", "tmp", "Temp directory used to use for on-disk "+
		"scratch space. Requires free space proportional to the size of the RDF file.")
	flag.IntVar(&opt.numGoroutines, "j", runtime.NumCPU(),
		"Number of worker threads to use (defaults to one less than logical CPUs)")
	flag.Int64Var(&opt.mapBufSize, "mapoutput_mb", 128,
		"The estimated size of each map file output. This directly affects the memory usage.")
	httpAddr := flag.String("http", "localhost:8080", "Address to serve http (pprof)")
	skipMapPhase := flag.Bool("skip_map_phase", false,
		"Skip the map phase (assumes that map output files already exist)")
	cleanUpTmp := flag.Bool("cleanup_tmp", true,
		"Clean up the tmp directory after the loader finishes")
	flag.BoolVar(&opt.skipExpandEdges, "skip_expand_edges", false,
		"Don't generate edges that allow nodes to be expanded using _predicate_ or expand(...).")

	flag.Parse()
	if len(flag.Args()) != 0 {
		flag.Usage()
		fmt.Println("No free args allowed, but got:", flag.Args())
		os.Exit(1)
	}
	if opt.rdfFiles == "" || opt.schemaFile == "" {
		flag.Usage()
		fmt.Println("RDF and schema file(s) must be specified.")
		os.Exit(1)
	}

	opt.mapBufSize = opt.mapBufSize << 20 // Convert from MB to B.

	go func() {
		log.Fatal(http.ListenAndServe(*httpAddr, nil))
	}()
	if *blockRate > 0 {
		runtime.SetBlockProfileRate(*blockRate)
	}

	// Ensure the badger output dir is empty.
	x.Check(os.RemoveAll(opt.badgerDir))
	x.Check(os.MkdirAll(opt.badgerDir, 0700))

	// Create a directory just for bulk loader's usage.
	if !*skipMapPhase {
		x.Check(os.RemoveAll(opt.tmpDir))
		x.Check(os.MkdirAll(opt.tmpDir, 0700))
	}
	if *cleanUpTmp {
		defer os.RemoveAll(opt.tmpDir)
	}

	loader := newLoader(opt)
	if !*skipMapPhase {
		loader.mapStage()
	}
	loader.reduceStage()
	loader.writeSchema()
	loader.cleanup()
}
