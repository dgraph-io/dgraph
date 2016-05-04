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
	"os"
	"strconv"
	"time"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal"
	"github.com/coreos/etcd/wal/walpb"
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
	maxIdx    uint64 = 1
)

type node struct {
	id        uint64
	addr      string
	ctx       context.Context
	pstore    map[string]string
	store     *raft.MemoryStorage
	cfg       *raft.Config
	raft      raft.Node
	wal       *wal.WAL
	waldir    string
	lastIndex uint64
	ticker    <-chan time.Time
	done      <-chan struct{}
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

type helloReplyRPC struct {
	Id     uint64
	MaxIdx uint64
}

type keyvalRequest struct {
	Op  string
	Key string
	Val string
}

func (w *Worker) GetMaxIdFromMaster(query *conn.Query, reply *conn.Reply) error {
	reply.Data = []byte(cur_node.pstore["maxIdx"])
	return nil
}

func getMaxId(masterIP string) uint64 {
	pool := conn.NewPool(masterIP, 5)
	defer pool.Close() //TODO (Manish) pool doesnt support Close fully
	query := new(conn.Query)
	reply := new(conn.Reply)
	if err := pool.Call("Worker.GetMaxIdFromMaster", query, reply); err != nil {
		glog.WithField("call", "Worker.GetMaxIdFromMaster").Fatal(err)
	}

	id, _ := strconv.Atoi(string(reply.Data))
	return uint64(id)
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
		if v.Id != cur_node.id {
			fmt.Println("$\n$\n$\n$\n$\n$\n", cur_node.id, v.Id, "$\n$\n$\n$\n$\n$\n")
			go connectWith(v.Addr)
		}
	}
	fmt.Println("In Hello : ", v.Id, v.Addr)
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	nodeMaxIdx, _ := strconv.Atoi(cur_node.pstore["maxIdx"])
	err = enc.Encode(helloReplyRPC{cur_node.id, uint64(nodeMaxIdx)})
	fmt.Println("In Hello : ", cur_node.id, nodeMaxIdx)
	if err != nil {
		glog.Fatalf("encode error in Hello:", err)
	}
	reply.Data = network.Bytes()
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

// openWAL returns a WAL ready for reading.
func (n *node) openWAL() *wal.WAL {
	if !wal.Exist(n.waldir) {
		if err := os.Mkdir(n.waldir, 0750); err != nil {
			log.Fatalf("raftexample: cannot create dir for wal (%v)", err)
		}

		w, err := wal.Create(n.waldir, nil)
		if err != nil {
			log.Fatalf("raftexample: create wal error (%v)", err)
		}
		w.Close()
	}

	fmt.Println("Here in openWAL")
	w, err := wal.Open(n.waldir, walpb.Snapshot{})
	if err != nil {
		log.Fatalf("raftexample: error loading wal (%v)", err)
	}
	fmt.Println("Here in openWAL")

	return w
}

// replayWAL replays WAL entries into the raft instance.
func (n *node) replayWAL() *wal.WAL {
	w := n.openWAL()
	fmt.Println("Here in openWAL")
	_, st, ents, err := w.ReadAll()
	if err != nil {
		log.Fatalf("raftexample: failed to read WAL (%v)", err)
	}
	// append to storage so raft starts at the right place in log
	fmt.Println("Here")
	n.store.Append(ents)
	fmt.Println("Here")
	// send nil once lastIndex is published so client knows commit channel is current
	if len(ents) > 0 {
		n.lastIndex = ents[len(ents)-1].Index
	} /*else {
		n.commitC <- nil
	}*/
	fmt.Println("Here")
	n.store.SetHardState(st)
	fmt.Println("Here")
	return w
}

func newNode(id uint64, addr string, walD string) (*node, bool) {
	store := raft.NewMemoryStorage()
	var restart bool
	n := &node{
		id:     id,
		addr:   addr,
		ctx:    context.TODO(),
		store:  store,
		waldir: fmt.Sprintf(walD),
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

	fmt.Println("opening wal dir")
	oldwal := wal.Exist(n.waldir)
	n.wal = n.replayWAL()

	if oldwal {
		fmt.Println("found wal dir")
		restart = true
	}

	return n, restart
}

func (n *node) run() {
	defer n.wal.Close()
	fmt.Println("Starting RAFT")
	for {
		fmt.Println("...")
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
					if cc.Type == raftpb.ConfChangeRemoveNode {
						// Make sure the node doesn't use the same NodeId as the one that crashed
						if cur_node.id == cc.NodeID {
							glog.Fatalf("Cannot join with the ID %v, as this was removed from the cluster", cur_node.id)
						}
						glog.Infof("Removing node %v", cc.NodeID)
						RemoveStaleKeys(peers[cc.NodeID])
						delete(peers, cc.NodeID)
						delete(pools, cc.NodeID)
					}
				}

			}
			n.raft.Advance()
		case <-n.done:
			return
		}
	}
}

func (n *node) saveToStorage(hardState raftpb.HardState, entries []raftpb.Entry, snapshot raftpb.Snapshot) {
	n.wal.Save(hardState, entries)
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
		RemoveNodeFromCluster(message.To)
		//cur_node.raft.ReportUnreachable(message.To)
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
		for i := 0; i < 3; i++ {
			err = pool.Call("Worker.ReceiveOverNetwork", query, reply)
			if err == nil {
				break
			}

			glog.WithField("error", err).
				WithField("call", "Worker.ReceiveOverNetwork").
				Info("Retrying connection...")
			time.Sleep(10 * time.Second)
		}
		if err != nil {
			if cur_node.id == cur_node.raft.Status().Lead {
				glog.WithField("Id", message.To).Infof("cant reach node")
				//cur_node.raft.ReportUnreachable(message.To)

				RemoveNodeFromCluster(message.To)
			}
			return
		}
	}
}

func RemoveNodeFromCluster(id uint64) {
	confCount++
	cur_node.raft.ProposeConfChange(cur_node.ctx, raftpb.ConfChange{
		ID:      confCount,
		Type:    raftpb.ConfChangeRemoveNode,
		NodeID:  id,
		Context: []byte(""),
	})
	//RemoveStaleKeys(peers[id])
	// Has to be done through raft message
	/*
		delete(peers, id)
		delete(pools, id)
		requestOtherNodesToRemove(id)
	*/
}

func RemoveStaleKeys(addr string) {
	glog.Infof("%v", addr)
	for k, v := range cur_node.pstore {
		if addr == v {
			prop := &keyvalRequest{
				Op:  "Del",
				Key: k,
				Val: "",
			}

			var buf bytes.Buffer
			enc := gob.NewEncoder(&buf)
			err := enc.Encode(prop)
			if err != nil {
				glog.Fatalf("encode:", err)
			}
			propByte := buf.Bytes()
			cur_node.raft.Propose(cur_node.ctx, propByte)
		}
	}
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

	buf := bytes.NewBuffer(reply.Data)
	dec := gob.NewDecoder(buf)
	var v helloReplyRPC
	err = dec.Decode(&v)
	if err != nil {
		glog.Fatal("decode:", err)
	}

	fmt.Println("In connectWith : ", v.Id, v.MaxIdx)
	pools[v.Id] = pool
	peers[v.Id] = pool.Addr
	return v.Id
}

func proposeJoin(id uint64) {
	pool := pools[id]
	addr := pool.Addr
	fmt.Println(addr)
	query := new(conn.Query)
	query.Data = []byte(strconv.Itoa(int(cur_node.id)))
	reply := new(conn.Reply)

	co := 0
	for cur_node.raft.Status().Lead != id && co < 30 {
		glog.Info("Trying to connect with master")
		if err := pool.Call("Worker.JoinCluster", query, reply); err != nil {
			glog.WithField("call", "Worker.JoinCluster").Fatal(err)
		}
		glog.WithField("addr", addr).Info("Trying to join master")
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
func printPeerList() {
	for {
		fmt.Println("$$$$$$$$$$$$$$$")
		for k, v := range peers {
			fmt.Println(k, ":", v)
		}
		fmt.Println("$$$$$$$$$$$$$$$")
		time.Sleep(5 * time.Second)
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
			fmt.Println(k, v)
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
	walDir = flag.String("waldir", "",
		"wal directory to persist logs")
	postingdir = flag.String("posting", "", "UID directory")
	clusterIP  = flag.String("clusterIP", "", "IP of a node in cluster")
	cur_node   *node
	isRestart  bool
)

func main() {
	flag.Parse()

	cur_node, isRestart = newNode(maxIdx, "", *walDir)

	if err := rpc.Register(w); err != nil {
		glog.Fatal(err)
	}
	if err := runServer(*workerPort); err != nil {
		glog.Fatal(err)
	}

	var predList []string
	if *postingdir != "" {
		ps1 := new(store.Store)
		ps1.Init(*postingdir)
		defer ps1.Close()

		predList = cluster.GetPredicateList(ps1)
	}

	go printPeerList()

	if *clusterIP != "" {
		master_ip := getMasterIp(*clusterIP)

		assignId := getMaxId(master_ip)

		cur_node.id = assignId
		cur_node.cfg.ID = assignId

		master_id := connectWith(master_ip)
		getPeerListFrom(master_id)
		peers[assignId] = *workerPort
		connectWithPeers()

		if isRestart {
			fmt.Println("found wal dir")
			cur_node.raft = raft.RestartNode(cur_node.cfg)
		} else {
			cur_node.raft = raft.StartNode(cur_node.cfg, []raft.Peer{{ID: assignId}})
		}
		go cur_node.run()
		if !isRestart {
			proposeJoin(master_id)
			maxIdx = assignId + 1

			prop := &keyvalRequest{
				Op:  "Set",
				Key: "maxIdx",
				Val: strconv.Itoa(int(maxIdx)),
			}
			var buf bytes.Buffer
			enc := gob.NewEncoder(&buf)
			err := enc.Encode(prop)
			if err != nil {
				glog.Fatalf("encode:", err)
			}
			propByte := buf.Bytes()
			cur_node.raft.Propose(cur_node.ctx, propByte)
		}
	} else {
		peers[maxIdx] = *workerPort
		connectWithPeers()
		cur_node.raft = raft.StartNode(cur_node.cfg, []raft.Peer{{ID: maxIdx}})
		go cur_node.run()
		cur_node.raft.Campaign(cur_node.ctx)

		maxIdx++

		prop := &keyvalRequest{
			Op:  "Set",
			Key: "maxIdx",
			Val: strconv.Itoa(int(maxIdx)),
		}
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		err := enc.Encode(prop)
		if err != nil {
			glog.Fatalf("encode:", err)
		}
		propByte := buf.Bytes()
		cur_node.raft.Propose(cur_node.ctx, propByte)
	}

	fmt.Println("proposal by node ", cur_node.id)
	nodeID := strconv.Itoa(int(cur_node.id))

	if *postingdir != "" {
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
			cur_node.raft.Propose(cur_node.ctx, propByte)
		}
	} else {
		fmt.Println(predList)
		prop := &keyvalRequest{
			Op:  "Set",
			Key: nodeID,
			Val: *workerPort,
		}

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		err := enc.Encode(prop)
		if err != nil {
			glog.Fatalf("encode:", err)
		}
		propByte := buf.Bytes()
		cur_node.raft.Propose(cur_node.ctx, propByte)

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
