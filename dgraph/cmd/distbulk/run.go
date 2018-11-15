/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package distbulk

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"

    "bytes"
    "context"
    "encoding/binary"
    "flag"
    "io"
    "log"
    "math"
    "os"
    "strings"

    "github.com/chrislusf/gleam/distributed"
    "github.com/chrislusf/gleam/flow"
    "github.com/chrislusf/gleam/gio"
    "github.com/chrislusf/gleam/plugins/file"
    "github.com/chrislusf/gleam/util"

    "github.com/dgraph-io/badger"
    "github.com/dgraph-io/dgo/protos/api"
    "github.com/dgraph-io/dgraph/gql"
    "github.com/dgraph-io/dgraph/posting"
    "github.com/dgraph-io/dgraph/protos/pb"
    "github.com/dgraph-io/dgraph/rdf"
    "github.com/dgraph-io/dgraph/schema"
    "github.com/dgraph-io/dgraph/tok"
    "github.com/dgraph-io/dgraph/types"
    "github.com/dgraph-io/dgraph/types/facets"
    "github.com/dgraph-io/dgraph/x"

    farm "github.com/dgryski/go-farm"
    "github.com/gogo/protobuf/proto"
    "github.com/pkg/errors"
    "google.golang.org/grpc"
)

type options struct {
    RDFDir        string
    SchemaFile    string
    DgraphsDir    string
    ExpandEdges   bool
    StoreXids     bool
}

var (
    Distbulk x.SubCommand
)

func init() {
	Distbulk.Cmd = &cobra.Command{
		Use:   "distbulk",
		Short: "Run Dgraph distributed MapReduce bulk loader",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Distbulk.Conf).Stop()
			run()
		},
	}
	Distbulk.EnvPrefix = "DGRAPH_DISTBULK"

	flag := Distbulk.Cmd.Flags()
	flag.StringP("rdfs", "r", "",
		"Directory containing *.rdf or *.rdf.gz files to load.")
	flag.StringP("schema_file", "s", "",
		"Location of schema file to load.")
	flag.String("out", "out",
		"Location to write the final dgraph data directories.")
	// flag.String("tmp", "tmp",
	// 	"Temp directory used to use for on-disk scratch space. Requires free space proportional"+
	// 		" to the size of the RDF file and the amount of indexing used.")
	// flag.IntP("num_go_routines", "j", runtime.NumCPU(),
	// 	"Number of worker threads to use (defaults to the number of logical CPUs)")
	// flag.Int64("mapoutput_mb", 64,
	// 	"The estimated size of each map file output. Increasing this increases memory usage.")
	flag.Bool("expand_edges", true,
		"Generate edges that allow nodes to be expanded using _predicate_ or expand(...). "+
			"Disable to increase loading speed.")
	// flag.Bool("skip_map_phase", false,
	// 	"Skip the map phase (assumes that map output files already exist).")
	// flag.Bool("cleanup_tmp", true,
	// 	"Clean up the tmp directory after the loader finishes. Setting this to false allows the"+
	// 		" bulk loader can be re-run while skipping the map phase.")
	// flag.Int("shufflers", 1,
	// 	"Number of shufflers to run concurrently. Increasing this can improve performance, and "+
	// 		"must be less than or equal to the number of reduce shards.")
	flag.BoolP("store_xids", "x", false, "Generate an xid edge for each node.")
	// flag.StringP("zero", "z", "localhost:5080", "gRPC address for Dgraph zero")
	// flag.String("http", "localhost:8080",
	// 	"Address to serve http (pprof).")
	// flag.Bool("ignore_errors", false, "ignore line parsing errors in rdf files")
	// flag.Int("map_shards", 1,
	// 	"Number of map output shards. Must be greater than or equal to the number of reduce "+
	// 		"shards. Increasing allows more evenly sized reduce shards, at the expense of "+
	// 		"increased memory usage.")
	// flag.Int("reduce_shards", 1,
	// 	"Number of reduce shards. This determines the number of dgraph instances in the final "+
	// 		"cluster. Increasing this potentially decreases the reduce stage runtime by using "+
	// 		"more parallelism, but increases memory usage.")
}

func run() {
	// opt := options{
	// 	RDFDir:        Distbulk.Conf.GetString("rdfs"),
	// 	SchemaFile:    Distbulk.Conf.GetString("schema_file"),
	// 	DgraphsDir:    Distbulk.Conf.GetString("out"),
	// 	ExpandEdges:   Distbulk.Conf.GetBool("expand_edges"),
	// 	Version:       Distbulk.Conf.GetBool("version"),
	// 	StoreXids:     Distbulk.Conf.GetBool("store_xids"),
	// }

    opt := options{
        RDFDir:        "data",
        SchemaFile:    "data.schema",
        DgraphsDir:    "dgraph-out",
        ExpandEdges:   true,
        StoreXids:     false,
    }

	if opt.RDFDir == "" || opt.SchemaFile == "" {
		flag.Usage()
		fmt.Fprint(os.Stderr, "RDF and schema file(s) must be specified.\n")
		os.Exit(1)
	}

	optBuf, err := json.MarshalIndent(&opt, "", "\t")
	x.Check(err)
	fmt.Println(string(optBuf))

	// Delete and recreate the output dirs to ensure they are empty.
	x.Check(os.RemoveAll(opt.DgraphsDir))

	loader := newLoader(opt)
	if !opt.SkipMapPhase {
		loader.mapStage()
		mergeMapShardsIntoReduceShards(opt)
	}
	loader.reduceStage()
	loader.writeSchema()
	loader.cleanup()
}

func readSchema(filename string) []*pb.SchemaUpdate {
    f, err := os.Open(filename)
    x.Check(err)
    defer f.Close()
    var r io.Reader = f
    if filepath.Ext(filename) == ".gz" {
        r, err = gzip.NewReader(f)
        x.Check(err)
    }

    buf, err := ioutil.ReadAll(r)
    x.Check(err)

    initialSchema, err := schema.Parse(string(buf))
    x.Check(err)
    return initialSchema
}
