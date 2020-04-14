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

// Package counter builds a tool that retrieves a value for UID=0x01, and increments
// it by 1. If successful, it prints out the incremented value. It assumes that it has
// access to UID=0x01, and that `val` predicate is of type int.
package counter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opencensus.io/trace"
)

// Increment is the sub-command invoked when calling "dgraph increment".
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
	flag.Int("retries", 10, "How many times to retry setting up the connection.")
	flag.Duration("wait", 0*time.Second, "How long to wait.")
	flag.String("user", "", "Username if login is required.")
	flag.String("password", "", "Password of the user.")
	flag.String("pred", "counter.val",
		"Predicate to use for storing the counter.")
	flag.Bool("ro", false,
		"Read-only. Read the counter value without updating it.")
	flag.Bool("be", false,
		"Best-effort. Read counter value without retrieving timestamp from Zero.")
	flag.String("jaeger.collector", "", "Send opencensus traces to Jaeger.")
	// TLS configuration
	x.RegisterClientTLSFlags(flag)
}

// Counter stores information about the value being incremented by this tool.
type Counter struct {
	Uid string `json:"uid"`
	Val int    `json:"val"`

	startTs  uint64 // Only used for internal testing.
	qLatency time.Duration
	mLatency time.Duration
}

func queryCounter(ctx context.Context, txn *dgo.Txn, pred string) (Counter, error) {
	span := trace.FromContext(ctx)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var counter Counter
	query := fmt.Sprintf("{ q(func: has(%s)) { uid, val: %s }}", pred, pred)
	resp, err := txn.Query(ctx, query)
	if err != nil {
		return counter, errors.Wrapf(err, "while doing query")
	}

	m := make(map[string][]Counter)
	if err := json.Unmarshal(resp.Json, &m); err != nil {
		return counter, err
	}
	switch len(m["q"]) {
	case 0:
		// Do nothing.
	case 1:
		counter = m["q"][0]
	default:
		x.Panic(errors.Errorf("Invalid response: %q", resp.Json))
	}
	span.Annotatef(nil, "Found counter: %+v", counter)
	counter.startTs = resp.GetTxn().GetStartTs()
	counter.qLatency = time.Duration(resp.Latency.GetTotalNs()).Round(time.Millisecond)
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
	defer func() {
		if err := txn.Discard(context.Background()); err != nil {
			fmt.Printf("Discarding transaction failed: %+v\n", err)
		}
	}()

	ctx, span := trace.StartSpan(context.Background(), "Counter")
	defer span.End()

	counter, err := queryCounter(ctx, txn, pred)
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
	resp, err := txn.Mutate(ctx, &mu)
	if err != nil {
		return Counter{}, err
	}

	counter.mLatency = time.Duration(resp.Latency.GetTotalNs()).Round(time.Millisecond)
	return counter, nil
}

func run(conf *viper.Viper) {
	trace.ApplyConfig(trace.Config{
		DefaultSampler:             trace.AlwaysSample(),
		MaxAnnotationEventsPerSpan: 256,
	})
	x.RegisterExporters(conf, "dgraph.increment")

	startTime := time.Now()
	defer func() { fmt.Println("Total:", time.Since(startTime).Round(time.Millisecond)) }()

	waitDur := conf.GetDuration("wait")
	num := conf.GetInt("num")
	format := "0102 03:04:05.999"

	dg, closeFunc := x.GetDgraphClient(Increment.Conf, true)
	defer closeFunc()

	for num > 0 {
		txnStart := time.Now() // Start time of transaction
		cnt, err := process(dg, conf)
		now := time.Now().UTC().Format(format)
		if err != nil {
			fmt.Printf("%-17s While trying to process counter: %v. Retrying...\n", now, err)
			time.Sleep(time.Second)
			continue
		}
		serverLat := cnt.qLatency + cnt.mLatency
		clientLat := time.Since(txnStart).Round(time.Millisecond)
		fmt.Printf("%-17s Counter VAL: %d   [ Ts: %d ] Latency: Q %s M %s S %s C %s D %s\n", now, cnt.Val,
			cnt.startTs, cnt.qLatency, cnt.mLatency, serverLat, clientLat, clientLat-serverLat)
		num--
		time.Sleep(waitDur)
	}
}
