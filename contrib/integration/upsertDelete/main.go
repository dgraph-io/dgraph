package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/y"
	"google.golang.org/grpc"
)

var (
	dgraAddr = flag.String("d", "localhost:9081", "dgraph address")
	concurr  = flag.Int("c", 5, "number of concurrent upserts per account")
)

var (
	firsts = []string{"Paul"}
	lasts  = []string{"Brown"}
	ages   = []int{20, 25}
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
			for _, age := range ages {
				accounts = append(accounts, account{
					first: first,
					last:  last,
					age:   age,
				})
			}
		}
	}
}

func main() {
	flag.Parse()
	c := newClient()
	setup(c)
	go func() {
		for {
			checkIntegrity(c)
		}
	}()
	doUpserts(c)
	checkIntegrity(c)
}

func newClient() *client.Dgraph {
	d, err := grpc.Dial(*dgraAddr, grpc.WithInsecure())
	x.Check(err)
	return client.NewDgraphClient(
		api.NewDgraphClient(d),
	)
}

func setup(c *client.Dgraph) {
	ctx := context.Background()
	x.Check(c.Alter(ctx, &api.Operation{
		DropAll: true,
	}))
	x.Check(c.Alter(ctx, &api.Operation{
		Schema: `
			first:  string   @index(term) @upsert .
			last:   string   @index(hash) @upsert .
			age:    int      @index(int)  @upsert .
			when:   int                   .
		`,
	}))
}

func doUpserts(c *client.Dgraph) {
	var wg sync.WaitGroup
	wg.Add(len(accounts) * *concurr)
	for _, acct := range accounts {
		for i := 0; i < *concurr; i++ {
			go func(acct account) {
				for i := 0; i < 50; i++ {
					upsert(c, acct)
				}
				wg.Done()
			}(acct)
		}
	}
	wg.Wait()
}

var (
	successCount uint64
	retryCount   uint64
	lastStatus   time.Time
)

func upsert(c *client.Dgraph, acc account) {
	for {
		if time.Since(lastStatus) > 100*time.Millisecond {
			fmt.Printf("Success: %d Retries: %d\n",
				atomic.LoadUint64(&successCount), atomic.LoadUint64(&retryCount))
			lastStatus = time.Now()
		}
		err := tryUpsert(c, acc)
		if err == nil {
			atomic.AddUint64(&successCount, 1)
			return
		}
		if err != y.ErrAborted {
			x.Check(err)
		}
		atomic.AddUint64(&retryCount, 1)
	}
}

func tryUpsert(c *client.Dgraph, acc account) error {
	ctx := context.Background()

	txn := c.NewTxn()
	defer txn.Discard(ctx)
	q := fmt.Sprintf(`
		{
			get(func: eq(first, %q)) @filter(eq(last, %q) AND eq(age, %d)) {
				uid
			}
		}
	`, acc.first, acc.last, acc.age)
	resp, err := txn.Query(ctx, q)
	if err != nil {
		return nil
	}

	decode := struct {
		Get []struct {
			Uid *string
		}
	}{}
	x.Check(json.Unmarshal(resp.GetJson(), &decode))

	x.AssertTrue(len(decode.Get) <= 1)
	var uid string
	if len(decode.Get) == 1 {
		x.AssertTrue(decode.Get[0].Uid != nil)
		uid = *decode.Get[0].Uid
		nqs := fmt.Sprintf(`
			<%s> <first> * .
			<%s> <last>  * .
			<%s> <age>   * .
		`,
			uid, uid, uid,
		)
		mu := &api.Mutation{DelNquads: []byte(nqs)}
		_, err := txn.Mutate(ctx, mu)
		if err != nil {
			return err
		}
		return txn.Commit(ctx)
	} else {
		nqs := fmt.Sprintf(`
			_:acct <first> %q .
			_:acct <last>  %q .
			_:acct <age>   "%d"^^<xs:int> .
		`,
			acc.first, acc.last, acc.age,
		)
		mu := &api.Mutation{SetNquads: []byte(nqs)}
		assigned, err := txn.Mutate(ctx, mu)
		if err != nil {
			return err
		}
		uid = assigned.GetUids()["acct"]
		x.AssertTrue(uid != "")

		return txn.Commit(ctx)
	}

}

func checkIntegrity(c *client.Dgraph) {
	ctx := context.Background()

	q := fmt.Sprintf(`
		{
			all(func: anyofterms(first, %q)) {
				uid
				first
				last
				age
			}
		}
	`, strings.Join(firsts, " "))
	txn := c.NewTxn()
	resp, err := txn.Query(ctx, q)
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
		x.AssertTruef(record.First != nil, "\n%q\n", resp.GetJson())
		x.AssertTruef(record.Last != nil, "\n%q\n", resp.GetJson())
		x.AssertTruef(record.Age != nil, "\n%q\n", resp.GetJson())
		str := fmt.Sprintf("%s_%s_%d", *record.First, *record.Last, *record.Age)
		_, ok := accountSet[str]
		x.AssertTrue(!ok)
		accountSet[str] = struct{}{}
	}
}
