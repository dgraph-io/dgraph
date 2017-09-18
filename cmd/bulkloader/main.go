package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

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
	flag.StringVar(&opt.DgraphsDir, "out", "out",
		"Location to write the final dgraph data directories.")
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
	flag.IntVar(&opt.MaxPendingBadgerWrites, "max_pending_badger_writes", 1000,
		"Maximum number of pending badger writes allowed at any time.")
	flag.IntVar(&opt.NumShufflers, "shufflers", 1, "Number of shufflers to run concurrently.")
	flag.IntVar(&opt.MapShards, "map_shards", 1, "Number of map output shards.")
	flag.IntVar(&opt.ReduceShards, "reduce_shards", 1, "Number of shuffle output shards.")

	flag.Parse()
	if len(flag.Args()) != 0 {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "No free args allowed, but got: %v\n", flag.Args())
		os.Exit(1)
	}
	if opt.RDFDir == "" || opt.SchemaFile == "" {
		flag.Usage()
		fmt.Fprint(os.Stderr, "RDF and schema file(s) must be specified.\n")
		os.Exit(1)
	}
	if opt.ReduceShards > opt.MapShards {
		fmt.Fprintf(os.Stderr, "Invalid flags: reduce_shards(%d) should be <= map_shards(%d)\n",
			opt.ReduceShards, opt.MapShards)
		os.Exit(1)
	}
	if opt.NumShufflers > opt.ReduceShards {
		fmt.Fprintf(os.Stderr, "Invalid flags: shufflers(%d) should be <= reduce_shards(%d)\n",
			opt.NumShufflers, opt.ReduceShards)
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

	// Delete and recreate the output dirs to ensure they are empty.
	x.Check(os.RemoveAll(opt.DgraphsDir))
	for i := 0; i < opt.ReduceShards; i++ {
		dir := filepath.Join(opt.DgraphsDir, strconv.Itoa(i))
		x.Check(os.MkdirAll(dir, 0700))
		opt.shardOutputDirs = append(opt.shardOutputDirs, dir)
	}

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
