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
	flag.IntVar(&opt.BlockRate, "block", 0,
		"Block profiling rate.")
	flag.StringVar(&opt.RDFDir, "r", "",
		"Directory containing *.rdf or *.rdf.gz files to load.")
	flag.StringVar(&opt.SchemaFile, "s", "",
		"Location of schema file to load.")
	flag.StringVar(&opt.DgraphsDir, "out", "out",
		"Location to write the final dgraph data directories.")
	flag.StringVar(&opt.TmpDir, "tmp", "tmp",
		"Temp directory used to use for on-disk scratch space. Requires free space proportional"+
			" to the size of the RDF file and the amount of indexing used.")
	flag.IntVar(&opt.NumGoroutines, "j", runtime.NumCPU(),
		"Number of worker threads to use (defaults to the number of logical CPUs)")
	flag.Int64Var(&opt.MapBufSize, "mapoutput_mb", 64,
		"The estimated size of each map file output. Increasing this increases memory usage.")
	httpAddr := flag.String("http", "localhost:8080",
		"Address to serve http (pprof).")
	flag.BoolVar(&opt.SkipMapPhase, "skip_map_phase", false,
		"Skip the map phase (assumes that map output files already exist).")
	flag.BoolVar(&opt.CleanupTmp, "cleanup_tmp", true,
		"Clean up the tmp directory after the loader finishes. Setting this to false allows the"+
			" bulk loader can be re-run while skipping the map phase.")
	flag.BoolVar(&opt.ExpandEdges, "expand_edges", true,
		"Generate edges that allow nodes to be expanded using _predicate_ or expand(...). "+
			"Disable to increase loading speed.")
	flag.IntVar(&opt.NumShufflers, "shufflers", 1,
		"Number of shufflers to run concurrently. Increasing this can improve performance, and "+
			"must be less than or equal to the number of reduce shards.")
	flag.IntVar(&opt.MapShards, "map_shards", 1,
		"Number of map output shards. Must be greater than or equal to the number of reduce "+
			"shards. Increasing allows more evenly sized reduce shards, at the expense of "+
			"increased memory usage.")
	flag.IntVar(&opt.ReduceShards, "reduce_shards", 1,
		"Number of reduce shards. This determines the number of dgraph instances in the final "+
			"cluster. Increasing this potentially decreases the reduce stage runtime by using "+
			"more parallelism, but increases memory usage.")
	flag.BoolVar(&opt.Version, "version", false, "Prints the version of dgraph-bulk-loader.")
	flag.BoolVar(&opt.StoreXids, "x", false, "Generate an xid edge for each node.")
	flag.StringVar(&opt.ZeroAddr, "z", "localhost:8888", "gRPC address for dgraphzero")

	flag.Parse()
	if len(flag.Args()) != 0 {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "No free args allowed, but got: %v\n", flag.Args())
		os.Exit(1)
	}
	if opt.Version {
		x.PrintVersionOnly()
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

	maxOpenFilesWarning()

	go func() {
		log.Fatal(http.ListenAndServe(*httpAddr, nil))
	}()
	if opt.BlockRate > 0 {
		runtime.SetBlockProfileRate(opt.BlockRate)
	}

	// Delete and recreate the output dirs to ensure they are empty.
	x.Check(os.RemoveAll(opt.DgraphsDir))
	for i := 0; i < opt.ReduceShards; i++ {
		dir := filepath.Join(opt.DgraphsDir, strconv.Itoa(i), "p")
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
		mergeMapShardsIntoReduceShards(opt)
	}
	loader.reduceStage()
	loader.writeSchema()
	loader.cleanup()
}

func maxOpenFilesWarning() {
	maxOpenFiles, err := queryMaxOpenFiles()
	const (
		red    = "\x1b[31m"
		green  = "\x1b[32m"
		yellow = "\x1b[33m"
		reset  = "\x1b[0m"
	)
	if err != nil {
		fmt.Printf(red+"Nonfatal error: max open file limit could not be detected: %v\n"+reset, err)
	}
	fmt.Println("The bulk loader needs to open many files at once. This number depends" +
		" on the size of the data set loaded, the map file output size, and the level " +
		"of indexing. 100,000 is adequate for most data set sizes. See `man ulimit` for" +
		" details of how to change the limit.")
	if err != nil {
		return
	}
	colour := green
	if maxOpenFiles < 1e5 {
		colour = yellow
	}
	fmt.Printf(colour+"Current max open files limit: %d\n"+reset, maxOpenFiles)
}
