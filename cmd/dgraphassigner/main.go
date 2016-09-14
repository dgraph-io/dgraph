package main

import (
	"compress/gzip"
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/Sirupsen/logrus"

	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

var (
	glog = x.Log("uidassigner_main")

	rdfGzips = flag.String("rdfgzips", "",
		"Comma separated gzip files containing RDF data")
	instanceIdx = flag.Uint64("instanceIdx", 0,
		"Only pick entities, where Fingerprint % numInstance == instanceIdx.")
	numInstances = flag.Uint64("numInstances", 1,
		"Total number of instances among which uid assigning is shared")
	uidDir = flag.String("uids", "",
		"Directory to store xid to uid posting lists")
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write memory profile to file")
	numcpu     = flag.Int("numCpu", runtime.NumCPU(),
		"Number of cores to be used by the process")
	prof = flag.String("profile", "", "Address at which profile is displayed")
)

func main() {
	x.Init()

	if len(*cpuprofile) > 0 {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			glog.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if *prof != "" {
		go func() {
			log.Println(http.ListenAndServe(*prof, nil))
		}()
	}
	logrus.SetLevel(logrus.InfoLevel)
	numCpus := *numcpu
	prevProcs := runtime.GOMAXPROCS(numCpus)
	glog.WithField("num_cpus", numCpus).
		WithField("prev_maxprocs", prevProcs).
		Info("Set max procs to num  cpus")

	glog.WithField("instanceIdx", *instanceIdx).
		WithField("numInstances", *numInstances).
		Info("Only XIDs with FP(xid)%numInstance == instanceIdx will be given UID")

	if len(*rdfGzips) == 0 {
		glog.Fatal("No RDF GZIP files specified")
	}

	// Create parent directory for uids.
	var err error
	err = os.MkdirAll(*uidDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for uids: %v", err)
	}

	ps, err := store.NewStore(*uidDir)
	if err != nil {
		glog.Fatalf("Fail to initialize ps: %v", err)
	}
	defer ps.Close()

	posting.Init()
	uid.Init(ps)
	loader.Init(nil, ps)

	files := strings.Split(*rdfGzips, ",")
	for _, path := range files {
		if len(path) == 0 {
			continue
		}
		glog.WithField("path", path).Info("Handling...")
		f, err := os.Open(path)
		if err != nil {
			glog.WithError(err).Fatal("Unable to open rdf file.")
		}

		r, err := gzip.NewReader(f)
		if err != nil {
			glog.WithError(err).Fatal("Unable to create gzip reader.")
		}

		count, err := loader.AssignUids(r, *instanceIdx, *numInstances)
		if err != nil {
			glog.WithError(err).Fatal("While handling rdf reader.")
		}
		glog.WithField("count", count).Info("RDFs parsed")
		r.Close()
		f.Close()
	}
	glog.Info("Calling merge lists")
	posting.MergeLists(100 * numCpus) // 100 per core.

	if len(*memprofile) > 0 {
		f, err := os.Create(*memprofile)
		if err != nil {
			glog.Fatal(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}
}
