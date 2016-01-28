/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"os"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/loader"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("loader_main")

var rdfGzips = flag.String("rdfgzips", "",
	"Comma separated gzip files containing RDF data")
var mod = flag.Uint64("mod", 1, "Only pick entities, where uid % mod == 0.")
var postingDir = flag.String("postings", "", "Directory to store posting lists")
var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

func main() {
	flag.Parse()
	if !flag.Parsed() {
		glog.Fatal("Unable to parse flags")
	}
	if len(*cpuprofile) > 0 {
		f, err := os.Create(*cpuprofile)
i		if err != nil {
			glog.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	logrus.SetLevel(logrus.InfoLevel)
	numCpus := runtime.NumCPU()
	prevProcs := runtime.GOMAXPROCS(numCpus)
	glog.WithField("num_cpu", numCpus).
		WithField("prev_maxprocs", prevProcs).
		Info("Set max procs to num cpus")

	if len(*rdfGzips) == 0 {
		glog.Fatal("No RDF GZIP files specified")
	}
	ps := new(store.Store)
	ps.Init(*postingDir)
	defer ps.Close()

	posting.Init(ps, nil)

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

		count, err := loader.HandleRdfReader(r, *mod)
		if err != nil {
			glog.WithError(err).Fatal("While handling rdf reader.")
		}
		glog.WithField("count", count).Info("RDFs parsed")
		r.Close()
		f.Close()
	}
	glog.Info("Calling merge lists")
	posting.MergeLists(100 * numCpus) // 100 per core.
}
