package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

type Person struct {
	Name  string `json:"name,omitempty"`
	email string
	count int
}

type Data struct {
	Name   string `json:"name,omitempty"`
	Counts []int  `json:"count,omitempty"`
}
type ResponseData struct {
	All []Data `json:"all,omitempty"`
}

func InitData(t *testing.T) {
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	testutil.DropAll(t, dg)
	require.NoError(t, err)
	schema := `
		name: string @index(exact) .
		email: string @index(exact) .
		count: [int]  .
	`

	err = dg.Alter(context.Background(), &api.Operation{Schema: schema})
	require.NoError(t, err)

	p := Person{
		Name:  "Alice",
		email: "alice@dgraph.io",
		count: 1,
	}
	pb, err := json.Marshal(p)
	require.NoError(t, err)

	mu := &api.Mutation{
		SetJson:   pb,
		CommitNow: true,
	}
	txn := dg.NewTxn()
	ctx := context.Background()
	defer txn.Discard(ctx)

	_, err = txn.Mutate(ctx, mu)
	require.NoError(t, err)
}

func TestConcurrentUpdate(t *testing.T) {
	InitData(t)
	ctx := context.Background()
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	count := 10
	var wg sync.WaitGroup
	wg.Add(count)
	mutation := func(i int) {
		defer wg.Done()
		query := fmt.Sprintf(`query {
			user as var(func: eq(name, "Alice"))
			}`)
		mu := &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`uid(user) <count> "%d" .`, i)),
		}
		req := &api.Request{
			Query:     query,
			Mutations: []*api.Mutation{mu},
			CommitNow: true,
		}
		_, err := dg.NewTxn().Do(ctx, req)
		require.NoError(t, err)
	}
	for i := 0; i < count; i++ {
		go mutation(i)
	}
	wg.Wait()

	q := `query all($a: string) {
			all(func: eq(name, $a)) {
			  name
			  count
			}
		  }`

	txn := dg.NewTxn()
	res, err := txn.QueryWithVars(ctx, q, map[string]string{"$a": "Alice"})
	require.NoError(t, err)
	var dat ResponseData
	err = json.Unmarshal(res.Json, &dat)
	require.NoError(t, err)

	require.Equal(t, len(dat.All[0].Counts), 10)
}

func TestSequentialUpdate(t *testing.T) {
	t.Log("TestSequentialUpdate")
	InitData(t)
	ctx := context.Background()
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	count := 10
	mutation := func(i int) {
		query := fmt.Sprintf(`query {
			user as var(func: eq(name, "Alice"))
			}`)
		mu := &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`uid(user) <count> "%d" .`, i)),
		}
		req := &api.Request{
			Query:     query,
			Mutations: []*api.Mutation{mu},
			CommitNow: true,
		}
		_, err := dg.NewTxn().Do(ctx, req)
		require.NoError(t, err)

	}
	for i := 0; i < count; i++ {
		mutation(i)
	}

	q := `query all($a: string) {
			all(func: eq(name, $a)) {
			  name
			  count
			}
		  }`

	txn := dg.NewTxn()
	res, err := txn.QueryWithVars(ctx, q, map[string]string{"$a": "Alice"})
	require.NoError(t, err)
	var dat ResponseData
	err = json.Unmarshal(res.Json, &dat)
	require.NoError(t, err)

	require.Equal(t, len(dat.All[0].Counts), 10)
}
