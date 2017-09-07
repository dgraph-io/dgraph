package main

import (
	"flag"
	"os"
	"runtime"
)

func main() {

	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

	var opt options
	flag.StringVar(&opt.rdfFiles, "r", "", "Location of rdf files to load (comma separated)")
	flag.StringVar(&opt.schemaFile, "s", "", "Location of schema file to load")
	flag.StringVar(&opt.badgerDir, "b", "", "Location of target badger data directory")
	flag.StringVar(&opt.tmpDir, "tmp", os.TempDir(), "Temp directory used to use for on-disk "+
		"scratch space. Requires free space proportional to the size of the RDF file.")
	flag.IntVar(&opt.numGoroutines, "j", runtime.NumCPU()-1,
		"Number of worker threads to use (defaults to one less than logical CPUs)")
	flag.Parse()

	loader := newLoader(opt)
	loader.mapStage()
	loader.reduceStage()
	loader.writeSchema()
	loader.writeLease()
	loader.cleanup()
}
