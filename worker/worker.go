package worker

import (
	"flag"
	"io"
	"net"
	"net/rpc"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"
)

var workerPort = flag.String("workerport", ":12345",
	"Port used by worker for internal communication.")

var glog = x.Log("worker")
var dataStore, uidStore *store.Store
var pools []*conn.Pool
var numInstances, instanceIdx uint64

var addrs []string

func Init(ps, uStore *store.Store, workerList []string, idx, numInst uint64) {
	dataStore = ps
	uidStore = uStore
	addrs = workerList
	instanceIdx = idx
	numInstances = numInst
}

func Connect() {
	w := new(Worker)
	if err := rpc.Register(w); err != nil {
		glog.Fatal(err)
	}
	if err := runServer(*workerPort); err != nil {
		glog.Fatal(err)
	}

	for _, addr := range addrs {
		if len(addr) == 0 {
			continue
		}
		pool := conn.NewPool(addr, 5)
		query := new(conn.Query)
		query.Data = []byte("hello")
		reply := new(conn.Reply)
		if err := pool.Call("Worker.Hello", query, reply); err != nil {
			glog.WithField("call", "Worker.Hello").Fatal(err)
		}
		glog.WithField("reply", string(reply.Data)).WithField("addr", addr).
			Info("Got reply from server")
		pools = append(pools, pool)
	}

	glog.Info("Server started. Clients connected.")
}

func ProcessTaskOverNetwork(qu []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(qu)
	q := new(task.Query)
	q.Init(qu, uo)

	attr := string(q.Attr())
	idx := farm.Fingerprint64([]byte(attr)) % numInstances

	if attr == "_xid_" || attr == "_uid_" {
		if instanceIdx == 0 {
			return ProcessTask(qu)
		} else { // Send the request to instance 0 which has uidstore
			pool := pools[0]
			addr := addrs[0]
			query := new(conn.Query)
			query.Data = qu
			reply := new(conn.Reply)
			if err := pool.Call("Worker.ServeTask", query, reply); err != nil {
				glog.WithField("call", "Worker.ServeTask").Fatal(err)
			}
			glog.WithField("reply", string(reply.Data)).WithField("addr", addr).
				Info("Got reply from server")

			return reply.Data, nil
		}
	} else {
		if idx == instanceIdx {
			return ProcessTask(qu)
		} else {
			pool := pools[idx]
			addr := addrs[idx]
			query := new(conn.Query)
			query.Data = qu
			reply := new(conn.Reply)
			if err := pool.Call("Worker.ServeTask", query, reply); err != nil {
				glog.WithField("call", "Worker.ServeTask").Fatal(err)
			}
			glog.WithField("reply", string(reply.Data)).WithField("addr", addr).
				Info("Got reply from server")

			return reply.Data, nil
		}
	}
}

func ProcessTask(query []byte) (result []byte, rerr error) {
	uo := flatbuffers.GetUOffsetT(query)
	q := new(task.Query)
	q.Init(query, uo)

	b := flatbuffers.NewBuilder(0)
	voffsets := make([]flatbuffers.UOffsetT, q.UidsLength())
	uoffsets := make([]flatbuffers.UOffsetT, q.UidsLength())

	attr := string(q.Attr())

	for i := 0; i < q.UidsLength(); i++ {
		uid := q.Uids(i)
		key := posting.Key(uid, attr)
		pl := posting.GetOrCreate(key, dataStore)

		var valoffset flatbuffers.UOffsetT
		if val, err := pl.Value(); err != nil {
			valoffset = b.CreateByteVector(x.Nilbyte)
		} else {
			valoffset = b.CreateByteVector(val)
		}
		task.ValueStart(b)
		task.ValueAddVal(b, valoffset)
		voffsets[i] = task.ValueEnd(b)

		ulist := pl.GetUids()
		uoffsets[i] = x.UidlistOffset(b, ulist)
	}
	task.ResultStartValuesVector(b, len(voffsets))
	for i := len(voffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(voffsets[i])
	}
	valuesVent := b.EndVector(len(voffsets))

	task.ResultStartUidmatrixVector(b, len(uoffsets))
	for i := len(uoffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(uoffsets[i])
	}
	matrixVent := b.EndVector(len(uoffsets))

	task.ResultStart(b)
	task.ResultAddValues(b, valuesVent)
	task.ResultAddUidmatrix(b, matrixVent)
	rend := task.ResultEnd(b)
	b.Finish(rend)
	return b.Bytes[b.Head():], nil
}

func NewQuery(attr string, uids []uint64) []byte {
	b := flatbuffers.NewBuilder(0)
	task.QueryStartUidsVector(b, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		b.PrependUint64(uids[i])
	}
	vend := b.EndVector(len(uids))

	ao := b.CreateString(attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	task.QueryAddUids(b, vend)
	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

type Worker struct {
}

func (w *Worker) Hello(query *conn.Query, reply *conn.Reply) error {
	if string(query.Data) == "hello" {
		reply.Data = []byte("Oh hello there!")
	} else {
		reply.Data = []byte("Hey stranger!")
	}
	return nil
}

func (w *Worker) ServeTask(query *conn.Query, reply *conn.Reply) (rerr error) {
	uo := flatbuffers.GetUOffsetT(query.Data)
	q := new(task.Query)
	q.Init(query.Data, uo)

	attr := string(q.Attr())
	if farm.Fingerprint64([]byte(attr))%numInstances == instanceIdx {
		reply.Data, rerr = ProcessTask(query.Data)
	} else {
		glog.Fatalf("Request sent to wrong server")
	}
	return rerr
}

func serveRequests(irwc io.ReadWriteCloser) {
	for {
		sc := &conn.ServerCodec{
			Rwc: irwc,
		}
		rpc.ServeRequest(sc)
	}
}

func runServer(address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		glog.Fatalf("While running server: %v", err)
		return err
	}
	glog.WithField("address", ln.Addr()).Info("Worker listening")

	go func() {
		for {
			cxn, err := ln.Accept()
			if err != nil {
				glog.Fatalf("listen(%q): %s\n", address, err)
				return
			}
			glog.WithField("local", cxn.LocalAddr()).
				WithField("remote", cxn.RemoteAddr()).
				Debug("Worker accepted connection")
			go serveRequests(cxn)
		}
	}()
	return nil
}
