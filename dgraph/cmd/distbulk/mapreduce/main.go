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
    "sort"

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
    // "github.com/chrislusf/gleam/util"
    "math"
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
    WriteTs       uint64
    isDistributed = flag.Bool("distributed", false, "run in distributed or not")
)

const (
	NUM_SHARDS                 = 1
	REQUESTED_PREDICATE_SHARDS = 1
    PATH_PREFIX = "./"
    // PATH_PREFIX = "../../"
)

func getWriteTimestamp(zero *grpc.ClientConn) uint64 {
    client := pb.NewZeroClient(zero)
    for {
        ctx, cancel := context.WithTimeout(context.Background(), time.Second)
        ts, err := client.Timestamps(ctx, &pb.Num{Val: 1})
        cancel()
        if err == nil {
            return ts.GetStartId()
        }
        fmt.Printf("Error communicating with dgraph zero, retrying: %v", err)
        time.Sleep(time.Second)
    }
}

func main() {
    var err error
	Opt = options{
		RDFDir:      PATH_PREFIX + "data",
		SchemaFile:  PATH_PREFIX + "data.schema",
		DgraphsDir:  PATH_PREFIX + xid.New().String() + "-dgraph-p",
		ExpandEdges: true,
		StoreXids:   false,
        ZeroAddr:    "127.0.0.1:5080",
	}

    zero, err := grpc.Dial(
        Opt.ZeroAddr,
        grpc.WithBlock(),
        grpc.WithInsecure(),
        grpc.WithTimeout(time.Minute),
    )
    x.Checkf(err, "Unable to connect to zero, Is it running at %s?", Opt.ZeroAddr)
    WriteTs = getWriteTimestamp(zero)

	Xdb = NewXidmap(PATH_PREFIX + "xids", 1 << 19)

    badgeropts := badger.DefaultOptions
    badgeropts.SyncWrites = false
    badgeropts.TableLoadingMode = bo.MemoryMap
    badgeropts.Dir = Opt.DgraphsDir
    badgeropts.ValueDir = Opt.DgraphsDir
    Outdb, err = badger.OpenManaged(badgeropts)
    x.Check(err)

    Outtx = Outdb.NewTransactionAt(math.MaxUint64, true)
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
		Read(file.Txt(Opt.RDFDir + "/*", NUM_SHARDS)).
		Map("rdfToMapEntry", RdfToMapEntry).
        // OutputRow(func(row *util.Row) error {
        //     var spl pb.SivaPostingList
        //     err := proto.Unmarshal(row.V[1].([]byte), &spl)
        //     if err != nil {
        //         return err
        //     }
        //     fmt.Printf("%v %v %v\n", row.K[0], row.V[0], spl)
        //     return nil
        // })
		PartitionByKey("shard by predicate", REQUESTED_PREDICATE_SHARDS).
		LocalReduceBy("create postings", TestDeserialize, flow.Field(2)).
        Map("write to badger out", WriteToBadger).
        Printlnf("%v %v %v")

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
    if err != nil {
        return err
    }

    var spl pb.SivaPostingList
    err = proto.Unmarshal(row[3].([]byte), &spl)
    if err != nil {
        return err
    }

    sort.Slice(ul.Uids, func(i, j int) bool {
        return ul.Uids[i] < ul.Uids[j]
    })
    sort.Slice(spl.Postings, func(i, j int) bool {
        return spl.Postings[i].Uid < spl.Postings[j].Uid
    })

    val, err := proto.Marshal(&pb.PostingList{
        Pack: codec.Encode(ul.Uids, 256),
        Postings: spl.Postings,
    })
    if err != nil {
        return err
    }

    err = Outtx.SetWithMeta(row[0].([]byte), val, posting.BitCompletePosting)
    if err == badger.ErrTxnTooBig {
        err = Outtx.CommitAt(WriteTs, nil)
        if err != nil {
            return err
        }

        // try again, if it fails, this time it's serious
        err = Outtx.SetWithMeta(row[0].([]byte), val, posting.BitCompletePosting)
        if err != nil {
            return err
        }
    } else if err != nil {
        return err
    }

    gio.Emit(row...)
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
