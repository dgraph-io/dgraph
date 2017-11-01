package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

var (
	users = flag.Int("users", 5, "Number of accounts.")
	conc  = flag.Int("txns", 100, "Number of concurrent transactions.")
)

type Account struct {
	Uid uint64 `json:"_uid_"`
	Bal int    `json:"bal"`
}

type State struct {
	sync.RWMutex
	dg   *client.Dgraph
	uids []uint64
}

func (s *State) createAccounts() {
	op := protos.Operation{DropAll: true}
	x.Check(s.dg.Alter(context.Background(), &op))

	op.DropAll = false
	op.Schema = `bal: int .`
	x.Check(s.dg.Alter(context.Background(), &op))

	var all []Account
	for i := 0; i < *users; i++ {
		all = append(all, Account{Bal: 100})
	}
	data, err := json.Marshal(all)
	x.Check(err)

	txn := s.dg.NewTxn()
	var mu protos.Mutation
	mu.SetJson = data
	assigned, err := txn.Mutate(context.Background(), &mu)
	x.Check(err)
	x.Check(txn.Commit(context.Background()))

	s.Lock()
	defer s.Unlock()
	for _, uid := range assigned.GetUids() {
		s.uids = append(s.uids, uid)
	}
}

func (s *State) runTransaction() error {
	ctx := context.Background()
	s.RLock()
	defer s.RUnlock()

	from := s.uids[rand.Intn(len(s.uids))]
	to := s.uids[rand.Intn(len(s.uids))]

	txn := s.dg.NewTxn()
	fq := fmt.Sprintf(`{me(func: uid(%d, %d)) { _uid_, bal }}`, from, to)
	resp, err := txn.Query(ctx, fq, nil)
	if err != nil {
		return err
	}

	type S struct {
		both []Account `json:"me"`
	}
	var s S
	if err := json.Unmarshal(resp.Json, &s); err != nil {
		return err
	}
	if len(s.both) != 2 {
		return errors.New("Unable to find both accounts")
	}
	s.both[0].Bal
}

func main() {
	flag.Parse()
	conn, err := grpc.Dial("localhost:8888", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	zero := protos.NewZeroClient(conn)

	conn, err = grpc.Dial("localhost:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := protos.NewDgraphClient(conn)

	dg := client.NewDgraphClient(zero, dc)
	s := State{dg: dg}
	s.createAccounts()
	fmt.Printf("s.uids: %v\n", s.uids)
}
