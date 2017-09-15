package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"

	"github.com/dgraph-io/dgraph/x"
)

func main() {

	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

	var opt options
	flag.IntVar(&opt.BlockRate, "block", 0, "Block profiling rate")
	flag.StringVar(&opt.RDFDir, "r", "", "Directory containing *.rdf or *.rdf.gz files to load")
	flag.StringVar(&opt.SchemaFile, "s", "", "Location of schema file to load")
	flag.StringVar(&opt.BadgerDir, "p", "p", "Location of the final Dgraph directory")
	flag.StringVar(&opt.LeaseFile, "l", "LEASE", "Location to write the lease file")
	flag.StringVar(&opt.TmpDir, "tmp", "tmp", "Temp directory used to use for on-disk "+
		"scratch space. Requires free space proportional to the size of the RDF file.")
	flag.IntVar(&opt.NumGoroutines, "j", runtime.NumCPU(),
		"Number of worker threads to use (defaults to one less than logical CPUs)")
	flag.Int64Var(&opt.MapBufSize, "mapoutput_mb", 128,
		"The estimated size of each map file output. This directly affects the memory usage.")
	httpAddr := flag.String("http", "localhost:8080", "Address to serve http (pprof)")
	flag.BoolVar(&opt.SkipMapPhase, "skip_map_phase", false,
		"Skip the map phase (assumes that map output files already exist)")
	flag.BoolVar(&opt.CleanupTmp, "cleanup_tmp", true,
		"Clean up the tmp directory after the loader finishes")
	flag.BoolVar(&opt.SkipExpandEdges, "skip_expand_edges", false,
		"Don't generate edges that allow nodes to be expanded using _predicate_ or expand(...).")
	flag.IntVar(&opt.NumShards, "shards", 1, "Number map phase output shards.")
	flag.IntVar(&opt.MaxPendingBadgerWrites, "max_pending_badger_writes", 1000,
		"Maximum number of pending badger writes allowed at any time.")
	flag.IntVar(&opt.NumShufflers, "shufflers", 1, "Number of shufflers to run concurrently.")

	flag.Parse()
	if len(flag.Args()) != 0 {
		flag.Usage()
		fmt.Println("No free args allowed, but got:", flag.Args())
		os.Exit(1)
	}
	if opt.RDFDir == "" || opt.SchemaFile == "" {
		flag.Usage()
		fmt.Println("RDF and schema file(s) must be specified.")
		os.Exit(1)
	}

	opt.MapBufSize = opt.MapBufSize << 20 // Convert from MB to B.

	optBuf, err := json.MarshalIndent(&opt, "", "\t")
	x.Check(err)
	fmt.Println(string(optBuf))

	go func() {
		log.Fatal(http.ListenAndServe(*httpAddr, nil))
	}()
	if opt.BlockRate > 0 {
		runtime.SetBlockProfileRate(opt.BlockRate)
	}

	// Ensure the badger output dir is empty.
	x.Check(os.RemoveAll(opt.BadgerDir))
	x.Check(os.MkdirAll(opt.BadgerDir, 0700))

	// Create a directory just for bulk loader's usage.
	if !opt.SkipMapPhase {
		x.Check(os.RemoveAll(opt.TmpDir))
		x.Check(os.MkdirAll(opt.TmpDir, 0700))
	}
	if opt.CleanupTmp {
		defer os.RemoveAll(opt.TmpDir)
	}

	loader := newLoader(opt)
	if !opt.SkipMapPhase {
		loader.mapStage()
	}
	loader.reduceStage()
	loader.writeSchema()
	loader.cleanup()
}
