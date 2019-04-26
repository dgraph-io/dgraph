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

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/spf13/cobra"
)

type flagOptions struct {
	addr            string
	text            string
	numMutations    uint64
	mutationsPerTxn uint64
	verbose         bool
}

var (
	opt     flagOptions
	rootCmd = &cobra.Command{
		Use:   "posting-list-benchmark",
		Short: "Benchmark to measure posting list performance.",
		Long: `This benchmark adds a lot of triples with same object and predicate, creates a
full-text index on that predicate, and measures the performance of mutations and
transactions. The benchmark is intended to compare the performance of normal
posting lists against the performance of multi-part posting lists.`,
		Run: func(cmd *cobra.Command, args []string) {
			runBenchmark()
		},
	}
	defaultText = "One morning, when Gregor Samsa woke from troubled dreams, he found himself " +
		"transformed in his bed into a horrible vermin. He lay on his armour-like back, and if " +
		"he lifted his head a little he could see his brown belly, slightly domed and divided by " +
		"arches into stiff sections. The bedding was hardly able to cover it and seemed ready to " +
		"slide off any moment."
)

func initCmd() {
	flags := rootCmd.Flags()
	flags.StringVarP(&opt.addr, "addr", "a", "localhost:9180",
		"Address of the Dgraph alpha to which updates will be sent.")
	flags.StringVarP(&opt.text, "text", "t", defaultText,
		"The English text to insert in each triple.")
	flags.Uint64VarP(&opt.numMutations, "num_mutations", "", uint64(1e6),
		"The total number of mutations that will be sent to the alpha.")
	flags.Uint64VarP(&opt.mutationsPerTxn, "mutations_per_txn", "", uint64(1e3),
		"The total number of mutations that will be sent in each transaction.")
	flags.BoolVarP(&opt.verbose, "verbose", "v", false, "Toggle verbose output.")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runBenchmark() {
	// Check the value of the flags are sane.
	if opt.mutationsPerTxn > opt.numMutations {
		log.Fatalf("Mutations per txn (%v) cannot be greater than the number of mutations (%v)",
			opt.mutationsPerTxn, opt.numMutations)
	}

	dg := z.DgraphClientWithGroot(opt.addr)
	var err error

	// Drop all existing data.
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}
	x.Check(err)

	// Initialize schema.
	err = dg.Alter(context.Background(), &api.Operation{
		Schema: "text: string @index(fulltext) @lang .",
	})
	x.Check(err)

	var triples []string
	for i := uint64(0); i < opt.numMutations; i++ {
		uid := fmt.Sprintf("_:uid%d", i)
		triple := fmt.Sprintf("%s <text> \"%s\"@en .", uid, opt.text)
		triples = append(triples, triple)

		if i > 0 && i%opt.mutationsPerTxn == 0 {
			commitTriples(dg, triples)
			triples = nil
		}
	}

	// Commit last transaction in case it has not still been done.
	commitTriples(dg, triples)
}

func commitTriples(dg *dgo.Dgraph, triples []string) {
	for {
		txn := dg.NewTxn()
		mu := api.Mutation{
			SetNquads: []byte(strings.Join(triples, "\n")),
			CommitNow: true,
		}
		_, err := txn.Mutate(context.Background(), &mu)

		if err == nil {
			break
		}

		// Retry in case the transaction has been aborted.
		if strings.Contains(err.Error(), "Transaction has been aborted") {
			continue
		}

		x.Check(err)
	}
}

func main() {
	initCmd()
}
