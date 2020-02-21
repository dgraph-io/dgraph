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

package main_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type state struct {
	dg *dgo.Dgraph
}

var s state

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	testutil.AssignUids(200)
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	x.CheckfNoTrace(err)
	s.dg = dg

	r := m.Run()
	os.Exit(r)
}

func runQueryWithRetry(ctx context.Context, txn *dgo.Txn, query string) (
	*api.Response, error) {

	for {
		response, err := txn.Query(ctx, query)
		if err != nil && strings.Contains(err.Error(), "is not indexed") {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		return response, err
	}
}

// readTs == startTs
func TestTxnRead1(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	txn := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	uid := retrieveUids(assigned.Uids)[0]
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := runQueryWithRetry(context.Background(), txn, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
	require.NoError(t, txn.Commit(context.Background()))
}

// readTs < commitTs
func TestTxnRead2(t *testing.T) {
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	txn2 := s.dg.NewTxn()

	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := runQueryWithRetry(context.Background(), txn2, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTruef(bytes.Equal(resp.Json, []byte("{\"me\":[]}")), "%s", resp.Json)
	require.NoError(t, txn.Commit(context.Background()))
}

// readTs > commitTs
func TestTxnRead3(t *testing.T) {
	op := &api.Operation{}
	op.DropAttr = "name"
	attempts := 0
	for attempts < 10 {
		if err := s.dg.Alter(context.Background(), op); err == nil {
			break
		}
		attempts++
	}

	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	txn = s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := runQueryWithRetry(context.Background(), txn, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
}

// readTs > commitTs
func TestTxnRead4(t *testing.T) {
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	txn2 := s.dg.NewTxn()

	txn3 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Manish2"}`, uid))
	_, err = txn3.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := runQueryWithRetry(context.Background(), txn2, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))

	require.NoError(t, txn3.Commit(context.Background()))

	txn4 := s.dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err = runQueryWithRetry(context.Background(), txn4, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish2\"}]}")))
}

func TestTxnRead5(t *testing.T) {
	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)

	txn = s.dg.NewReadOnlyTxn()
	resp, err := runQueryWithRetry(context.Background(), txn, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
	x.AssertTrue(resp.Txn.StartTs > 0)

	mu = &api.Mutation{CommitNow: true}
	mu.SetJson = []byte(fmt.Sprintf("{\"uid\": \"%s\", \"name\": \"Manish2\"}", uid))

	txn = s.dg.NewTxn()
	res, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	x.AssertTrue(res.Txn.StartTs > 0)
	txn = s.dg.NewReadOnlyTxn()
	resp, err = runQueryWithRetry(context.Background(), txn, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte(`{"me":[{"name":"Manish2"}]}`)))
}

func TestConflict(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	txn := s.dg.NewTxn()

	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Manish"}`, uid))
	x.Check2(txn2.Mutate(context.Background(), mu))

	require.NoError(t, txn.Commit(context.Background()))
	err = txn2.Commit(context.Background())
	x.AssertTrue(err != nil)

	txn = s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := runQueryWithRetry(context.Background(), txn, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
}

func TestConflictTimeout(t *testing.T) {
	var uid string
	txn := s.dg.NewTxn()
	{
		mu := &api.Mutation{}
		mu.SetJson = []byte(`{"name": "Manish"}`)
		assigned, err := txn.Mutate(context.Background(), mu)
		if err != nil {
			log.Fatalf("Error while running mutation: %v\n", err)
		}
		if len(assigned.Uids) != 1 {
			log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
		}
		for _, u := range assigned.Uids {
			uid = u
		}
	}

	txn2 := s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	_, err := runQueryWithRetry(context.Background(), txn2, q)
	require.NoError(t, err)

	mu := &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Jan the man"}`, uid))
	_, err = txn2.Mutate(context.Background(), mu)
	if err == nil {
		require.NoError(t, txn2.Commit(context.Background()))
	}

	err = txn.Commit(context.Background())
	x.AssertTrue(err != nil)

	txn3 := s.dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	_, err = runQueryWithRetry(context.Background(), txn3, q)
	require.NoError(t, err)
}

func TestConflictTimeout2(t *testing.T) {
	var uid string
	txn := s.dg.NewTxn()
	{

		mu := &api.Mutation{}
		mu.SetJson = []byte(`{"name": "Manish"}`)
		assigned, err := txn.Mutate(context.Background(), mu)
		if err != nil {
			log.Fatalf("Error while running mutation: %v\n", err)
		}
		if len(assigned.Uids) != 1 {
			log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
		}
		for _, u := range assigned.Uids {
			uid = u
		}
	}

	txn2 := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Jan the man"}`, uid))
	x.Check2(txn2.Mutate(context.Background(), mu))

	require.NoError(t, txn.Commit(context.Background()))
	err := txn2.Commit(context.Background())
	x.AssertTrue(err != nil)

	txn3 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s", "name": "Jan the man"}`, uid))
	assigned, err := txn3.Mutate(context.Background(), mu)
	if err == nil {
		require.NoError(t, txn3.Commit(context.Background()))
	}
	for _, u := range assigned.Uids {
		uid = u
	}

	txn4 := s.dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	_, err = runQueryWithRetry(context.Background(), txn4, q)
	require.NoError(t, err)
}

func TestIgnoreIndexConflict(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `name: string @index(exact) .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	txn := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid1, uid2 string
	for _, u := range assigned.Uids {
		uid1 = u
	}

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err = txn2.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	for _, u := range assigned.Uids {
		uid2 = u
	}

	require.NoError(t, txn.Commit(context.Background()))
	require.NoError(t, txn2.Commit(context.Background()))

	txn = s.dg.NewTxn()
	q := `{ me(func: eq(name, "Manish")) { uid }}`
	resp, err := runQueryWithRetry(context.Background(), txn, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	expectedResp := []byte(fmt.Sprintf(`{"me":[{"uid":"%s"},{"uid":"%s"}]}`, uid1, uid2))
	require.Equal(t, expectedResp, resp.Json)
}

func TestReadIndexKeySameTxn(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `name: string @index(exact) .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	txn := s.dg.NewTxn()

	mu := &api.Mutation{
		CommitNow: true,
		SetJson:   []byte(`{"name": "Manish"}`),
	}
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	txn = s.dg.NewTxn()
	defer txn.Discard(context.Background())
	q := `{ me(func: le(name, "Manish")) { uid }}`
	resp, err := runQueryWithRetry(context.Background(), txn, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	expectedResp := []byte(fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid))
	x.AssertTrue(bytes.Equal(resp.Json, expectedResp))
}

func TestEmailUpsert(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `email: string @index(exact) @upsert .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	txn1 := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "_:user1", "email": "email@email.org"}`)
	_, err := txn1.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "_:user2", "email": "email@email.org"}`)
	_, err = txn2.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	txn3 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "_:user3", "email": "email3@email.org"}`)
	_, err = txn3.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	require.NoError(t, txn1.Commit(context.Background()))
	require.NotNil(t, txn2.Commit(context.Background()))
	require.NoError(t, txn3.Commit(context.Background()))
}

// TestFriendList tests that we are not able to set a node to node edge between
// the same nodes concurrently.
func TestFriendList(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `
	friend: [uid] @reverse .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	txn1 := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "0x01", "friend": [{"uid": "0x02"}]}`)
	_, err := txn1.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "0x01", "friend": [{"uid": "0x02"}]}`)
	_, err = txn2.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	txn3 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "0x01", "friend": [{"uid": "0x03"}]}`)
	_, err = txn3.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	require.NoError(t, txn1.Commit(context.Background()))
	require.NotNil(t, txn2.Commit(context.Background()))
	require.NoError(t, txn3.Commit(context.Background()))
}

// TestNameSet tests that we are not able to set a property edge for the same
// subject id concurrently.
func TestNameSet(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `name: string .`
	if err := s.dg.Alter(context.Background(), op); err != nil {
		log.Fatal(err)
	}

	txn1 := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "0x01", "name": "manish"}`)
	_, err := txn1.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	txn2 := s.dg.NewTxn()
	mu = &api.Mutation{}
	mu.SetJson = []byte(`{"uid": "0x01", "name": "contributor"}`)
	_, err = txn2.Mutate(context.Background(), mu)
	assert.Nil(t, err)

	require.NoError(t, txn1.Commit(context.Background()))
	require.NotNil(t, txn2.Commit(context.Background()))
}

// retrieve the uids in the uidMap in the order of ascending keys
func retrieveUids(uidMap map[string]string) []string {
	keys := make([]string, 0, len(uidMap))
	for key := range uidMap {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		num1 := strings.Split(keys[i], ".")[2]

		num2 := strings.Split(keys[j], ".")[2]
		n1, err := strconv.Atoi(num1)
		x.Check(err)
		n2, err := strconv.Atoi(num2)
		x.Check(err)
		return n1 < n2
	})

	uids := make([]string, 0, len(uidMap))
	for _, k := range keys {
		uids = append(uids, uidMap[k])
	}
	return uids
}

func TestSPStar(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `friend: [uid] .`
	require.NoError(t, s.dg.Alter(context.Background(), op))

	txn := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish", "friend": [{"name": "Jan"}]}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	require.Equal(t, 2, len(assigned.Uids))
	uid1 := retrieveUids(assigned.Uids)[0]
	require.NoError(t, err)
	require.Equal(t, 2, len(assigned.Uids))
	require.NoError(t, txn.Commit(context.Background()))

	txn = s.dg.NewTxn()
	mu = &api.Mutation{}
	dgo.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan2"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))
	uid2 := retrieveUids(assigned.Uids)[0]

	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			friend {
				uid
				name
			}
		}
	}`, uid1)

	resp, err := runQueryWithRetry(context.Background(), txn, q)
	require.NoError(t, err)
	expectedResp := fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan2", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))
}

func TestSPStar2(t *testing.T) {
	op := &api.Operation{}
	op.DropAll = true
	require.NoError(t, s.dg.Alter(context.Background(), op))

	op = &api.Operation{}
	op.Schema = `friend: [uid] .`
	require.NoError(t, s.dg.Alter(context.Background(), op))

	// Add edge
	txn := s.dg.NewTxn()
	mu := &api.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish", "friend": [{"name": "Jan"}]}`)
	assigned, err := txn.Mutate(context.Background(), mu)

	require.NoError(t, err)
	require.Equal(t, 2, len(assigned.Uids))

	uids := retrieveUids(assigned.Uids)
	uid1 := uids[0]
	uid2 := uids[1]
	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			friend {
				uid
				name
			}
		}
	}`, uid1)

	resp, err := runQueryWithRetry(context.Background(), txn, q)
	require.NoError(t, err)
	expectedResp := fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Delete S P *
	mu = &api.Mutation{}
	dgo.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	resp, err = runQueryWithRetry(context.Background(), txn, q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid1)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Add edge
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan2"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))
	uid3 := retrieveUids(assigned.Uids)[0]
	resp, err = runQueryWithRetry(context.Background(), txn, q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan2", "uid":"%s"}]}]}`,
		uid1, uid3)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Delete S P *
	mu = &api.Mutation{}
	dgo.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	resp, err = runQueryWithRetry(context.Background(), txn, q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid1)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Add edge
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan3"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))

	uid4 := retrieveUids(assigned.Uids)[0]
	resp, err = runQueryWithRetry(context.Background(), txn, q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan3", "uid":"%s"}]}]}`, uid1, uid4)
	require.JSONEq(t, expectedResp, string(resp.Json))
}

var (
	ctxb       = context.Background()
	countQuery = `
query countAnswers($num: int) {
  me(func: eq(count(answer), $num)) {
    uid
    count(answer)
  }
}
`
)

func TestCountIndexConcurrentTxns(t *testing.T) {
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	x.Check(err)
	testutil.DropAll(t, dg)
	alterSchema(dg, "answer: [uid] @count .")

	// Expected edge count of 0x100: 1
	txn0 := dg.NewTxn()
	mu := api.Mutation{SetNquads: []byte("<0x100> <answer> <0x200> .")}
	_, err = txn0.Mutate(ctxb, &mu)
	x.Check(err)
	err = txn0.Commit(ctxb)
	x.Check(err)

	// The following two mutations are in separate interleaved txns.
	txn1 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x2> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	x.Check(err)

	txn2 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x3> .")}
	_, err = txn2.Mutate(ctxb, &mu)
	x.Check(err)

	err = txn1.Commit(ctxb)
	x.Check(err)
	err = txn2.Commit(ctxb)
	require.Error(t, err,
		"the txn2 should be aborted due to concurrent update on the count index of	<0x01>")

	// retry the mutatiton
	txn3 := dg.NewTxn()
	_, err = txn3.Mutate(ctxb, &mu)
	x.Check(err)
	err = txn3.Commit(ctxb)
	x.Check(err)

	// Verify count queries
	txn := dg.NewReadOnlyTxn()
	vars := map[string]string{"$num": "1"}
	resp, err := txn.QueryWithVars(ctxb, countQuery, vars)
	x.Check(err)
	js := string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 1, "uid": "0x100"}]}`,
		js)
	txn = dg.NewReadOnlyTxn()
	vars = map[string]string{"$num": "2"}
	resp, err = txn.QueryWithVars(ctxb, countQuery, vars)
	x.Check(err)
	js = string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 2, "uid": "0x1"}]}`,
		js)
}

func TestCountIndexSerialTxns(t *testing.T) {
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	x.Check(err)
	testutil.DropAll(t, dg)
	alterSchema(dg, "answer: [uid] @count .")

	// Expected Edge count of 0x100: 1
	txn0 := dg.NewTxn()
	mu := api.Mutation{SetNquads: []byte("<0x100> <answer> <0x200> .")}
	_, err = txn0.Mutate(ctxb, &mu)
	require.NoError(t, err)
	err = txn0.Commit(ctxb)
	require.NoError(t, err)

	// Expected edge count of 0x1: 2
	// This should NOT appear in the query result
	// The following two mutations are in serial txns.
	txn1 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x2> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	require.NoError(t, err)
	err = txn1.Commit(ctxb)
	require.NoError(t, err)

	txn2 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x3> .")}
	_, err = txn2.Mutate(ctxb, &mu)
	require.NoError(t, err)
	err = txn2.Commit(ctxb)
	require.NoError(t, err)

	// Verify query
	txn := dg.NewReadOnlyTxn()
	vars := map[string]string{"$num": "1"}
	resp, err := txn.QueryWithVars(ctxb, countQuery, vars)
	require.NoError(t, err)
	js := string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 1, "uid": "0x100"}]}`,
		js)
	txn = dg.NewReadOnlyTxn()
	vars = map[string]string{"$num": "2"}
	resp, err = txn.QueryWithVars(ctxb, countQuery, vars)
	require.NoError(t, err)
	js = string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 2, "uid": "0x1"}]}`,
		js)
}

func TestCountIndexSameTxn(t *testing.T) {
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	x.Check(err)
	testutil.DropAll(t, dg)
	alterSchema(dg, "answer: [uid] @count .")

	// Expected Edge count of 0x100: 1
	txn0 := dg.NewTxn()
	mu := api.Mutation{SetNquads: []byte("<0x100> <answer> <0x200> .")}
	_, err = txn0.Mutate(ctxb, &mu)
	x.Check(err)
	err = txn0.Commit(ctxb)
	x.Check(err)

	// Expected edge count of 0x1: 2
	// This should NOT appear in the query result
	// The following two mutations are in the same txn.
	txn1 := dg.NewTxn()
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x2> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	x.Check(err)
	mu = api.Mutation{SetNquads: []byte("<0x1> <answer> <0x3> .")}
	_, err = txn1.Mutate(ctxb, &mu)
	x.Check(err)
	err = txn1.Commit(ctxb)
	x.Check(err)

	// Verify query
	txn := dg.NewReadOnlyTxn()
	vars := map[string]string{"$num": "1"}
	resp, err := txn.QueryWithVars(ctxb, countQuery, vars)
	x.Check(err)
	js := string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 1, "uid": "0x100"}]}`,
		js)
	txn = dg.NewReadOnlyTxn()
	vars = map[string]string{"$num": "2"}
	resp, err = txn.QueryWithVars(ctxb, countQuery, vars)
	x.Check(err)
	js = string(resp.GetJson())
	require.JSONEq(t,
		`{"me": [{"count(answer)": 2, "uid": "0x1"}]}`,
		js)
}

func TestConcurrentQueryMutate(t *testing.T) {
	testutil.DropAll(t, s.dg)
	alterSchema(s.dg, "name: string .")

	txn := s.dg.NewTxn()
	defer txn.Discard(context.Background())

	// Do one query, so a new timestamp is assigned to the txn.
	q := `{me(func: uid(0x01)) { name }}`
	_, err := runQueryWithRetry(context.Background(), txn, q)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(2)
	start := time.Now()
	go func() {
		defer wg.Done()
		for time.Since(start) < 5*time.Second {
			mu := &api.Mutation{}
			mu.SetJson = []byte(`{"uid": "0x01", "name": "manish"}`)
			_, err := txn.Mutate(context.Background(), mu)
			assert.Nil(t, err)
		}
	}()

	go func() {
		defer wg.Done()
		for time.Since(start) < 5*time.Second {
			_, err := runQueryWithRetry(context.Background(), txn, q)
			require.NoError(t, err)
		}
	}()
	wg.Wait()
	t.Logf("Done\n")
}

func TestTxnDiscardBeforeCommit(t *testing.T) {
	testutil.DropAll(t, s.dg)
	alterSchema(s.dg, "name: string .")

	txn := s.dg.NewTxn()
	mu := &api.Mutation{
		SetNquads: []byte(`_:1 <name> "abc" .`),
	}
	_, err := txn.Mutate(context.Background(), mu)
	require.NoError(t, err, "unable to mutate")

	err = txn.Discard(context.Background())
	// Since client is discarding this transaction server should not throw ErrAborted err.
	require.NotEqual(t, err, dgo.ErrAborted)
}

func alterSchema(dg *dgo.Dgraph, schema string) {
	op := api.Operation{}
	op.Schema = schema
	err := dg.Alter(ctxb, &op)
	x.Check(err)
}
