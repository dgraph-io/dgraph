package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

var (
	dgraAddr  = flag.String("d", "localhost:9081", "dgraph address")
	timeout   = flag.Int("timeout", 60, "query/mutation timeout")
	numSents  = flag.Int("sentences", 100, "number of sentences")
	numSwaps  = flag.Int("swaps", 1000, "number of swaps to attempt")
	concurr   = flag.Int("concurrency", 10, "number of concurrent swaps to run concurrently")
	invPerSec = flag.Int("inv", 10, "number of times to check invariants per second")
)

var (
	successCount uint64
	failCount    uint64
	invChecks    uint64
)

func main() {
	flag.Parse()

	sents := createSentences(*numSents)
	sort.Strings(sents)
	wordCount := make(map[string]int)
	for _, s := range sents {
		words := strings.Split(s, " ")
		for _, w := range words {
			wordCount[w] += 1
		}
	}
	type wc struct {
		word  string
		count int
	}
	var wcs []wc
	for w, c := range wordCount {
		wcs = append(wcs, wc{w, c})
	}
	sort.Slice(wcs, func(i, j int) bool {
		wi := wcs[i]
		wj := wcs[j]
		return wi.word < wj.word
	})
	for _, w := range wcs {
		fmt.Printf("%15s: %3d\n", w.word, w.count)
	}

	c := newClient()
	uids := setup(c, sents)

	// Check invariants before doing any mutations as a sanity check.
	x.Check(checkInvariants(c, uids, sents))

	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(*invPerSec))
		for range ticker.C {
			for {
				if err := checkInvariants(c, uids, sents); err == nil {
					break
				} else {
					fmt.Printf("Error while running inv: %v\n", err)
				}
			}
			atomic.AddUint64(&invChecks, 1)
		}
	}()

	done := make(chan struct{})
	go func() {
		pending := make(chan struct{}, *concurr)
		for i := 0; i < *numSwaps; i++ {
			pending <- struct{}{}
			go func() {
				swapSentences(c,
					uids[rand.Intn(len(uids))],
					uids[rand.Intn(len(uids))],
				)
				<-pending
			}()
		}
		for i := 0; i < *concurr; i++ {
			pending <- struct{}{}
		}
		close(done)
	}()

	for {
		select {
		case <-time.After(time.Second):
			fmt.Printf("Success:%d Fail:%d Check:%d\n",
				atomic.LoadUint64(&successCount),
				atomic.LoadUint64(&failCount),
				atomic.LoadUint64(&invChecks),
			)
		case <-done:
			// One final check for invariants.
			x.Check(checkInvariants(c, uids, sents))
			return
		}
	}

}

func createSentences(n int) []string {
	sents := make([]string, n)
	for i := range sents {
		sents[i] = nextWord()
	}

	// add trailing words -- some will be common between sentences
	same := 2
	for {
		var w string
		var count int
		for i := range sents {
			if i%same == 0 {
				w = nextWord()
				count++
			}
			sents[i] += " " + w
		}
		if count == 1 {
			// Every sentence got the same trailing word, no point going any further.  Sort the
			// words within each sentence.
			for i, one := range sents {
				splits := strings.Split(one, " ")
				sort.Strings(splits)
				sents[i] = strings.Join(splits, " ")
			}
			return sents
		}
		same *= 2
	}
}

func newClient() *client.Dgraph {
	d, err := grpc.Dial(*dgraAddr, grpc.WithInsecure())
	x.Check(err)
	return client.NewDgraphClient(
		api.NewDgraphClient(d),
	)
}

func setup(c *client.Dgraph, sentences []string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()
	x.Check(c.Alter(ctx, &api.Operation{
		DropAll: true,
	}))
	x.Check(c.Alter(ctx, &api.Operation{
		Schema: `sentence: string @index(term) .`,
	}))

	rdfs := ""
	for i, s := range sentences {
		rdfs += fmt.Sprintf("_:s%d <sentence> %q .\n", i, s)
	}
	txn := c.NewTxn()
	defer txn.Discard(ctx)
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		IgnoreIndexConflict: true,
		SetNquads:           []byte(rdfs),
	})
	x.Check(err)
	x.Check(txn.Commit(ctx))

	var uids []string
	for _, uid := range assigned.GetUids() {
		uids = append(uids, uid)
	}
	return uids
}

func swapSentences(c *client.Dgraph, node1, node2 string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	resp, err := txn.Query(ctx, fmt.Sprintf(`
	{
		node1(func: uid(%s)) {
			sentence
		}
		node2(func: uid(%s)) {
			sentence
		}
	}
	`, node1, node2))
	x.Check(err)

	decode := struct {
		Node1 []struct {
			Sentence *string
		}
		Node2 []struct {
			Sentence *string
		}
	}{}
	json.Unmarshal(resp.GetJson(), &decode)
	x.AssertTrue(len(decode.Node1) == 1)
	x.AssertTrue(len(decode.Node2) == 1)
	x.AssertTrue(decode.Node1[0].Sentence != nil)
	x.AssertTrue(decode.Node2[0].Sentence != nil)

	// Delete sentences as an intermediate step.
	delRDFs := fmt.Sprintf(`
		<%s> <sentence> %q .
		<%s> <sentence> %q .
	`,
		node1, *decode.Node1[0].Sentence,
		node2, *decode.Node2[0].Sentence,
	)
	if _, err := txn.Mutate(ctx, &api.Mutation{
		IgnoreIndexConflict: true,
		DelNquads:           []byte(delRDFs),
	}); err != nil {
		atomic.AddUint64(&failCount, 1)
		return
	}

	// Add garbage data as an intermediate step.
	garbageRDFs := fmt.Sprintf(`
		<%s> <sentence> "...garbage..." .
		<%s> <sentence> "...garbage..." .
	`,
		node1, node2,
	)
	if _, err := txn.Mutate(ctx, &api.Mutation{
		IgnoreIndexConflict: true,
		SetNquads:           []byte(garbageRDFs),
	}); err != nil {
		atomic.AddUint64(&failCount, 1)
		return
	}

	// Perform swap.
	rdfs := fmt.Sprintf(`
		<%s> <sentence> %q .
		<%s> <sentence> %q .
	`,
		node1, *decode.Node2[0].Sentence,
		node2, *decode.Node1[0].Sentence,
	)
	if _, err := txn.Mutate(ctx, &api.Mutation{
		IgnoreIndexConflict: true,
		SetNquads:           []byte(rdfs),
	}); err != nil {
		atomic.AddUint64(&failCount, 1)
		return
	}
	if err := txn.Commit(ctx); err != nil {
		atomic.AddUint64(&failCount, 1)
		return
	}
	atomic.AddUint64(&successCount, 1)
}

func checkInvariants(c *client.Dgraph, uids []string, sentences []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// Get the sentence for each node. Then build (in memory) a term index.
	// Then we can query dgraph for each term, and make sure the posting list
	// is the same.

	txn := c.NewTxn()
	uidList := strings.Join(uids, ",")
	resp, err := txn.Query(ctx, fmt.Sprintf(`
	{
		q(func: uid(%s)) {
			sentence
			uid
		}
	}
	`, uidList))
	if err != nil {
		return err
	}
	decode := struct {
		Q []struct {
			Sentence *string
			Uid      *string
		}
	}{}
	x.Check(json.Unmarshal(resp.GetJson(), &decode))
	x.AssertTrue(len(decode.Q) == len(sentences))

	index := map[string][]string{} // term to uid list
	var gotSentences []string
	for _, node := range decode.Q {
		x.AssertTrue(node.Sentence != nil)
		x.AssertTrue(node.Uid != nil)
		gotSentences = append(gotSentences, *node.Sentence)
		for _, word := range strings.Split(*node.Sentence, " ") {
			index[word] = append(index[word], *node.Uid)
		}
	}
	sort.Strings(gotSentences)
	for i := 0; i < len(sentences); i++ {
		if sentences[i] != gotSentences[i] {
			fmt.Printf("Sentence doesn't match. Wanted: %q. Got: %q\n", sentences[i], gotSentences[i])
			fmt.Printf("All sentences: %v\n", sentences)
			fmt.Printf("Got sentences: %v\n", gotSentences)
			x.AssertTrue(false)
		}
	}

	for word, uids := range index {
		q := fmt.Sprintf(`
		{
			q(func: anyofterms(sentence, %q)) {
				uid
			}
		}
		`, word)

		resp, err := txn.Query(ctx, q)
		if err != nil {
			return err
		}
		decode := struct {
			Q []struct {
				Uid *string
			}
		}{}
		x.Check(json.Unmarshal(resp.GetJson(), &decode))
		var gotUids []string
		for _, node := range decode.Q {
			x.AssertTrue(node.Uid != nil)
			gotUids = append(gotUids, *node.Uid)
		}

		sort.Strings(gotUids)
		sort.Strings(uids)
		if !reflect.DeepEqual(gotUids, uids) {
			panic(fmt.Sprintf(`query: %s\n
			Uids in index for %q didn't match
			calculated: %v. Len: %d
				got:        %v
			`, q, word, uids, len(uids), gotUids))
		}
	}
	return nil
}
