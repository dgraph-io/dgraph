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
    // "log"

	"flag"

	"github.com/chrislusf/gleam/distributed"
	"github.com/chrislusf/gleam/flow"
	"github.com/chrislusf/gleam/gio"
	"github.com/chrislusf/gleam/plugins/file"
	// "github.com/chrislusf/gleam/util"

    "github.com/dgraph-io/badger"
    "github.com/dgraph-io/dgraph/x"
    "github.com/rs/xid"

	"github.com/gogo/protobuf/proto"
    "github.com/dgraph-io/dgraph/protos/pb"
    "github.com/dgraph-io/dgraph/posting"
    "github.com/dgraph-io/dgraph/codec"
    // "fmt"
    // "os"
    // "reflect"
)

type options struct {
	RDFDir      string
	SchemaFile  string
	DgraphsDir  string
	ExpandEdges bool
	StoreXids   bool
}

var (
	RdfToMapEntry   = gio.RegisterMapper(rdfToMapEntry)
	TestDeserialize = gio.RegisterReducer(testDeserialize)
    WriteToBadger = gio.RegisterMapper(writeToBadger)
	Xdb           *XidMap
    Outdb         *badger.DB
    Outwb         *badger.WriteBatch
	Opt           options
	Schema        *schemaStore
	isDistributed = flag.Bool("distributed", false, "run in distributed or not")
)

const (
	NUM_SHARDS                 = 4
	REQUESTED_PREDICATE_SHARDS = 4
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
		RDFDir:      "../../data",
		SchemaFile:  "../../data.schema",
		DgraphsDir:  "../" + xid.New().String() + "-dgraph-p",
		ExpandEdges: true,
		StoreXids:   false,
	}

	Xdb = NewXidmap("../../xids")

    badgeropts := badger.DefaultOptions
    badgeropts.Dir = Opt.DgraphsDir
    badgeropts.ValueDir = Opt.DgraphsDir
    Outdb, err = badger.Open(badgeropts)
    x.Check(err)

    Outwb = Outdb.NewWriteBatch()
	Schema = newSchemaStore(readSchema(Opt.SchemaFile), Opt)

    gio.RegisterCleanup(cleanup)

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
		PartitionByKey("shard by predicate", REQUESTED_PREDICATE_SHARDS).
		LocalReduceBy("create postings", TestDeserialize, flow.Field(2)).
        Map("write to badger out", WriteToBadger)
		// ReduceBy("mapEntriesToPostingList", MapEntriesToPostingList).
		// OutputRow(func(row *util.Row) error {
		//     var pl pb.SivaPostingList
		//     err := proto.Unmarshal(row.V[1].([]byte), &pl)
		//     if err != nil {
		//         return err
		//     }
        //     fmt.Printf("Posting list:\n\tpred: %s\n\tkey: %v\n\tpostings: %+v\n\n",
        //         row.V[0].(string), row.K[0].([]byte), pl)
		//     // fmt.Printf("Posting list:\n\tkey: %v\n\tpostings: %+v\n\n", row.K[0].([]byte), row.V[0].([]byte))
		//     return nil
		// })

	if *isDistributed {
		f.Run(distributed.Option())
	} else {
		f.Run()
	}

    cleanup()
}

func writeToBadger(row []interface{}) error {
    // fmt.Fprintf(os.Stderr, "row:\n\t%v\n\t%v\n\t%v\n\t%v\n\n",
    //     row[0],row[1],row[2],reflect.TypeOf(row[3].([]interface{})[0]))
    var spl pb.SivaPostingList
    err := proto.Unmarshal(row[2].([]byte), &spl)
    if err != nil {
        return err
    }

    var list pb.List
    err = proto.Unmarshal(row[3].([]byte), &list)
    if err != nil {
        return err
    }

    // TODO figure out pl.commit_ts
    val, err := proto.Marshal(&pb.PostingList{
        Pack: codec.Encode(list.Uids, 256),
        Postings: spl.Postings,
    })
    if err != nil {
        return err
    }

    return Outwb.Set(row[0].([]byte), val, posting.BitCompletePosting)
    // return nil
}

func cleanup() {
    if Xdb != nil {
        Xdb.Close()
    }
    if Outwb != nil {
        Outwb.Flush()
    }
    if Outdb != nil {
        Outdb.Close()
    }
}
