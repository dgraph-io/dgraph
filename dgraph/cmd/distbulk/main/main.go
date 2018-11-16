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

package main

import (
    // "fmt"
    // "log"
    // "os"

    "flag"

    "github.com/chrislusf/gleam/distributed"
    "github.com/chrislusf/gleam/flow"
    "github.com/chrislusf/gleam/gio"
    "github.com/chrislusf/gleam/plugins/file"
    // "github.com/chrislusf/gleam/util"

    "github.com/dgraph-io/badger"
    "github.com/dgraph-io/dgraph/x"
)

type options struct {
    RDFDir      string
    SchemaFile  string
    DgraphsDir  string
    ExpandEdges bool
    StoreXids   bool
}

var (
    RdfToMapEntry = gio.RegisterMapper(rdfToMapEntry)
    // MapEntriesToPostingList = gio.RegisterReducer(mapEntriesToPostingList)
    Xidmap *badger.DB
    Opt    options
    Schema *schemaStore
    isDistributed   = flag.Bool("distributed", false, "run in distributed or not")
)

const (
    NUM_SHARDS = 4
)

// func init() {
//     flag.StringP("rdfs", "r", "",
//     "Directory containing *.rdf or *.rdf.gz files to load.")
//     flag.StringP("schema_file", "s", "",
//     "Location of schema file to load.")
//     flag.String("out", "out",
//     "Location to write the final dgraph data directories.")
//     flag.Bool("expand_edges", true,
//     "Generate edges that allow nodes to be expanded using _predicate_ or expand(...). "+
//     "Disable to increase loading speed.")
//     flag.BoolP("store_xids", "x", false, "Generate an xid edge for each node.")
// }

func main() {
    var err error
    Opt = options{
        RDFDir:      "data",
        SchemaFile:  "data.schema",
        DgraphsDir:  "dgraph-out",
        ExpandEdges: true,
        StoreXids:   false,
    }

    badgeropts := badger.DefaultOptions
    badgeropts.ReadOnly = true
    badgeropts.Dir = "./xids/"
    badgeropts.ValueDir = "./xids/"
    Xidmap, err = badger.Open(badgeropts)
    x.Check(err)
    defer Xidmap.Close()

    Schema = newSchemaStore(readSchema(Opt.SchemaFile), Opt)

    // ^^^^^^^^^^ Executor code above, driver code below vvvvvvvvvvv
    gio.Init()

    // if Opt.RDFDir == "" || Opt.SchemaFile == "" {
    //     flag.Usage()
    //     fmt.Fprint(os.Stderr, "RDF and schema file(s) must be specified.\n")
    //     os.Exit(1)
    // }

    // Delete and recreate the output dirs to ensure they are empty.
    // x.Check(os.RemoveAll(Opt.DgraphsDir))

    f := flow.New("dgraph distributed bulk loader").
        Read(file.Txt("data/*", NUM_SHARDS)).
        Map("rdfToMapEntry", RdfToMapEntry).
        // ReduceByKey("mapEntriesToPostingList", MapEntriesToPostingList).
        Printlnf("%v\t%v\t%v\t%v")

    if *isDistributed {
        f.Run(distributed.Option())
    } else {
        f.Run()
    }

    // loader := newLoader(opt)
    // if !opt.SkipMapPhase {
    // 	loader.mapStage()
    // 	mergeMapShardsIntoReduceShards(opt)
    // }
    // loader.reduceStage()
    // loader.writeSchema()
    // loader.cleanup()
}
