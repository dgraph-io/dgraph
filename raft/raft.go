package main

import (
	"bytes"
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/rpc"
	"strconv"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/dgraph-io/dgraph/cluster"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
)

var (
	hb               = 1
	pools            = make(map[uint64]*conn.Pool)
	glog             = x.Log("RAFT")
	peers            = make(map[uint64]string)
	confCount uint64 = 0
)

type node struct {
	id     uint64
	addr   string
	ctx    context.Context
	pstore map[string]string
	store  *raft.MemoryStorage
	cfg    *raft.Config
	raft   raft.Node
	ticker <-chan time.Time
	done   <-chan struct{}
}

type Worker struct {
}

type raftRPC struct {
	Ctx     context.Context
	Message raftpb.Message
}

type helloRPC struct {
	Id   uint64
	Addr string
}

type keyvalRequest struct {
	Op  string
	Key string
	Val string
}

func (w *Worker) Hello(query *conn.Query, reply *conn.Reply) error {
	buf := bytes.NewBuffer(query.Data)
	dec := gob.NewDecoder(buf)
	var v helloRPC
	err := dec.Decode(&v)
	if err != nil {
		glog.Fatal("decode:", err)
	}

	if _, ok := pools[v.Id]; !ok {
		go connectWith(v.Addr)
	}
	reply.Data = []byte(strconv.Itoa(int(cur_node.id)))

	fmt.Println("In Hello")
	return nil
}

func (w *Worker) JoinCluster(query *conn.Query, reply *conn.Reply) error {
	i, _ := strconv.Atoi(string(query.Data))
	id := uint64(i)
	confCount++
	cur_node.raft.ProposeConfChange(cur_node.ctx, raftpb.ConfChange{
		ID:      confCount,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  id,
		Context: []byte(""),
	})
	return nil
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

func newNode(id uint64, addr string, peers []raft.Peer) *node {
	store := raft.NewMemoryStorage()
	n := &node{
		id:    id,
		addr:  addr,
		ctx:   context.TODO(),
		store: store,
		cfg: &raft.Config{
			ID:              id,
			ElectionTick:    5 * hb,
			HeartbeatTick:   hb,
			Storage:         store,
			MaxSizePerMsg:   math.MaxUint16,
			MaxInflightMsgs: 256,
		},
		pstore: make(map[string]string),
		ticker: time.Tick(time.Second),
		done:   make(chan struct{}),
	}

	n.raft = raft.StartNode(n.cfg, peers)
	return n
}

func (n *node) run() {
	for {
		select {
		case <-n.ticker:
			n.raft.Tick()
		case rd := <-n.raft.Ready():
			n.saveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			n.send(rd.Messages)
			if !raft.IsEmptySnap(rd.Snapshot) {
				n.processSnapshot(rd.Snapshot)
			}
			for _, entry := range rd.CommittedEntries {
				n.process(entry)
				if entry.Type == raftpb.EntryConfChange {
					var cc raftpb.ConfChange
					cc.Unmarshal(entry.Data)
					n.raft.ApplyConfChange(cc)
				}
			}
			n.raft.Advance()
		case <-n.done:
			return
		}
	}
}

func (n *node) saveToStorage(hardState raftpb.HardState, entries []raftpb.Entry, snapshot raftpb.Snapshot) {
	n.store.Append(entries)

	if !raft.IsEmptyHardState(hardState) {
		n.store.SetHardState(hardState)
	}

	if !raft.IsEmptySnap(snapshot) {
		n.store.ApplySnapshot(snapshot)
	}
}

func (n *node) send(messages []raftpb.Message) {
	for _, m := range messages {
		log.Println(raft.DescribeMessage(m, nil))

		go sendOverNetwork(n.ctx, m)
	}
}

func sendOverNetwork(ctx context.Context, message raftpb.Message) {
	pool, ok := pools[message.To]
	if !ok {
		glog.WithField("From", cur_node.id).WithField("To", message.To).
			Error("Error in making connetions")
		return
	}
	addr := pool.Addr
	fmt.Println(addr)
	query := new(conn.Query)

	var network bytes.Buffer
	gob.Register(ctx)
	enc := gob.NewEncoder(&network)
	err := enc.Encode(raftRPC{ctx, message})
	if err != nil {
		glog.Fatalf("encode:", err)
	}

	query.Data = network.Bytes()
	reply := new(conn.Reply)
	if err := pool.Call("Worker.ReceiveOverNetwork", query, reply); err != nil {
		glog.WithField("call", "Worker.ReceiveOverNetwork").Error(err)
		if cur_node.id == cur_node.raft.Status().Lead {
			glog.WithField("Id", message.To).Infof("Removing a node")
			RemoveNodeFromCluster(message.To)
		}
		return
	}
	glog.WithField("reply_len", len(reply.Data)).WithField("addr", addr).
		Info("Got reply from server")

}

func RemoveNodeFromCluster(id uint64) {
	confCount++
	cur_node.raft.ProposeConfChange(cur_node.ctx, raftpb.ConfChange{
		ID:      confCount,
		Type:    raftpb.ConfChangeRemoveNode,
		NodeID:  id,
		Context: []byte(""),
	})
	delete(peers, id)
	delete(pools, id)
	requestOtherNodesToRemove(id)
}

func requestOtherNodesToRemove(id uint64) {
	for idRem, _ := range peers {
		go removeFromPeerList(strconv.Itoa(int(id)), idRem)
	}
}

func removeFromPeerList(id string, idRem uint64) {
	pool := pools[idRem]
	query := new(conn.Query)
	query.Data = []byte(id)
	reply := new(conn.Reply)
	if err := pool.Call("Worker.RemovePeer", query, reply); err != nil {
		glog.WithField("call", "Worker.RemovePeer").Fatal(err)
	}
}

func (w *Worker) RemovePeer(query *conn.Query, reply *conn.Reply) error {
	id, _ := strconv.Atoi(string(query.Data))
	id1 := uint64(id)
	delete(peers, id1)
	delete(pools, id1)
	return nil
}

func (w *Worker) ReceiveOverNetwork(query *conn.Query, reply *conn.Reply) error {
	buf := bytes.NewBuffer(query.Data)
	dec := gob.NewDecoder(buf)
	gob.Register(context.Background())
	var v raftRPC
	err := dec.Decode(&v)
	if err != nil {
		glog.Fatal("decode:", err)
	}
	cur_node.receive(v.Ctx, v.Message)

	return nil
}

func (n *node) processSnapshot(snapshot raftpb.Snapshot) {
	panic(fmt.Sprintf("Applying snapshot on node %v is not implemented", n.id))
}

func (n *node) process(entry raftpb.Entry) {
	log.Printf("node %v: processing entry: %v\n", n.id, entry)
	if entry.Type == raftpb.EntryNormal && entry.Data != nil {
		buf := bytes.NewBuffer(entry.Data)
		dec := gob.NewDecoder(buf)
		var v keyvalRequest
		err := dec.Decode(&v)
		if err != nil {
			glog.Fatal("decode keyvalRequest:", err)
		}

		if v.Op == "Set" {
			n.pstore[v.Key] = v.Val
		} else if v.Op == "Del" {
			delete(n.pstore, v.Key)
		}
	}
}

func (n *node) receive(ctx context.Context, message raftpb.Message) {
	n.raft.Step(ctx, message)
}

func connectWith(addr string) uint64 {
	if len(addr) == 0 {
		return 0
	}
	pool := conn.NewPool(addr, 5)
	query := new(conn.Query)
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(helloRPC{cur_node.id, *workerPort})
	if err != nil {
		glog.Fatalf("encode:", err)
	}
	query.Data = network.Bytes()

	reply := new(conn.Reply)
	if err := pool.Call("Worker.Hello", query, reply); err != nil {
		glog.WithField("call", "Worker.Hello").Fatal(err)
	}
	i, _ := strconv.Atoi(string(reply.Data))
	glog.WithField("reply", i).WithField("addr", addr).
		Info("Got reply from server")

	pools[uint64(i)] = pool
	peers[uint64(i)] = pool.Addr
	return uint64(i)
}

func proposeJoin(id uint64) {
	pool := pools[id]
	addr := pool.Addr
	fmt.Println(addr)
	query := new(conn.Query)
	query.Data = []byte(strconv.Itoa(int(cur_node.id)))
	reply := new(conn.Reply)

	co := 0
	for cur_node.raft.Status().Lead != id && co < 3330 {
		glog.Info("Trying to connect with master")
		if err := pool.Call("Worker.JoinCluster", query, reply); err != nil {
			glog.WithField("call", "Worker.JoinCluster").Fatal(err)
		}
		glog.WithField("reply_len", len(reply.Data)).WithField("addr", addr).
			Info("Got reply from server")
		time.Sleep(1000 * time.Millisecond) // sleep for a second and rety joining the cluster
		co++
	}

	if cur_node.raft.Status().Lead != id {
		glog.Fatalf("Unable to joing the cluster")
	}
}
func (w *Worker) GetPeers(query *conn.Query, reply *conn.Reply) error {
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(peers)
	if err != nil {
		glog.Fatalf("encode:", err)
	}

	reply.Data = network.Bytes()
	return nil
}

func getPeerListFrom(id uint64) {
	pool := pools[id]
	addr := pool.Addr
	query := new(conn.Query)
	reply := new(conn.Reply)
	fmt.Println("Got Peer List")
	if err := pool.Call("Worker.GetPeers", query, reply); err != nil {
		glog.WithField("call", "Worker.GetPeers").Fatal(err)
	}
	fmt.Println("Got Peer List")
	glog.WithField("reply_len", len(reply.Data)).WithField("addr", addr).
		Info("Got peerList from server")

	buf := bytes.NewBuffer(reply.Data)
	dec := gob.NewDecoder(buf)
	var v = make(map[uint64]string)
	err := dec.Decode(&v)
	if err != nil {
		glog.Fatal("decode:", err)
	}
	fmt.Println("Got Peer List")
	updatePeerList(v)
}

func checkConnection(id uint64) error {
	pool := pools[id]
	query := new(conn.Query)
	reply := new(conn.Reply)
	if err := pool.Call("Worker.Ping", query, reply); err != nil {
		return err
	}
	return nil
}

func (w *Worker) Ping(query *conn.Query, reply *conn.Reply) error {
	reply.Data = []byte("reachable")
	return nil
}

func checkPeerList() {
	for k, _ := range peers {
		err := checkConnection(k)
		if err != nil {
			delete(peers, k)
			delete(pools, k)
		}
	}
	fmt.Println("####################")
	for k, _ := range pools {
		fmt.Println(k)
	}
	fmt.Println("####################")
}

func trackPeerList() {
	for {
		time.Sleep(10 * time.Second)
		go checkPeerList()
	}
}

func updatePeerList(pl map[uint64]string) {
	for k, v := range pl {
		if _, ok := pools[k]; !ok {
			peers[k] = v
		}
	}
}

func connectWithPeers() {
	for k, v := range peers {
		if _, ok := pools[k]; !ok {
			go connectWith(v)
		}
	}
}

func (w *Worker) GetMasterIP(query *conn.Query, reply *conn.Reply) error {
	buf := bytes.NewBuffer(query.Data)
	dec := gob.NewDecoder(buf)
	var v helloRPC
	err := dec.Decode(&v)
	if err != nil {
		glog.Fatal("decode:", err)
	}

	if _, ok := pools[v.Id]; !ok {
		go connectWith(v.Addr)
	}
	reply.Data = []byte(peers[cur_node.raft.Status().Lead])
	fmt.Println("In Hello")
	return nil
}

func getMasterIp(ip string) string {
	if len(ip) == 0 {
		return ""
	}
	pool := conn.NewPool(ip, 5)
	query := new(conn.Query)
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(helloRPC{cur_node.id, *workerPort})
	if err != nil {
		glog.Fatalf("encode:", err)
	}
	query.Data = network.Bytes()

	reply := new(conn.Reply)
	if err := pool.Call("Worker.GetMasterIP", query, reply); err != nil {
		glog.WithField("call", "Worker.GetMasterIP").Fatal(err)
	}
	masterIP := string(reply.Data)
	glog.WithField("reply", masterIP).WithField("addr", ip).
		Info("Got reply from server")

	return masterIP
}

var (
	nodes      = make(map[int]*node)
	w          = new(Worker)
	workerPort = flag.String("workerport", ":12345",
		"Port used by worker for internal communication.")
	instanceIdx = flag.Uint64("idx", 1,
		"raft instance id")
	postingdir = flag.String("posting", "", "UID directory")
	clusterIP  = flag.String("clusterIP", "", "IP of a node in cluster")
	cur_node   *node
)

func main() {
	flag.Parse()
	cur_node = newNode(*instanceIdx, "", []raft.Peer{{ID: *instanceIdx}})
	if err := rpc.Register(w); err != nil {
		glog.Fatal(err)
	}
	if err := runServer(*workerPort); err != nil {
		glog.Fatal(err)
	}

	ps1 := new(store.Store)
	ps1.Init(*postingdir)
	defer ps1.Close()

	predList := cluster.GetPredicateList(ps1)

	peers[*instanceIdx] = *workerPort

	if *clusterIP != "" {
		master_ip := getMasterIp(*clusterIP)
		master_id := connectWith(master_ip)
		getPeerListFrom(master_id)
		connectWithPeers()
		go cur_node.run()
		proposeJoin(master_id)
	} else {
		connectWithPeers()
		go cur_node.run()
		cur_node.raft.Campaign(cur_node.ctx)
	}

	go trackPeerList()

	fmt.Println("proposal by node ", cur_node.id)
	//nodeID := strconv.Itoa(int(cur_node.id))

	for _, pred := range predList {
		prop := &keyvalRequest{
			Op:  "Set",
			Key: pred,
			Val: *workerPort,
		}

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		err := enc.Encode(prop)
		if err != nil {
			glog.Fatalf("encode:", err)
		}
		propByte := buf.Bytes()
		cur_node.raft.Propose(cur_node.ctx, []byte(propByte))
	}

	for {
		fmt.Printf("** Node %v **\n", cur_node.id)
		for k, v := range cur_node.pstore {
			fmt.Printf("%v = %v\n", k, v)
		}
		fmt.Printf("*************\n")
		time.Sleep(5 * time.Second)
	}
}
