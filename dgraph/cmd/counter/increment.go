/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// This binary would retrieve a value for UID=0x01, and increment it by 1. If
// successful, it would print out the incremented value. It assumes that it has
// access to UID=0x01, and that `val` predicate is of type int.
package counter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Increment x.SubCommand

func init() {
	Increment.Cmd = &cobra.Command{
		Use:   "increment",
		Short: "Increment a counter transactionally",
		Run: func(cmd *cobra.Command, args []string) {
			run(Increment.Conf)
		},
	}
	Increment.EnvPrefix = "DGRAPH_INCREMENT"

	flag := Increment.Cmd.Flags()
	flag.String("alpha", "localhost:9080", "Address of Dgraph Alpha.")
	flag.Int("num", 1, "How many times to run.")
	flag.Duration("wait", 0*time.Second, "How long to wait.")
	flag.String("user", "", "Username if login is required.")
	flag.String("password", "", "Password of the user.")
	flag.String("pred", "counter.val",
		"Predicate to use for storing the counter.")
	flag.Bool("ro", false,
		"Read-only. Read the counter value without updating it.")
	flag.Bool("be", false,
		"Best-effort. Read counter value without retrieving timestamp from Zero.")
	// TLS configuration
	x.RegisterClientTLSFlags(flag)
}

type Counter struct {
	Uid string `json:"uid"`
	Val int    `json:"val"`

	startTs  uint64 // Only used for internal testing.
	qLatency time.Duration
	mLatency time.Duration
}

func queryCounter(txn *dgo.Txn, pred string) (Counter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var counter Counter
	query := fmt.Sprintf("{ q(func: has(%s)) { uid, val: %s }}", pred, pred)
	resp, err := txn.Query(ctx, query)
	if err != nil {
		return counter, fmt.Errorf("Query error: %v", err)
	}

	// Total query latency is sum of encoding, parsing and processing latencies.
	queryLatency := resp.Latency.GetEncodingNs() +
		resp.Latency.GetParsingNs() + resp.Latency.GetProcessingNs()

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
	counter.qLatency = time.Duration(queryLatency).Round(time.Millisecond)
	return counter, nil
}

func process(dg *dgo.Dgraph, conf *viper.Viper) (Counter, error) {
	ro := conf.GetBool("ro")
	be := conf.GetBool("be")
	pred := conf.GetString("pred")
	var txn *dgo.Txn

	switch {
	case be:
		txn = dg.NewReadOnlyTxn().BestEffort()
	case ro:
		txn = dg.NewReadOnlyTxn()
	default:
		txn = dg.NewTxn()
	}
	defer txn.Discard(nil)

	counter, err := queryCounter(txn, pred)
	if err != nil {
		return Counter{}, err
	}
	if be || ro {
		return counter, nil
	}

	counter.Val++
	var mu api.Mutation
	mu.CommitNow = true
	if len(counter.Uid) == 0 {
		counter.Uid = "_:new"
	}
	mu.SetNquads = []byte(fmt.Sprintf(`<%s> <%s> "%d"^^<xs:int> .`, counter.Uid, pred, counter.Val))

	// Don't put any timeout for mutation.
	resp, err := txn.Mutate(context.Background(), &mu)
	if err != nil {
		return Counter{}, err
	}

	mutationLatency := resp.Latency.GetProcessingNs() +
		resp.Latency.GetParsingNs() + resp.Latency.GetEncodingNs()
	counter.mLatency = time.Duration(mutationLatency).Round(time.Millisecond)
	return counter, nil
}

func run(conf *viper.Viper) {
	startTime := time.Now()
	defer func() { fmt.Println("Total:", time.Since(startTime).Round(time.Millisecond)) }()

	alpha := conf.GetString("alpha")
	waitDur := conf.GetDuration("wait")
	num := conf.GetInt("num")

	tlsCfg, err := x.LoadClientTLSConfig(conf)
	x.CheckfNoTrace(err)

	format := "0102 03:04:05.999"

retryConn:
	conn, err := x.SetupConnection(alpha, tlsCfg, false)
	if err != nil {
		fmt.Printf("[%s] While trying to setup connection: %v. Retrying...",
			time.Now().UTC().Format(format), err)
		time.Sleep(time.Second)
		goto retryConn
	}
	dc := api.NewDgraphClient(conn)
	dg := dgo.NewDgraphClient(dc)
	if user := conf.GetString("user"); len(user) > 0 {
		x.CheckfNoTrace(dg.Login(context.Background(), user, conf.GetString("password")))
	}

	for num > 0 {
		txnStart := time.Now() // Start time of transaction
		cnt, err := process(dg, conf)
		now := time.Now().UTC().Format(format)
		if err != nil {
			fmt.Printf("%-17s While trying to process counter: %v. Retrying...\n", now, err)
			time.Sleep(time.Second)
			goto retryConn
		}
		serverLat := cnt.qLatency + cnt.mLatency
		clientLat := time.Since(txnStart).Round(time.Millisecond)
		fmt.Printf("%-17s Counter VAL: %d   [ Ts: %d ] Latency: Q %s M %s S %s C %s D %s\n", now, cnt.Val,
			cnt.startTs, cnt.qLatency, cnt.mLatency, serverLat, clientLat, clientLat-serverLat)
		num--
		time.Sleep(waitDur)
	}
}
