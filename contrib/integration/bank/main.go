/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package main

import (
	"bufio"
	"bytes"
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
	users = flag.Int("users", 100, "Number of accounts.")
	conc  = flag.Int("txns", 10, "Number of concurrent transactions.")
	dur   = flag.String("dur", "1m", "How long to run the transactions.")
	addr  = flag.String("addr", "localhost:9080", "Address of Dgraph server.")
)

var startBal int = 10

type Account struct {
	Uid  string `json:"uid"`
	Key  int    `json:"key,omitempty"`
	Bal  int    `json:"bal,omitempty"`
	Type string `json:"type"`
}

type State struct {
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
	type: string @index(exact) @upsert .
	`
	x.Check(s.dg.Alter(context.Background(), &op))

	var all []Account
	for i := 1; i <= *users; i++ {
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
	txn.Sequencing(api.LinRead_SERVER_SIDE)
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
				uid
				key
				bal
			}
		}
	`
	txn := s.dg.NewTxn()
	txn.Sequencing(api.LinRead_SERVER_SIDE)
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
	var total int
	for _, a := range accounts {
		total += a.Bal
	}
	log.Printf("Read: %v. Total: %d\n", accounts, total)
	if len(accounts) > *users {
		log.Fatalf("len(accounts) = %d", len(accounts))
	}
	if total != *users*startBal {
		log.Fatalf("Total = %d", total)
	}
	return nil
}

func (s *State) findAccount(txn *dgo.Txn, key int) (Account, error) {
	// query := fmt.Sprintf(`{ q(func: eq(key, %d)) @filter(eq(type, "ba")) { key, uid, bal, type }}`, key)
	query := fmt.Sprintf(`{ q(func: eq(key, %d)) { key, uid, bal, type }}`, key)
	// log.Printf("findACcount: %s\n", query)
	resp, err := txn.Query(context.Background(), query)
	if err != nil {
		return Account{}, err
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
		log.Printf("Unable to find account for K_%02d. JSON: %s\n", key, resp.Json)
		return Account{Key: key, Type: "ba"}, nil
	}
	return accounts[0], nil
}

func (s *State) runTransaction(buf *bytes.Buffer) error {
	// w := os.Stdout
	w := bufio.NewWriter(buf)
	fmt.Fprintf(w, "==>\n")
	defer func() {
		fmt.Fprintf(w, "---\n")
		w.Flush()
	}()

	ctx := context.Background()
	txn := s.dg.NewTxn()
	txn.Sequencing(api.LinRead_SERVER_SIDE)
	defer txn.Discard(ctx)

	if rand.Intn(*users) < 2 {
		return s.runTotal()
	}

	var sk, sd int
	for {
		sk = rand.Intn(*users + 1)
		sd = rand.Intn(*users + 1)
		if sk == 0 || sd == 0 { // Don't touch zero.
			continue
		}
		if sk != sd {
			break
		}
	}

	src, err := s.findAccount(txn, sk)
	if err != nil {
		return err
	}
	dst, err := s.findAccount(txn, sd)
	if err != nil {
		return err
	}
	if src.Key == dst.Key {
		return nil
	}

	amount := rand.Intn(10)
	if src.Bal-amount <= 0 {
		amount = src.Bal
	}
	fmt.Fprintf(w, "Moving [$%d, K_%02d -> K_%02d]. Src:%+v. Dst: %+v\n",
		amount, src.Key, dst.Key, src, dst)
	src.Bal -= amount
	dst.Bal += amount
	var mu api.Mutation
	if len(src.Uid) > 0 {
		// If there was no src.Uid, then don't run any mutation.
		if src.Bal == 0 {
			// TODO: WHAT a fucking hack.
			d := map[string]string{"uid": src.Uid}
			pb, err := json.Marshal(d)
			x.Check(err)
			mu.DeleteJson = pb
			fmt.Fprintf(w, "Deleting K_%02d: %s\n", src.Key, mu.DeleteJson)
		} else {
			data, err := json.Marshal(src)
			x.Check(err)
			mu.SetJson = data
		}
		_, err := txn.Mutate(ctx, &mu)
		if err != nil {
			fmt.Fprintf(w, "Error while mutate: %v", err)
			return err
		}
	}

	mu = api.Mutation{}
	data, err := json.Marshal(dst)
	x.Check(err)
	mu.SetJson = data
	assigned, err := txn.Mutate(ctx, &mu)
	if err != nil {
		fmt.Fprintf(w, "Error while mutate: %v", err)
		return err
	}

	if err := txn.Commit(ctx); err != nil {
		return err
	}
	if len(assigned.GetUids()) > 0 {
		fmt.Fprintf(w, "CREATED K_%02d: %+v for %+v\n", dst.Key, assigned.GetUids(), dst)
		for _, uid := range assigned.GetUids() {
			dst.Uid = uid
		}
	}
	fmt.Fprintf(w, "MOVED [$%d, K_%02d -> K_%02d]. Src:%+v. Dst: %+v\n",
		amount, src.Key, dst.Key, src, dst)
	return nil
}

func (s *State) loop(wg *sync.WaitGroup) {
	defer wg.Done()
	dur, err := time.ParseDuration(*dur)
	if err != nil {
		log.Fatal(err)
	}
	end := time.Now().Add(dur)

	var buf bytes.Buffer
	for {
		buf.Reset()
		err := s.runTransaction(&buf)
		log.Printf("Final error: %v. %s", err, buf.String())
		if err != nil {
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

	conn, err := grpc.Dial(*addr, grpc.WithInsecure())
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
