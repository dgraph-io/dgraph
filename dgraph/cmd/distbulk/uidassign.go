package main

import (
	"context"
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
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/rdf"

	"google.golang.org/grpc"
)

const (
	numIds = 10000000
)

var (
	isDistributed = flag.Bool("distributed", false, "run in distributed or not")
	uidStart      uint64
	uidEnd        uint64
	zc            pb.ZeroClient
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
	opts.Dir = "./xids/"
	opts.ValueDir = "./xids/"
	db, err := badger.Open(opts)
	check(err)
	defer db.Close()

	wb := db.NewWriteBatch()
	defer wb.Cancel()

	mapreduce := flow.New("dgraph distributed bulk loader").
		Read(file.Txt("data/*", 4)).
		Map("parseRdf", ParseRdf).
		Map("assignUid", AssignUid).
		Distinct("unique UID", flow.Field(1)).
		// Printlnf("%s %d")
		OutputRow(func(row *util.Row) error {
			node := gio.ToString(row.K[0])
			var uidBuf [binary.MaxVarintLen64]byte
			n := binary.PutUvarint(uidBuf[:], row.V[0].(uint64))
			return wb.Set([]byte(node), uidBuf[:n], 0)
		})

	if *isDistributed {
		mapreduce.Run(distributed.Option())
	} else {
		mapreduce.Run()
	}
	check(wb.Flush())
}

func parseRdf(row []interface{}) error {
	nq, err := rdf.Parse(gio.ToString(row[0]))
	if err == rdf.ErrEmpty {
		return nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v: %+v\n", err, nq)
		return err
	}

	gio.Emit(nq.GetSubject())
	objId := nq.GetObjectId()
	if objId == "" {
		gio.Emit(nq.GetObjectValue().GetDefaultVal())
	} else {
		gio.Emit(objId)
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
	uidStart += 1
	gio.Emit(row...)
	return nil
}
