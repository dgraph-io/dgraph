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
	"flag"

	"github.com/chrislusf/gleam/distributed"
	"github.com/chrislusf/gleam/flow"
	"github.com/chrislusf/gleam/gio"
	"github.com/chrislusf/gleam/plugins/file"

    "github.com/dgraph-io/badger"
    bo "github.com/dgraph-io/badger/options"
    "github.com/dgraph-io/dgraph/x"
    "github.com/rs/xid"

	"github.com/gogo/protobuf/proto"
    "github.com/dgraph-io/dgraph/protos/pb"
    "github.com/dgraph-io/dgraph/posting"
    "github.com/dgraph-io/dgraph/codec"

    // "fmt"
    // "os"
    // "github.com/chrislusf/gleam/util"
    // "context"
    // "time"
    // "sort"
)

type options struct {
	RDFDir      string
	SchemaFile  string
	DgraphsDir  string
	ExpandEdges bool
	StoreXids   bool
    ZeroAddr    string
}

var (
	RdfToMapEntry   = gio.RegisterMapper(rdfToMapEntry)
	TestDeserialize = gio.RegisterReducer(testDeserialize)
    WriteToBadger = gio.RegisterMapper(writeToBadger)
	Xdb           *XidMap
    Outdb         *badger.DB
    Outtx         *badger.Txn
	Opt           options
	Schema        *schemaStore
    isDistributed = flag.Bool("distributed", false, "run in distributed or not")
)

const (
	NUM_SHARDS                 = 4
	REQUESTED_PREDICATE_SHARDS = 4
    // PATH_PREFIX = "./"
    PATH_PREFIX = "../../"
    WriteTs = 1
)

func main() {
    var err error
	Opt = options{
		RDFDir:      "./data",
		SchemaFile:  PATH_PREFIX + "data.schema.full",
		DgraphsDir:  PATH_PREFIX + xid.New().String() + "-dgraph-p",
		ExpandEdges: true,
        // ExpandEdges: false,
		StoreXids:   false,
        ZeroAddr:    "127.0.0.1:5080",
	}

	Xdb = NewXidmap(PATH_PREFIX + "xids", 1 << 19)

    badgeropts := badger.DefaultOptions
    badgeropts.SyncWrites = false
    badgeropts.TableLoadingMode = bo.MemoryMap
    badgeropts.Dir = Opt.DgraphsDir
    badgeropts.ValueDir = Opt.DgraphsDir
    Outdb, err = badger.OpenManaged(badgeropts)
    x.Check(err)

    Outtx = Outdb.NewTransactionAt(WriteTs, true)
	Schema = newSchemaStore(readSchema(Opt.SchemaFile), Opt.StoreXids)

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
		Read(file.Txt(Opt.RDFDir + "/*", NUM_SHARDS)).
		Map("rdfToMapEntry", RdfToMapEntry).
		PartitionByKey("shard by predicate", REQUESTED_PREDICATE_SHARDS).
        LocalSort("sort keys", flow.OrderBy(2, true).By(3, true)).
		LocalReduceBy("create postings", TestDeserialize, flow.Field(2)). // REQUIRES LOCAL SORT TO WORK CORRECTLY
        Map("write to badger", WriteToBadger).
        Printlnf("%v %v")

	if *isDistributed {
		f.Run(distributed.Option())
	} else {
		f.Run()
	}

    cleanup()
}

// format becomes row = {key, pred, uid list, posting list} after reduction
func writeToBadger(row []interface{}) error {
    var ul pb.List
    err := proto.Unmarshal(row[2].([]byte), &ul)
    x.Check(err)

    var spl pb.SivaPostingList
    err = proto.Unmarshal(row[3].([]byte), &spl)
    x.Check(err)

    // sort.Slice(ul.Uids, func(i, j int) bool {
    //     return ul.Uids[i] < ul.Uids[j]
    // })
    // sort.Slice(spl.Postings, func(i, j int) bool {
    //     return spl.Postings[i].Uid < spl.Postings[j].Uid
    // })

    val, err := proto.Marshal(&pb.PostingList{
        Pack: codec.Encode(ul.Uids, 256),
        Postings: spl.Postings,
    })
    x.Check(err)

    key := row[0].([]byte)
    gio.Emit(key, val)

    err = Outtx.SetWithMeta(key, val, posting.BitCompletePosting)
    if err != badger.ErrTxnTooBig {
        x.Check(err)
    }

    // txn got too big
    x.Check(Outtx.CommitAt(WriteTs, nil))

    Outtx = Outdb.NewTransactionAt(WriteTs, true)

    // try again, if it fails, this time it's serious
    x.Check(Outtx.SetWithMeta(row[0].([]byte), val, posting.BitCompletePosting))

    return nil
}

func cleanup() {
    if Xdb != nil {
        Xdb.Close()
    }

    if Outtx != nil {
        x.Check(Outtx.CommitAt(WriteTs, nil))
    }
    if Schema != nil {
        Schema.write(Outdb)
    }
    if Outdb != nil {
        Outdb.Close()
    }
}
