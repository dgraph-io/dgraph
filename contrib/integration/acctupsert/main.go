/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/x"
	"github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/z"
)

var (
	alpha   = flag.String("alpha", "localhost:9180", "dgraph alpha address")
	concurr = flag.Int("c", 3, "number of concurrent upserts per account")
)

var (
	firsts = []string{"Paul", "Eric", "Jack", "John", "Martin"}
	lasts  = []string{"Brown", "Smith", "Robinson", "Waters", "Taylor"}
	ages   = []int{20, 25, 30, 35}
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
	c := z.DgraphClientWithGroot(*alpha)
	setup(c)
	fmt.Println("Doing upserts")
	doUpserts(c)
	fmt.Println("Checking integrity")
	checkIntegrity(c)
}

func setup(c *dgo.Dgraph) {
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

func doUpserts(c *dgo.Dgraph) {
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

func upsert(c *dgo.Dgraph, acc account) {
	for {
		if time.Since(lastStatus) > 100*time.Millisecond {
			fmt.Printf("[%s] Success: %d Retries: %d\n", time.Now().Format(time.Stamp),
				atomic.LoadUint64(&successCount), atomic.LoadUint64(&retryCount))
			lastStatus = time.Now()
		}
		err := tryUpsert(c, acc)
		if err == nil {
			atomic.AddUint64(&successCount, 1)
			return
		} else if err == y.ErrAborted {
			// pass
		} else {
			fmt.Printf("ERROR: %v", err)
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
				uid
				expand(_all_) {uid}
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
	t := rand.Intn(len(types))

	var uid string
	if len(decode.Get) == 1 {
		x.AssertTrue(decode.Get[0].Uid != nil)
		uid = *decode.Get[0].Uid
	} else {
		nqs := fmt.Sprintf(`
			_:acct <first> %q .
			_:acct <last>  %q .
			_:acct <age>   "%d"^^<xs:int> .
			_:acct <%s> "" .
		 `,
			acc.first, acc.last, acc.age, types[t],
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
