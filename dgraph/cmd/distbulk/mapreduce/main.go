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
    "github.com/chrislusf/gleam/util"
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
	ConcatenatePostings = gio.RegisterReducer(concatenatePostings)
    WriteToBadgerMany = gio.RegisterMapper(writeToBadgerMany)
	Xdb           *XidMap
    Outdb         *badger.DB
    Outtx         *badger.Txn
	Opt           options
	Schema        *schemaStore
    isDistributed = flag.Bool("distributed", false, "run in distributed or not")
)

const (
	NUM_SHARDS                 = 4
	REQUESTED_PREDICATE_SHARDS = 1
    WriteTs = 1
)

func main() {
    _ = util.Row{}
    Opt = options{
        RDFDir:      "data",
        SchemaFile:  "../../data.schema.full",
        ExpandEdges: true,
        // ExpandEdges: false,
        StoreXids:   false,
        ZeroAddr:    "127.0.0.1:5080",
    }
    if REQUESTED_PREDICATE_SHARDS > 1 {
        Opt.DgraphsDir = "../../" + xid.New().String() + "-dgraph-p"
        initOutDb()
        gio.RegisterCleanup(cleanup)
    } else {
        Opt.DgraphsDir = xid.New().String() + "-dgraph-p"
    }

    Xdb = NewXidmap("../../xids", 1 << 19)
    Schema = newSchemaStore(readSchema(Opt.SchemaFile), Opt.StoreXids)

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
		Map("rdfToMapEntry", RdfToMapEntry)

    if REQUESTED_PREDICATE_SHARDS > 1 {
        f = f.
            PartitionByKey("shard by predicate", REQUESTED_PREDICATE_SHARDS).
            LocalSort("sort keys", flow.OrderBy(2, true).By(3, true)).
            LocalReduceBy("create postings", ConcatenatePostings, flow.Field(2)).
            Map("write to badger", WriteToBadgerMany)
    } else {
        initOutDb()
        f = f.
            // Sort("sort keys and uids", flow.OrderBy(2, true).By(3, true)).
            Sort("sort keys and uids", flow.OrderBy(1, true).By(2, true).By(3, true)).
            LocalReduceBy("create postings", ConcatenatePostings, flow.Field(2)).
            OutputRow(func(row *util.Row) error {
                // key, pred, uid, uidlist, pbytes
                writeToBadger(row.K[0].([]byte), row.V[2].([]byte), row.V[3].([]byte))
                return nil
            })
            // Printlnf("%v %v %v %v %v")
    }

	if *isDistributed {
		f.Run(distributed.Option())
	} else {
		f.Run()
	}

    cleanup()
}

// format becomes row = {key, pred, uidbuf, uid list, posting list} after reduction
func writeToBadgerMany(row []interface{}) error {
    writeToBadger(row[0].([]byte), row[3].([]byte), row[4].([]byte))
    return nil
}

func writeToBadger(key, ubytes, pbytes []byte) {
    var ul pb.List
    err := proto.Unmarshal(ubytes, &ul)
    x.Check(err)

    var spl pb.SivaPostingList
    err = proto.Unmarshal(pbytes, &spl)
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

    // gio.Emit(key, val)

    err = Outtx.SetWithMeta(key, val, posting.BitCompletePosting)
    if err != badger.ErrTxnTooBig {
        x.Check(err)
    }

    // txn got too big
    x.Check(Outtx.CommitAt(WriteTs, nil))

    Outtx = Outdb.NewTransactionAt(WriteTs, true)

    // try again, if it fails, this time it's serious
    x.Check(Outtx.SetWithMeta(key, val, posting.BitCompletePosting))
}

func initOutDb() {
    var err error
    badgeropts := badger.DefaultOptions
    badgeropts.SyncWrites = false
    badgeropts.TableLoadingMode = bo.MemoryMap
    badgeropts.Dir = Opt.DgraphsDir
    badgeropts.ValueDir = Opt.DgraphsDir
    Outdb, err = badger.OpenManaged(badgeropts)
    x.Check(err)

    Outtx = Outdb.NewTransactionAt(WriteTs, true)
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
