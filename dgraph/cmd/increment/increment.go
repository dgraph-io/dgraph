/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

// Package increment builds a tool that retrieves a value for UID=0x01, and increments
// it by 1. If successful, it prints out the incremented value. It assumes that it has
// access to UID=0x01, and that `val` predicate is of type int.
package increment

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opencensus.io/trace"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
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
		Annotations: map[string]string{"group": "tool"},
	}
	Increment.EnvPrefix = "DGRAPH_INCREMENT"
	Increment.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := Increment.Cmd.Flags()
	// --tls SuperFlag
	x.RegisterClientTLSFlags(flag)

	flag.String("cloud", "", "addr: xxx; jwt: xxx")
	flag.String("alpha", "localhost:9080", "Address of Dgraph Alpha.")
	flag.Int("num", 1, "How many times to run per goroutine.")
	flag.Int("retries", 10, "How many times to retry setting up the connection.")
	flag.Duration("wait", 0*time.Second, "How long to wait.")
	flag.Int("conc", 1, "How many goroutines to run.")

	flag.String("creds", "",
		`Various login credentials if login is required.
	user defines the username to login.
	password defines the password of the user.
	namespace defines the namespace to log into.
	Sample flag could look like --creds user=username;password=mypass;namespace=2`)

	flag.String("pred", "counter.val",
		"Predicate to use for storing the counter.")
	flag.Bool("ro", false,
		"Read-only. Read the counter value without updating it.")
	flag.Bool("be", false,
		"Best-effort. Read counter value without retrieving timestamp from Zero.")
	flag.String("jaeger", "", "Send opencensus traces to Jaeger.")
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
	conc := int(conf.GetInt("conc"))
	format := "0102 03:04:05.999"

	// Do a sanity check on the passed credentials.
	_ = z.NewSuperFlag(Increment.Conf.GetString("creds")).MergeAndCheckDefault(x.DefaultCreds)

	var dg *dgo.Dgraph
	sf := z.NewSuperFlag(conf.GetString("cloud"))
	if addr := sf.GetString("addr"); len(addr) > 0 {
		conn, err := dgo.DialSlashEndpoint(addr, sf.GetString("jwt"))
		x.Check(err)
		dc := api.NewDgraphClient(conn)
		dg = dgo.NewDgraphClient(dc)
	} else {
		dgTmp, closeFunc := x.GetDgraphClient(Increment.Conf, true)
		defer closeFunc()
		dg = dgTmp
	}

	processOne := func(i int) error {
		txnStart := time.Now() // Start time of transaction
		cnt, err := process(dg, conf)
		now := time.Now().UTC().Format(format)
		if err != nil {
			return err
		}
		serverLat := cnt.qLatency + cnt.mLatency
		clientLat := time.Since(txnStart).Round(time.Millisecond)
		fmt.Printf(
			"[w%d] %-17s Counter VAL: %d   [ Ts: %d ] Latency: Q %s M %s S %s C %s D %s\n",
			i, now, cnt.Val, cnt.startTs, cnt.qLatency, cnt.mLatency,
			serverLat, clientLat, clientLat-serverLat)
		return nil
	}

	// Run things serially first, if conc > 1.
	if conc > 1 {
		for i := 0; i < conc; i++ {
			err := processOne(0)
			x.Check(err)
			num--
		}
	}

	var wg sync.WaitGroup
	f := func(worker int) {
		defer wg.Done()
		count := 0
		for count < num {
			if err := processOne(worker); err != nil {
				now := time.Now().UTC().Format(format)
				fmt.Printf("%-17s While trying to process counter: %v. Retrying...\n", now, err)
				time.Sleep(time.Second)
				continue
			}
			time.Sleep(waitDur)
			count++
		}
	}

	for i := 0; i < conc; i++ {
		wg.Add(1)
		go f(i + 1)
	}
	wg.Wait()
}
