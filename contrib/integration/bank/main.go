/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/x"
	"google.golang.org/grpc"
)

var (
	users = flag.Int("users", 10, "Number of accounts.")
	conc  = flag.Int("txns", 5, "Number of concurrent transactions.")
	dur   = flag.String("dur", "1m", "How long to run the transactions.")
)

var startBal int = 10

type Account struct {
	Uid  string `json:"uid"`
	Key  int    `json:"key,omitempty"`
	Bal  int    `json:"bal,omitempty"`
	Type string `json:"type"`
}

type State struct {
	sync.RWMutex
	dg     *dgo.Dgraph
	aborts int32
	runs   int32
}

func (s *State) createAccounts() {
	op := api.Operation{DropAll: true}
	x.Check(s.dg.Alter(context.Background(), &op))

	op.DropAll = false
	op.Schema = `
	key: int @index(int) @upsert .
	bal: int .
	type: string @index(exact) .
	`
	x.Check(s.dg.Alter(context.Background(), &op))

	var all []Account
	for i := 0; i < *users; i++ {
		a := Account{
			Key:  i,
			Bal:  startBal,
			Type: "ba",
		}
		all = append(all, a)
	}
	data, err := json.Marshal(all)
	x.Check(err)

	txn := s.dg.NewTxn()
	defer txn.Discard(context.Background())
	var mu api.Mutation
	mu.SetJson = data
	log.Printf("mutation: %s\n", mu.SetJson)
	_, err = txn.Mutate(context.Background(), &mu)
	x.Check(err)
	x.Check(txn.Commit(context.Background()))
}

func (s *State) runTotal() error {
	query := `
		{
			q(func: eq(type, "ba")) {
				key
				bal
			}
		}
	`
	txn := s.dg.NewTxn()
	defer txn.Discard(context.Background())
	resp, err := txn.Query(context.Background(), query)
	if err != nil {
		return err
	}

	m := make(map[string][]Account)
	if err := json.Unmarshal(resp.Json, &m); err != nil {
		return err
	}
	accounts := m["q"]
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Key < accounts[j].Key
	})
	log.Printf("Read: %v\n", accounts)
	if len(accounts) != *users {
		log.Fatalf("len(accounts) = %d", len(accounts))
	}
	var total int
	for _, a := range accounts {
		total += a.Bal
	}
	if total != *users*startBal {
		log.Fatalf("Total = %d", total)
	}
	return nil
}

func (s *State) findAccount(txn *dgo.Txn, key int) Account {
	query := fmt.Sprintf(`{ q(func: eq(key, %d)) { key, uid, bal, type }}`, key)
	// log.Printf("findACcount: %s\n", query)
	resp, err := txn.Query(context.Background(), query)
	if err != nil {
		log.Fatal(err)
	}
	m := make(map[string][]Account)
	if err := json.Unmarshal(resp.Json, &m); err != nil {
		log.Fatal(err)
	}
	accounts := m["q"]
	if len(accounts) > 1 {
		log.Printf("Query: %s. Response: %s\n", query, resp.Json)
		log.Fatal("Found multiple accounts")
	}
	if len(accounts) == 0 {
		return Account{Key: key, Type: "ba"}
	}
	return accounts[0]
}

func (s *State) runTransaction() error {
	ctx := context.Background()
	s.RLock()
	defer s.RUnlock()

	txn := s.dg.NewTxn()
	defer txn.Discard(ctx)

	if rand.Intn(*users) < 2 {
		return s.runTotal()
	}

	src := s.findAccount(txn, rand.Intn(*users))
	dst := s.findAccount(txn, rand.Intn(*users))
	if src.Key == dst.Key {
		return nil
	}

	amount := rand.Intn(10)
	if src.Bal-amount <= 0 {
		dst.Bal += src.Bal
		src.Bal = 0
	} else {
		src.Bal -= amount
		dst.Bal += amount
	}

	log.Printf("Moving [$%d, %d->%d]. Src:%+v. Dst: %+v\n", amount, src.Key, dst.Key, src, dst)
	var mu api.Mutation
	if len(src.Uid) > 0 && src.Bal == 0 {
		src.Key = 0
		src.Type = "*"
		data, err := json.Marshal(src)
		x.Check(err)
		mu.DeleteJson = data
		log.Printf("Deleting: %s\n", mu.DeleteJson)
	} else {
		data, err := json.Marshal(src)
		x.Check(err)
		mu.SetJson = data
	}
	_, err := txn.Mutate(ctx, &mu)
	if err != nil {
		log.Printf("Error while mutate: %v", err)
		return err
	}

	mu = api.Mutation{}
	data, err := json.Marshal(dst)
	x.Check(err)
	mu.SetJson = data
	_, err = txn.Mutate(ctx, &mu)
	if err != nil {
		log.Printf("Error while mutate: %v", err)
		return err
	}

	if err := txn.Commit(ctx); err != nil {
		return err
	}
	log.Printf("MOVED [$%d, %d->%d]. Src:%+v. Dst: %+v\n", amount, src.Key, dst.Key, src, dst)
	return nil
}

func (s *State) loop(wg *sync.WaitGroup) {
	defer wg.Done()
	dur, err := time.ParseDuration(*dur)
	if err != nil {
		log.Fatal(err)
	}
	end := time.Now().Add(dur)

	for {
		if err := s.runTransaction(); err != nil {
			atomic.AddInt32(&s.aborts, 1)
		} else {
			r := atomic.AddInt32(&s.runs, 1)
			if r%100 == 0 {
				a := atomic.LoadInt32(&s.aborts)
				fmt.Printf("Runs: %d. Aborts: %d\r", r, a)
			}
			if time.Now().After(end) {
				return
			}
		}
	}
}

func main() {
	flag.Parse()

	conn, err := grpc.Dial("localhost:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)

	dg := dgo.NewDgraphClient(dc)
	s := State{dg: dg}
	s.createAccounts()

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
