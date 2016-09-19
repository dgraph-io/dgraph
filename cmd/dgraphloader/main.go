/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

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
	glog = x.Log("loader_main")

	rdfGzips = flag.String("rdfgzips", "",
		"Comma separated gzip files containing RDF data")
	instanceIdx = flag.Uint64("idx", 0,
		"Only pick entities, where Fingerprint % numInstance == instanceIdx.")
	numInstances = flag.Uint64("num", 1,
		"Total number of instances among which uid assigning is shared")
	postingDir = flag.String("p", "", "Directory to store posting lists")
	uidDir     = flag.String("u", "", "Directory to read UID posting lists")
	cpuprofile = flag.String("cpu", "", "write cpu profile to file")
	memprofile = flag.String("mem", "", "write memory profile to file")
	numcpu     = flag.Int("cores", runtime.NumCPU(),
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
		runtime.SetBlockProfileRate(1)
		go func() {
			log.Println(http.ListenAndServe(*prof, nil))
		}()
	}
	logrus.SetLevel(logrus.InfoLevel)
	numCpus := *numcpu
	prevProcs := runtime.GOMAXPROCS(numCpus)
	glog.WithField("num_cpu", numCpus).
		WithField("prev_maxprocs", prevProcs).
		Info("Set max procs to num cpus")

	// Create parent directory for postings.
	var err error
	err = os.MkdirAll(*postingDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for postings: %v", err)
	}

	if len(*rdfGzips) == 0 {
		glog.Fatal("No RDF GZIP files specified")
	}

	dataStore, err := store.NewStore(*postingDir)
	if err != nil {
		glog.Fatalf("Fail to initialize dataStore: %v", err)
	}
	defer dataStore.Close()
	posting.InitIndex(dataStore)

	uidStore, err := store.NewReadOnlyStore(*uidDir)
	if err != nil {
		glog.Fatalf("Fail to initialize uidStore: %v", err)
	}
	defer uidStore.Close()

	posting.Init()
	uid.Init(uidStore)
	loader.Init(uidStore, dataStore)

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

		// Load NQuads and write them to internal storage.
		count, err := loader.LoadEdges(r, *instanceIdx, *numInstances)
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
