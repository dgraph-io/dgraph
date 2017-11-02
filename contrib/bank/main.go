package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

var (
	users = flag.Int("users", 100, "Number of accounts.")
	conc  = flag.Int("txns", 10, "Number of concurrent transactions.")
	num   = flag.Int("num", 1e3, "Number of total transactions to run.")
)

type Account struct {
	Uid string `json:"_uid_"`
	Bal int    `json:"bal"`
}

type State struct {
	sync.RWMutex
	dg     *client.Dgraph
	uids   []string
	aborts int32
	runs   int32
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
		s.uids = append(s.uids, fmt.Sprintf("%#x", uid))
	}
}

func (s *State) runTotal() error {
	q := fmt.Sprintf(
		`
		{
			var(func: uid(%s)) {
				b as bal
			}
			total() {
				bal: sum(val(b))
			}
		}
	`, strings.Join(s.uids, ","))
	txn := s.dg.NewTxn()
	resp, err := txn.Query(context.Background(), q, nil)
	if err != nil {
		return err
	}
	fmt.Printf("\nresponse json: %q\n", resp.Json)
	return nil
}

func (s *State) runTotalInLoop() {
	for {
		err := s.runTotal()
		if err != nil {
			continue
		}
		time.Sleep(time.Second)
	}
}

func (s *State) runTransaction() error {
	ctx := context.Background()
	s.RLock()
	defer s.RUnlock()

	var from, to string
	for {
		from = s.uids[rand.Intn(len(s.uids))]
		to = s.uids[rand.Intn(len(s.uids))]
		if from != to {
			break
		}
	}

	txn := s.dg.NewTxn()
	fq := fmt.Sprintf(`{me(func: uid(%s, %s)) { _uid_, bal }}`, from, to)
	resp, err := txn.Query(ctx, fq, nil)
	if err != nil {
		return err
	}

	type Accounts struct {
		Both []Account `json:"me"`
	}
	var a Accounts
	if err := json.Unmarshal(resp.Json, &a); err != nil {
		return err
	}
	if len(a.Both) != 2 {
		return errors.New("Unable to find both accounts")
	}

	a.Both[0].Bal += 5
	a.Both[1].Bal -= 5

	var mu protos.Mutation
	data, err := json.Marshal(a.Both)
	x.Check(err)
	mu.SetJson = data
	_, err = txn.Mutate(ctx, &mu)
	if err != nil {
		return err
	}
	return txn.Commit(ctx)
}

func (s *State) loop(wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		if err := s.runTransaction(); err != nil {
			atomic.AddInt32(&s.aborts, 1)
		} else {
			r := atomic.AddInt32(&s.runs, 1)
			if r%100 == 0 {
				a := atomic.LoadInt32(&s.aborts)
				fmt.Printf("Runs: %d. Aborts: %d\r", r, a)
			}
			if int(r) >= *num {
				return
			}
		}
	}
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
	go s.runTotalInLoop()

	var wg sync.WaitGroup
	wg.Add(*conc)
	for i := 0; i < *conc; i++ {
		go s.loop(&wg)
	}
	wg.Wait()
	fmt.Println()
	fmt.Println("Total aborts", s.aborts)
	fmt.Println("Total success", s.runs)
	s.runTotal()
}
