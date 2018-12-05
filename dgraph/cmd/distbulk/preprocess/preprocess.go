package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"

	"github.com/chrislusf/gleam/distributed"
	"github.com/chrislusf/gleam/flow"
	"github.com/chrislusf/gleam/gio"
	"github.com/chrislusf/gleam/plugins/file"
	"github.com/chrislusf/gleam/util"

	"github.com/dgraph-io/badger"
    bo "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/rdf"
    "github.com/dgraph-io/dgraph/gql"

    "github.com/gogo/protobuf/proto"

	"google.golang.org/grpc"
    "context"
    "time"
)

const (
	numIds = 1000000
    NUM_FILES = 4
    SCHEMA_FILE = "data.schema"
    STORE_XIDS = false
)

var (
	isDistributed = flag.Bool("distributed", false, "run in distributed or not")
	uidStart      uint64
	uidEnd        uint64
	zc            pb.ZeroClient
    // uidMap = map[string]uint64{
    //     "6993395339427947996": 20001,
    //     "9124413486081389766": 1,
    //     "12157535227869930158": 90001,
    //     "2204567941178379046": 120001,
    //     "2695865011725746053": 110001,
    //     "13259440069764597455": 30001,
    //     "5050508413252056582": 40001,
    //     "7725715438840337311": 10001,
    //     "17007490938041521212": 50001,
    //     "8397113093673239460": 80001,
    //     "12236106871166960662": 100001,
    //     "12277518267403927018": 70001,
    //     "3811935039818710427": 60001,
    // }
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func allocateUids() error {
	assignedIds, err := zc.AssignUids(context.Background(), &pb.Num{Val: numIds})
	if err != nil {
		return err
	}

	uidStart = assignedIds.StartId
	uidEnd = assignedIds.EndId
	return nil
}

func getWriteTimestamp() uint64 {
    for {
        ctx, cancel := context.WithTimeout(context.Background(), time.Second)
        ts, err := zc.Timestamps(ctx, &pb.Num{Val: 1})
        cancel()
        if err == nil {
            return ts.GetStartId()
        }
        fmt.Printf("Error communicating with dgraph zero, retrying: %v", err)
        time.Sleep(time.Second)
    }
}

func main() {
	zero, err := grpc.Dial("127.0.0.1:5080",
		grpc.WithBlock(),
		grpc.WithInsecure())
	check(err)

	zc = pb.NewZeroClient(zero)
	check(allocateUids())

	ParseRdf := gio.RegisterMapper(parseRdf)
	AssignUid := gio.RegisterMapper(assignUid)

	// This line is the differentiator between driver and executor
	// Anything above this line will run on the driver and ALL executors
	// Anything below this will ONLY run on the driver
	// NOTE: this also calls flag.Parse
	gio.Init()

	opts := badger.DefaultOptions
    opts.SyncWrites = false
    opts.TableLoadingMode = bo.MemoryMap
	opts.Dir = "./xids/"
	opts.ValueDir = "./xids/"
	db, err := badger.Open(opts)
	check(err)
	defer db.Close()

	wb := db.NewWriteBatch()
	defer wb.Cancel()

    // TODO pass in options for where to read stuff (esp. store xids)
    sch := newSchemaStore(readSchema(SCHEMA_FILE), STORE_XIDS)

    schout, err := os.OpenFile(SCHEMA_FILE + ".full", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
    check(err)
    defer schout.Close()

	mapreduce := flow.New("dgraph distributed bulk loader").
		Read(file.Txt("./data/*", NUM_FILES)).
		Map("parseRdf", ParseRdf).
		Map("assignUid", AssignUid).
		Distinct("unique UID", flow.Field(1)).
		OutputRow(func(row *util.Row) error {
            var de pb.DirectedEdge
            err := proto.Unmarshal(row.V[0].([]byte), &de)
            if err != nil {
                return err
            }

            // find schema entry for given predicate
            // if not found, create a basic schema entry for the value type given
            sch.validateType(&de)

			var uidBuf [binary.MaxVarintLen64]byte
            xid := row.K[0].(string)
            uid := row.V[1].(uint64)
			n := binary.PutUvarint(uidBuf[:], uid)
            // fmt.Fprintf(os.Stderr, "XID ENTRY: %s -> %d\n\n", xid, uid)
			return wb.Set([]byte(xid), uidBuf[:n], 0)
		})

	if *isDistributed {
		mapreduce.Run(distributed.Option())
	} else {
		mapreduce.Run()
	}
	check(wb.Flush())
    for k, v := range sch.m {
        fmt.Fprintf(schout, toSchemaFileString(k, v))
    }
    fmt.Printf("writets to use: %d\n", getWriteTimestamp())
}

func parseNQuad(line string) (gql.NQuad, error) {
    nq, err := rdf.Parse(line)
    if err != nil {
        return gql.NQuad{}, err
    }
    return gql.NQuad{NQuad: &nq}, nil
}

func parseRdf(row []interface{}) error {
	nq, err := parseNQuad(gio.ToString(row[0]))
	if err == rdf.ErrEmpty {
		return nil
	} else if err != nil {
		return err
	}

    // sid and oid are not known, leave them at 0
    var de *pb.DirectedEdge
    if nq.GetObjectValue() == nil {
        de = nq.CreateUidEdge(0, 0)
        de.ValueType = pb.Posting_UID
    } else {
        de, err = nq.CreateValueEdge(0)
        if err != nil {
            return err
        }
    }

    debytes, err := proto.Marshal(de)
    if err != nil {
        return err
    }

    // always assign a UID to a Subject XID
    gio.Emit(nq.GetSubject(), debytes)

    // only assign a UID to an Object XID if it's not a value
	if nq.GetObjectValue() == nil {
        gio.Emit(nq.GetObjectId(), debytes)
	}

	return nil
}

func assignUid(row []interface{}) error {
	if uidStart >= uidEnd {
		if err := allocateUids(); err != nil {
			return err
		}
	}
	row = append(row, uidStart)
    // row = append(row, uidMap[row[0].(string)])
	uidStart += 1
	gio.Emit(row...)
	return nil
}
