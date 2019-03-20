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
	"log"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
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

	flag := Increment.Cmd.Flags()
	flag.String("addr", "localhost:9080", "Address of Dgraph alpha.")
	flag.Int("num", 1, "How many times to run.")
	flag.Bool("ro", false, "Only read the counter value, don't update it.")
	flag.Bool("be", false, "Read counter value without retrieving timestamp from Zero.")
	flag.Duration("wait", 0*time.Second, "How long to wait.")
	flag.String("pred", "counter.val", "Predicate to use for storing the counter.")
}

type Counter struct {
	Uid string `json:"uid"`
	Val int    `json:"val"`

	startTs uint64 // Only used for internal testing.
}

func queryCounter(txn *dgo.Txn, pred string) (Counter, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var counter Counter
	query := fmt.Sprintf("{ q(func: has(%s)) { uid, val: %s }}", pred, pred)
	resp, err := txn.Query(ctx, query)
	if err != nil {
		return counter, fmt.Errorf("Query error: %v", err)
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
	if len(counter.Uid) == 0 {
		counter.Uid = "_:new"
	}
	mu.SetNquads = []byte(fmt.Sprintf(`<%s> <%s> "%d"^^<xs:int> .`, counter.Uid, pred, counter.Val))

	// Don't put any timeout for mutation.
	_, err = txn.Mutate(context.Background(), &mu)
	if err != nil {
		return Counter{}, err
	}
	return counter, txn.Commit(context.Background())
}

func run(conf *viper.Viper) {
	addr := conf.GetString("addr")
	waitDur := conf.GetDuration("wait")
	num := conf.GetInt("num")
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)
	dg := dgo.NewDgraphClient(dc)

	for num > 0 {
		cnt, err := process(dg, conf)
		now := time.Now().UTC().Format("0102 03:04:05.999")
		if err != nil {
			fmt.Printf("%-17s While trying to process counter: %v. Retrying...\n", now, err)
			time.Sleep(time.Second)
			continue
		}
		fmt.Printf("%-17s Counter VAL: %d   [ Ts: %d ]\n", now, cnt.Val, cnt.startTs)
		num--
		time.Sleep(waitDur)
	}
}
