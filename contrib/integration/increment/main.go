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
	wait = flag.String("wait", "0", "How long to wait.")
)

type Counter struct {
	Uid string `json:"uid"`
	Val int    `json:"counter.val"`
}

func increment(dg *dgo.Dgraph) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	txn := dg.NewTxn()
	resp, err := txn.Query(ctx, `{ q(func: has(counter.val)) { uid, counter.val }}`)
	if err != nil {
		return 0, err
	}
	m := make(map[string][]Counter)
	if err := json.Unmarshal(resp.Json, &m); err != nil {
		return 0, err
	}
	var counter Counter
	if len(m["q"]) == 0 {
		// Do nothing.
	} else if len(m["q"]) == 1 {
		counter = m["q"][0]
	} else {
		log.Fatalf("Invalid response: %q", resp.Json)
	}
	counter.Val += 1

	var mu api.Mutation
	data, err := json.Marshal(counter)
	if err != nil {
		return 0, err
	}
	mu.SetJson = data
	_, err = txn.Mutate(ctx, &mu)
	if err != nil {
		return 0, err
	}
	return counter.Val, txn.Commit(ctx)
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
		val, err := increment(dg)
		if err != nil {
			fmt.Printf("While trying to increment counter: %v. Retrying...\n", err)
			time.Sleep(time.Second)
			continue
		}
		fmt.Printf("Counter SET OK: %d\n", val)
		*num -= 1
		time.Sleep(waitDur)
	}
}
