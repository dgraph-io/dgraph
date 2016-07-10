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
	"os"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/store/boltdb"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"

	_ "github.com/dgraph-io/dgraph/store/rocksdb"
)

var glog = x.Log("loader_main")

var rdfGzips = flag.String("rdfgzips", "",
	"Comma separated gzip files containing RDF data")
var instanceIdx = flag.Uint64("instanceIdx", 0,
	"Only pick entities, where Fingerprint % numInstance == instanceIdx.")
var numInstances = flag.Uint64("numInstances", 1,
	"Total number of instances among which uid assigning is shared")
var postingDir = flag.String("postings", "", "Directory to store posting lists")
var uidDir = flag.String("uids", "", "Directory to read UID posting lists")
var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
var memprofile = flag.String("memprofile", "", "write memory profile to file")
var numcpu = flag.Int("numCpu", runtime.NumCPU(),
	"Number of cores to be used by the process")
var storeType = flag.String("store", boltdb.Name, "Backend store type.")

func main() {
	flag.Parse()
	if !flag.Parsed() {
		glog.Fatal("Unable to parse flags")
	}
	if len(*cpuprofile) > 0 {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			glog.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	logrus.SetLevel(logrus.InfoLevel)
	numCpus := *numcpu
	prevProcs := runtime.GOMAXPROCS(numCpus)
	glog.WithField("num_cpu", numCpus).
		WithField("prev_maxprocs", prevProcs).
		Info("Set max procs to num cpus")

	if len(*rdfGzips) == 0 {
		glog.Fatal("No RDF GZIP files specified")
	}
	dataStore, err := store.Registry.Get(*storeType)
	if err != nil {
		log.Fatalln(err)
	}
	dataStore.Init(*postingDir)
	defer dataStore.Close()

	uidStore, err := store.Registry.Get(*storeType)
	if err != nil {
		log.Fatalln(err)
	}
	uidStore.InitReadOnly(*uidDir)
	defer uidStore.Close()

	posting.Init(nil)
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
