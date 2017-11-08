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
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

var (
	dgraAddr = flag.String("d", "localhost:9080", "dgraph address")
	concurr  = flag.Int("c", 5, "number of concurrent upserts per account")
)

var (
	firsts = []string{"Paul", "Eric", "Jack", "John", "Martin"}
	lasts  = []string{"Brown", "Smith", "Robinson", "Waters", "Taylor"}
	ages   = []int{20, 25, 30, 35}
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
	doUpserts(c)
	checkIntegrity(c)
}

func newClient() *client.Dgraph {
	d, err := grpc.Dial(*dgraAddr, grpc.WithInsecure())
	x.Check(err)
	return client.NewDgraphClient(
		protos.NewDgraphClient(d),
	)
}

func setup(c *client.Dgraph) {
	ctx := context.Background()
	x.Check(c.Alter(ctx, &protos.Operation{
		DropAll: true,
	}))
	x.Check(c.Alter(ctx, &protos.Operation{
		Schema: `
			first:  string   @index(term) .
			last:   string   @index(hash) .
			age:    int      @index(int)  .
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
				upsert(c, acct)
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
		if !strings.Contains(strings.ToLower(err.Error()), "aborted") {
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
				uid: _uid_
			}
		}
	`, acc.first, acc.last, acc.age)
	resp, err := txn.Query(ctx, q)
	x.Check(err)

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
	} else {
		nqs := fmt.Sprintf(`
			_:acct <first> %q .
			_:acct <last>  %q .
			_:acct <age>   "%d"^^<xs:int> .
		`,
			acc.first, acc.last, acc.age,
		)
		mu := &protos.Mutation{SetNquads: []byte(nqs)}
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
	mu := &protos.Mutation{SetNquads: []byte(nq)}
	if _, err = txn.Mutate(ctx, mu); err != nil {
		return err
	}

	return txn.Commit(ctx)
}

func checkIntegrity(c *client.Dgraph) {
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
