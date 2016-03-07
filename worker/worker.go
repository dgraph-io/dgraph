package worker

import (
	"flag"
	"io"
	"net"
	"net/rpc"

	"github.com/dgraph-io/dgraph/conn"
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

func Init(ps, uStore *store.Store, idx, numInst uint64) {
	dataStore = ps
	uidStore = uStore
	instanceIdx = idx
	numInstances = numInst
}

func Connect(workerList []string) {
	w := new(Worker)
	if err := rpc.Register(w); err != nil {
		glog.Fatal(err)
	}
	if err := runServer(*workerPort); err != nil {
		glog.Fatal(err)
	}
	if uint64(len(workerList)) != numInstances {
		glog.WithField("len(list)", len(workerList)).
			WithField("numInstances", numInstances).
			Fatalf("Wrong number of instances in workerList")
	}

	for _, addr := range workerList {
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

func (w *Worker) GetOrAssign(query *conn.Query,
	reply *conn.Reply) (rerr error) {

	uo := flatbuffers.GetUOffsetT(query.Data)
	xids := new(task.XidList)
	xids.Init(query.Data, uo)

	if instanceIdx != 0 {
		glog.WithField("instanceIdx", instanceIdx).
			WithField("GetOrAssign", true).
			Fatal("We shouldn't be receiving this request.")
	}
	reply.Data, rerr = getOrAssignUids(xids)
	return
}

func (w *Worker) Mutate(query *conn.Query, reply *conn.Reply) (rerr error) {
	m := new(Mutations)
	if err := m.Decode(query.Data); err != nil {
		return err
	}

	left := new(Mutations)
	if err := mutate(m, left); err != nil {
		return err
	}
	reply.Data, rerr = left.Encode()
	return
}

func (w *Worker) ServeTask(query *conn.Query, reply *conn.Reply) (rerr error) {
	uo := flatbuffers.GetUOffsetT(query.Data)
	q := new(task.Query)
	q.Init(query.Data, uo)
	attr := string(q.Attr())
	glog.WithField("attr", attr).WithField("instanceIdx", instanceIdx).Info("ServeTask")

	if attr == "_xid_" ||
		farm.Fingerprint64([]byte(attr))%numInstances == instanceIdx {

		reply.Data, rerr = processTask(query.Data)
	} else {
		glog.WithField("attribute", attr).
			WithField("instanceIdx", instanceIdx).
			Fatalf("Request sent to wrong server")
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
