/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

// This binary would retrieve a value for UID=0x01, and increment it by 1. If
// successful, it would print out the incremented value. It assumes that it has
// access to UID=0x01, and that `val` predicate is of type int.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"google.golang.org/grpc"
)

var (
	addr = flag.String("addr", "localhost:9080", "Address of Dgraph server.")
	num  = flag.Int("num", 1, "How many times to run.")
	ro   = flag.Bool("ro", false, "Only read the counter value, don't update it.")
	wait = flag.String("wait", "0", "How long to wait.")
	pred = flag.String("pred", "counter.val", "Predicate to use for storing the counter.")
)

type Counter struct {
	Uid string `json:"uid"`
	Val int    `json:"val"`

	startTs uint64 // Only used for internal testing.
}

func queryCounter(txn *dgo.Txn) (Counter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var counter Counter
	query := fmt.Sprintf("{ q(func: has(%s)) { uid, val: %s }}", *pred, *pred)
	resp, err := txn.Query(ctx, query)
	if err != nil {
		return counter, err
	}
	m := make(map[string][]Counter)
	if err := json.Unmarshal(resp.Json, &m); err != nil {
		return counter, err
	}
	if len(m["q"]) == 0 {
		// Do nothing.
	} else if len(m["q"]) == 1 {
		counter = m["q"][0]
	} else {
		panic(fmt.Sprintf("Invalid response: %q", resp.Json))
	}
	counter.startTs = resp.GetTxn().GetStartTs()
	return counter, nil
}

func process(dg *dgo.Dgraph, readOnly bool) (Counter, error) {
	if readOnly {
		txn := dg.NewReadOnlyTxn()
		defer txn.Discard(nil)
		counter, err := queryCounter(txn)
		return counter, err
	}

	txn := dg.NewTxn()
	counter, err := queryCounter(txn)
	if err != nil {
		return Counter{}, err
	}
	counter.Val += 1

	var mu api.Mutation
	if len(counter.Uid) == 0 {
		counter.Uid = "_:new"
	}
	mu.SetNquads = []byte(fmt.Sprintf(`<%s> <%s> "%d"^^<xs:int> .`, counter.Uid, *pred, counter.Val))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = txn.Mutate(ctx, &mu)
	if err != nil {
		return Counter{}, err
	}
	return counter, txn.Commit(ctx)
}

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*addr, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)
	dg := dgo.NewDgraphClient(dc)

	waitDur, err := time.ParseDuration(*wait)
	if err != nil {
		log.Fatal(err)
	}

	for *num > 0 {
		cnt, err := process(dg, *ro)
		if err != nil {
			fmt.Printf("While trying to process counter: %v. Retrying...\n", err)
			time.Sleep(time.Second)
			continue
		}
		fmt.Printf("Counter VAL: %d   [ Ts: %d ]\n", cnt.Val, cnt.startTs)
		*num -= 1
		time.Sleep(waitDur)
	}
}
