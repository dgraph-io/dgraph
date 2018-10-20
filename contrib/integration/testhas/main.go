package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/x"
	"github.com/dgraph-io/dgo/y"
	"google.golang.org/grpc"
)

var (
	addr    = flag.String("addr", "localhost:9080", "dgraph address")
	concurr = flag.Int("c", 3, "number of concurrent upserts per account")
)

var (
	firsts = []string{"Paul", "Eric", "Jack", "John", "Martin"}
	lasts  = []string{"Brown", "Smith", "Robinson", "Waters", "Taylor"}
	types  = []string{"CEO", "COO", "CTO", "CFO"}
)

type account struct {
	first string
	last  string
	age   int
}

var accounts []account

func init() {
	for _, first := range firsts {
		for _, last := range lasts {
			for i := 0; i < 1000; i++ {
				accounts = append(accounts, account{
					first: first,
					last:  last,
					age:   i,
				})
			}
		}
	}
}

func main() {
	flag.Parse()
	c := newClient()
	setup(c)
	fmt.Println("Doing upserts")
	doUpserts(c)
	fmt.Println("Checking integrity")
	checkIntegrity(c)
}

func newClient() *dgo.Dgraph {
	d, err := grpc.Dial(*addr, grpc.WithInsecure())
	x.Check(err)
	return dgo.NewDgraphClient(
		api.NewDgraphClient(d),
	)
}

func setup(c *dgo.Dgraph) {
	ctx := context.Background()
	x.Check(c.Alter(ctx, &api.Operation{
		DropAll: true,
	}))
	x.Check(c.Alter(ctx, &api.Operation{
		Schema: `
			first:  string   @index(term) .
			last:   string   @index(hash) .
			age:    int      @index(int)  .
			when:   int                   .
		`,
	}))
}

func doUpserts(c *dgo.Dgraph) {
	var wg sync.WaitGroup
	inputCh := make(chan account, 1000)
	go func() {
		for _, acct := range accounts {
			inputCh <- acct
		}
		close(inputCh)
	}()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			for acct := range inputCh {
				upsert(c, acct)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

var (
	successCount uint64
	retryCount   uint64
	totalCount   uint64
)

func upsert(c *dgo.Dgraph, acc account) {
	for {
		if atomic.AddUint64(&totalCount, 1)%100 == 0 {
			fmt.Printf("[%s] Success: %d Retries: %d Account: %v\n", time.Now().Format(time.Stamp),
				atomic.LoadUint64(&successCount), atomic.LoadUint64(&retryCount), acc)
		}
		err := tryUpsert(c, acc)
		if err == nil {
			atomic.AddUint64(&successCount, 1)
			return
		} else if err == y.ErrAborted {
			// pass
		} else {
			fmt.Printf("Error: %v\n", err)
		}
		atomic.AddUint64(&retryCount, 1)
	}
}

func tryUpsert(c *dgo.Dgraph, acc account) error {
	ctx := context.Background()

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	q := fmt.Sprintf(`
		{
			get(func: eq(first, %q)) @filter(eq(last, %q) AND eq(age, %d)) {
				uid expand(_all_) {uid}
			}
		}
	`, acc.first, acc.last, acc.age)

retry:
	resp, err := txn.Query(ctx, q)
	if err != nil {
		log.Printf("Got error while querying: %v. Retrying...\n", err)
		goto retry
	}

	decode := struct {
		Get []struct {
			Uid *string
		}
	}{}
	x.Check(json.Unmarshal(resp.GetJson(), &decode))

	x.AssertTrue(len(decode.Get) <= 1)
	s := rand.NewSource(time.Now().Unix())
	r := rand.New(s) // initialize local pseudorandom generator
	t := r.Intn(len(types))

	var uid string
	if len(decode.Get) == 1 {
		x.AssertTrue(decode.Get[0].Uid != nil)
		uid = *decode.Get[0].Uid
	} else {
		nqs := fmt.Sprintf(`
			_:acct <first> %q .
			_:acct <last>  %q .
			_:acct <age>   "%d"^^<xs:int> .
			_:acct <type> %q .
			_:acct <%s> "" .
			_:acct <human> "" .
		`,
			acc.first, acc.last, acc.age, types[t], types[t],
		)
		mu := &api.Mutation{SetNquads: []byte(nqs)}
		assigned, err := txn.Mutate(ctx, mu)
		if err != nil {
			return err
		}
		uid = assigned.GetUids()["acct"]
		x.AssertTrue(uid != "")
	}

	nq := fmt.Sprintf(`
		<%s> <when> "%d"^^<xs:int> .
	`,
		uid, time.Now().Nanosecond(),
	)
	mu := &api.Mutation{SetNquads: []byte(nq)}
	if _, err = txn.Mutate(ctx, mu); err != nil {
		return err
	}

	return txn.Commit(ctx)
}

func checkIntegrity(c *dgo.Dgraph) {
	ctx := context.Background()

	q := fmt.Sprintf(`
		{
			all(func: anyofterms(first, %q)) {
				first
				last
				age
			}
		}
	`, strings.Join(firsts, " "))
	resp, err := c.NewTxn().Query(ctx, q)
	x.Check(err)

	decode := struct {
		All []struct {
			First *string
			Last  *string
			Age   *int
		}
	}{}
	x.Check(json.Unmarshal(resp.GetJson(), &decode))

	// Make sure there is exactly one of each account.
	accountSet := make(map[string]struct{})
	for _, record := range decode.All {
		x.AssertTrue(record.First != nil)
		x.AssertTrue(record.Last != nil)
		x.AssertTrue(record.Age != nil)
		str := fmt.Sprintf("%s_%s_%d", *record.First, *record.Last, *record.Age)
		accountSet[str] = struct{}{}
	}
	x.AssertTrue(len(accountSet) == len(accounts))
	for _, acct := range accounts {
		str := fmt.Sprintf("%s_%s_%d", acct.first, acct.last, acct.age)
		_, ok := accountSet[str]
		x.AssertTrue(ok)
	}
}
