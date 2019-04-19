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
	"io/ioutil"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/spf13/cobra"
)

type integrationOptions struct {
	addr            string
	text            string
	numMutations    uint64
	mutationsPerTxn uint64
	verbose         bool
}

type internalOptions struct {
	times              int
	size               int
	mutationsPerRollup int
}

var (
	integrationOpts integrationOptions
	internalOpts    internalOptions

	rootCmd = &cobra.Command{
		Use: "posting-list-benchmark",
		Run: func(cmd *cobra.Command, args []string) {
			// Do Stuff Here
		},
	}

	integrationCmd = &cobra.Command{
		Use:   "integration",
		Short: "Integration benchmark to measure posting list performance.",
		Long: `This benchmark adds a lot of triples with same object and predicate, creates a
full-text index on that predicate, and measures the performance of mutations and
transactions. The benchmark is intended to compare the performance of normal
posting lists against the performance of multi-part posting lists.`,
		Run: func(cmd *cobra.Command, args []string) {
			runIntegrationBenchmark()
		},
	}

	internalCmd = &cobra.Command{
		Use:   "internal",
		Short: "internal benchmark to measure posting list performance.",
		Long: `This benchmark creates posting lists, and measures the time to create them
and iterate through them, with and without splitting.`,
		Run: func(cmd *cobra.Command, args []string) {
			runInternalBenchmark()
		},
	}

	defaultText = "One morning, when Gregor Samsa woke from troubled dreams, he found himself " +
		"transformed in his bed into a horrible vermin. He lay on his armour-like back, and if " +
		"he lifted his head a little he could see his brown belly, slightly domed and divided by " +
		"arches into stiff sections. The bedding was hardly able to cover it and seemed ready to " +
		"slide off any moment."
	ps *badger.DB
)

func initCmd() {
	integrationFlags := integrationCmd.Flags()
	integrationFlags.StringVarP(&integrationOpts.addr, "addr", "a", "localhost:9180",
		"Address of the Dgraph alpha to which updates will be sent.")
	integrationFlags.StringVarP(&integrationOpts.text, "text", "t", defaultText,
		"The English text to insert in each triple.")
	integrationFlags.Uint64VarP(&integrationOpts.numMutations, "num_mutations", "", uint64(1e6),
		"The total number of mutations that will be sent to the alpha.")
	integrationFlags.Uint64VarP(&integrationOpts.mutationsPerTxn, "mutations_per_txn", "",
		uint64(1e3),
		"The total number of mutations that will be sent in each transaction.")
	integrationFlags.BoolVarP(&integrationOpts.verbose, "verbose", "v", false,
		"Toggle verbose output.")

	internalFlags := internalCmd.Flags()
	internalFlags.IntVarP(&internalOpts.times, "times", "t", 1,
		"Number of times to run each individual benchmark.")
	internalFlags.IntVarP(&internalOpts.size, "size", "s", int(1e6),
		"Size of the posting lists created by each benchmark.")
	internalFlags.IntVarP(&internalOpts.mutationsPerRollup, "mutations_per_rollup", "", 5000,
		"Number of mutations in between each list rollup.")

	rootCmd.AddCommand(integrationCmd)
	rootCmd.AddCommand(internalCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
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

func runIntegrationBenchmark() {
	// Check the value of the flags are sane.
	if integrationOpts.mutationsPerTxn > integrationOpts.numMutations {
		log.Fatalf("Mutations per txn (%v) cannot be greater than the number of mutations (%v)",
			integrationOpts.mutationsPerTxn, integrationOpts.numMutations)
	}

	dg := z.DgraphClientWithGroot(integrationOpts.addr)
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
	for i := uint64(0); i < integrationOpts.numMutations; i++ {
		uid := fmt.Sprintf("_:uid%d", i)
		triple := fmt.Sprintf("%s <text> \"%s\"@en .", uid, integrationOpts.text)
		triples = append(triples, triple)

		if i > 0 && i%integrationOpts.mutationsPerTxn == 0 {
			commitTriples(dg, triples)
			triples = nil
		}
	}

	// Commit last transaction in case it has not still been done.
	commitTriples(dg, triples)
}

func initInternalBenchmarks() {
	dir, err := ioutil.TempDir("/data/tmp", "plist_benchmark_")
	x.Check(err)

	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir
	ps, err = badger.OpenManaged(opt)
	x.Check(err)
	posting.Init(ps)
	schema.Init(ps)
}

func writePostingList(kvs []*bpb.KV) error {
	writer := posting.NewTxnWriter(ps)
	for _, kv := range kvs {
		if err := writer.SetAt(kv.Key, kv.Value, kv.UserMeta[0], kv.Version); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func createPostingList(key []byte, edges []*pb.DirectedEdge) *posting.List {
	list, err := posting.GetNew(key, ps)
	x.Check(err)

	for i, edge := range edges {
		txn := posting.Txn{StartTs: uint64(i)}
		err := list.AddMutation(context.Background(), &txn, edge)
		x.Check(err)
		list.CommitMutation(uint64(i), uint64(i)+1)

		if (i+1)%internalOpts.mutationsPerRollup == 0 {
			kvs, err := list.Rollup()
			x.Check(err)
			writePostingList(kvs)
			list, err = posting.GetNew(key, ps)
			x.Check(err)
		}
	}

	kvs, err := list.Rollup()
	x.Check(err)
	writePostingList(kvs)
	list, err = posting.GetNew(key, ps)
	x.Check(err)

	return list
}

func generateEdges(num int, label string) []*pb.DirectedEdge {
	var edges []*pb.DirectedEdge
	for i := 1; i <= num; i++ {
		edges = append(edges, &pb.DirectedEdge{
			ValueId: uint64(i),
			Op:      pb.DirectedEdge_SET,
			Label:   label,
		})
	}
	return edges
}

func createBasicPlistNoPostings(benchmarkId int) time.Duration {
	posting.MaxListSize = math.MaxInt32

	edges := generateEdges(internalOpts.size, "")
	key := x.DataKey("create_basic", uint64(benchmarkId))

	start := time.Now()
	createPostingList(key, edges)
	return time.Since(start)
}

func createBasicPlist(benchmarkId int) time.Duration {
	posting.MaxListSize = math.MaxInt32

	edges := generateEdges(internalOpts.size, "benchmark")
	key := x.DataKey("create_basic_posting", uint64(benchmarkId))

	start := time.Now()
	createPostingList(key, edges)
	return time.Since(start)
}

func iterateBasicPlist(benchmarkId int) time.Duration {
	posting.MaxListSize = math.MaxInt32

	edges := generateEdges(internalOpts.size, "benchmark")
	key := x.DataKey("iterate_basic", uint64(benchmarkId))

	list := createPostingList(key, edges)

	start := time.Now()
	list.Iterate(math.MaxUint64, 0, func(posting *pb.Posting) error {
		return nil
	})
	return time.Since(start)
}

func createMultiPartPlistNoPostings(benchmarkId int) time.Duration {
	posting.MaxListSize = posting.MB / 2

	edges := generateEdges(internalOpts.size, "")
	key := x.DataKey("create_multi_part", uint64(benchmarkId))

	start := time.Now()
	createPostingList(key, edges)
	return time.Since(start)
}

func createMultiPartPlist(benchmarkId int) time.Duration {
	posting.MaxListSize = posting.MB / 2

	edges := generateEdges(internalOpts.size, "benchmark")
	key := x.DataKey("create_multi_part_posting", uint64(benchmarkId))

	start := time.Now()
	createPostingList(key, edges)
	return time.Since(start)
}

func iterateMultiPartPlist(benchmarkId int) time.Duration {
	posting.MaxListSize = posting.MB / 2

	edges := generateEdges(internalOpts.size, "benchmark")
	key := x.DataKey("iterate_multi_part", uint64(benchmarkId))

	list := createPostingList(key, edges)

	start := time.Now()
	list.Iterate(math.MaxUint64, 0, func(posting *pb.Posting) error {
		return nil
	})
	return time.Since(start)
}

func runBenchmark(name string, benchmark func(int) time.Duration) {
	var runtimes []time.Duration
	for i := 0; i < internalOpts.times; i++ {
		runtimes = append(runtimes, benchmark(i))
	}
	outputResults(name, runtimes)
}

func outputResults(name string, runtimes []time.Duration) {
	var totalRuntime float64
	for _, runtime := range runtimes {
		totalRuntime += runtime.Seconds()
	}
	avgRuntime := totalRuntime / float64(len(runtimes))

	fmt.Printf("Ran benchmark: %s. Avg runtime: %.3f seconds\n", name, avgRuntime)
}

func runInternalBenchmark() {
	initInternalBenchmarks()

	runBenchmark("Create non-split list", createBasicPlistNoPostings)
	runBenchmark("Create non-split list with postings", createBasicPlist)
	runBenchmark("Iterate over non-split list", iterateBasicPlist)
	runBenchmark("Create split list", createMultiPartPlistNoPostings)
	runBenchmark("Create split list with postings", createMultiPartPlist)
	runBenchmark("Iterate over split list", iterateMultiPartPlist)
}

func main() {
	initCmd()
}
