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
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

var (
	dgraAddr  = flag.String("d", "localhost:9080", "dgraph address")
	zeroAddr  = flag.String("z", "localhost:8888", "zero address")
	timeout   = flag.Int("timeout", 5, "query/mutation timeout")
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
	for i, s := range sents {
		fmt.Printf("%d: %s\n", i, s)
	}
	sort.Strings(sents)

	c := newClient()
	uids := setup(c, sents)
	for _, uid := range uids {
		fmt.Printf("%#x\n", uid)
	}

	// Check invariants before doing any mutations as a sanity check.
	checkInvariants(c, uids, sents)

	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(*invPerSec))
		for {
			select {
			case <-ticker.C:
			}
			checkInvariants(c, uids, sents)
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
			checkInvariants(c, uids, sents)
			return
		}
	}

}

type sentence struct {
	first string
	whole string
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
	z, err := grpc.Dial(*zeroAddr, grpc.WithInsecure())
	x.Check(err)
	d, err := grpc.Dial(*dgraAddr, grpc.WithInsecure())
	x.Check(err)
	return client.NewDgraphClient(
		protos.NewZeroClient(z),
		protos.NewDgraphClient(d),
	)
}

func setup(c *client.Dgraph, sentences []string) []uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()
	x.Check(c.Alter(ctx, &protos.Operation{
		DropAll: true,
	}))
	x.Check(c.Alter(ctx, &protos.Operation{
		Schema: `sentence: string @index(term) .`,
	}))

	rdfs := ""
	for i, s := range sentences {
		rdfs += fmt.Sprintf("_:s%d <sentence> %q .\n", i, s)
	}
	txn := c.NewTxn()
	assigned, err := txn.Mutate(ctx, &protos.Mutation{SetNquads: []byte(rdfs)})
	x.Check(err)
	x.Check(txn.Commit(ctx))

	var uids []uint64
	for _, uid := range assigned.GetUids() {
		uids = append(uids, uid)
	}
	return uids
}

func swapSentences(c *client.Dgraph, node1, node2 uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// TODO: We could do some intermediate deletes and put in some intermediate
	// garbage values for good measure.

	txn := c.NewTxn()
	// TODO: Use query variables...
	resp, err := txn.Query(ctx, fmt.Sprintf(`
	{
		node1(func: uid(%d)) {
			sentence
		}
		node2(func: uid(%d)) {
			sentence
		}
	}
	`, node1, node2), nil)
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

	rdfs := fmt.Sprintf(`
		<%#x> <sentence> %q .
		<%#x> <sentence> %q .
	`,
		node1, *decode.Node2[0].Sentence,
		node2, *decode.Node1[0].Sentence,
	)
	if _, err := txn.Mutate(ctx, &protos.Mutation{SetNquads: []byte(rdfs)}); err != nil {
		atomic.AddUint64(&failCount, 1)
	}
	if err := txn.Commit(ctx); err != nil {
		atomic.AddUint64(&failCount, 1)
	}
	atomic.AddUint64(&successCount, 1)
}

func checkInvariants(c *client.Dgraph, uids []uint64, sentences []string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// Get the sentence for each node. Then build (in memory) a term index.
	// Then we can query dgraph for each term, and make sure the posting list
	// is the same.

	txn := c.NewTxn()
	uidList := ""
	for i, uid := range uids {
		if i != 0 {
			uidList += ","
		}
		uidList += fmt.Sprintf("%#x", uid)
	}
	resp, err := txn.Query(ctx, fmt.Sprintf(`
	{
		q(func: uid(%s)) {
			sentence
			uid: _uid_
		}
	}
	`, uidList), nil)
	x.Check(err)
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
	x.AssertTruef(reflect.DeepEqual(gotSentences, sentences), "sentences didn't match")

	for word, uids := range index {
		resp, err := txn.Query(ctx, fmt.Sprintf(`
		{
			q(func: anyofterms(sentence, %q)) {
				uid: _uid_
			}
		}
		`, word), nil)
		x.Check(err)
		decode := struct {
			Q []struct {
				Uid *string
			}
		}{}
		x.Check(json.Unmarshal(resp.GetJson(), &decode))
		x.AssertTrue(len(decode.Q) > 0)
		var gotUids []string
		for _, node := range decode.Q {
			x.AssertTrue(node.Uid != nil)
			gotUids = append(gotUids, *node.Uid)
		}

		sort.Strings(gotUids)
		sort.Strings(uids)
		if !reflect.DeepEqual(gotUids, uids) {
			panic(fmt.Sprintf(`uids in index for %q didn't match
				calculated: %v
				got:        %v
			`, word, uids, gotUids))
		}
	}
}
