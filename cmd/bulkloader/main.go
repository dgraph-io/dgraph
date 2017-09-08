package main

import (
	"flag"
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
	flag.StringVar(&opt.badgerDir, "p", "", "Location of the final Dgraph directory")
	flag.StringVar(&opt.tmpDir, "tmp", "tmp", "Temp directory used to use for on-disk "+
		"scratch space. Requires free space proportional to the size of the RDF file.")
	flag.IntVar(&opt.numGoroutines, "j", runtime.NumCPU(),
		"Number of worker threads to use (defaults to one less than logical CPUs)")
	flag.Int64Var(&opt.mapBufSize, "mapoutput_mb", 0,
		"The estimated size of each map file output. This directly affects the memory usage.")
	httpAddr := flag.String("http", "localhost:8080", "Address to serve http (pprof)")
	skipMapPhase := flag.Bool("skip_map_phase", false,
		"Skip the map phase (assumes that map output files already exist")
	cleanUpTmp := flag.Bool("cleanup_tmp", true,
		"Clean up the tmp directory after the loader finishes")

	flag.Parse()
	if len(flag.Args()) != 0 {
		log.Fatal("No free args allowed, but got:", flag.Args())
	}

	x.AssertTruef(opt.mapBufSize > 0, "Please specify how much memory is available for this program.")
	opt.mapBufSize = opt.mapBufSize << 20 // Convert from MB to B.

	go func() {
		log.Fatal(http.ListenAndServe(*httpAddr, nil))
	}()
	runtime.SetBlockProfileRate(*blockRate)
	x.Check(os.MkdirAll(opt.badgerDir, 0700))

	// Create a directory just for bulk loader's usage.
	if !*skipMapPhase {
		os.RemoveAll(opt.tmpDir)
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
	loader.writeLease()
	loader.cleanup()
}
