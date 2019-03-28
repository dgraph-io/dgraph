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
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type state struct {
	dg *dgo.Dgraph
}

var s state
var addr string = z.SockAddr

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	dg := z.DgraphClientWithGroot(z.SockAddr)
	s.dg = dg

	r := m.Run()
	os.Exit(r)
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
	var uid string
	for _, u := range assigned.Uids {
		uid = u
	}

	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn.Query(context.Background(), q)
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
	resp, err := txn2.Query(context.Background(), q)
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
	resp, err := txn.Query(context.Background(), q)
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
	assigned, err = txn3.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	q := fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err := txn2.Query(context.Background(), q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))

	require.NoError(t, txn3.Commit(context.Background()))

	txn4 := s.dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%s)) { name }}`, uid)
	resp, err = txn4.Query(context.Background(), q)
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
	// We don't supply startTs, it should be fetched from zero by dgraph alpha.
	req := api.Request{
		Query: q,
	}

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := api.NewDgraphClient(conn)

	resp, err := dc.Query(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"Manish\"}]}")))
	x.AssertTrue(resp.Txn.StartTs > 0)

	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"uid\": \"%s\", \"name\": \"Manish2\"}", uid))

	mu.CommitNow = true
	res, err := dc.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	x.AssertTrue(res.Context.StartTs > 0)
	resp, err = dc.Query(context.Background(), &req)
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
	resp, err := txn.Query(context.Background(), q)
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
	_, err := txn2.Query(context.Background(), q)
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
	_, err = txn3.Query(context.Background(), q)
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
	_, err = txn4.Query(context.Background(), q)
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
	resp, err := txn.Query(context.Background(), q)
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
	resp, err := txn.Query(context.Background(), q)
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
	uid1 := assigned.Uids["blank-0"]
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
	uid2 := assigned.Uids["blank-0"]

	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			friend {
				uid
				name
			}
		}
	}`, uid1)

	resp, err := txn.Query(context.Background(), q)
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
	uid1 := assigned.Uids["blank-0"]
	uid2 := assigned.Uids["blank-1"]
	require.NoError(t, err)
	require.Equal(t, 2, len(assigned.Uids))

	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			friend {
				uid
				name
			}
		}
	}`, uid1)

	resp, err := txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp := fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Delete S P *
	mu = &api.Mutation{}
	dgo.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid1)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Add edge
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan2"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))
	uid2 = assigned.Uids["blank-0"]

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan2", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Delete S P *
	mu = &api.Mutation{}
	dgo.DeleteEdges(mu, uid1, "friend")
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 0, len(assigned.Uids))

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s"}]}`, uid1)
	require.JSONEq(t, expectedResp, string(resp.Json))

	// Add edge
	mu = &api.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf(`{"uid": "%s" ,"name": "Manish", "friend": [{"name": "Jan3"}]}`, uid1))
	assigned, err = txn.Mutate(context.Background(), mu)
	require.NoError(t, err)
	require.Equal(t, 1, len(assigned.Uids))
	uid2 = assigned.Uids["blank-0"]

	resp, err = txn.Query(context.Background(), q)
	require.NoError(t, err)
	expectedResp = fmt.Sprintf(`{"me":[{"uid":"%s", "friend": [{"name": "Jan3", "uid":"%s"}]}]}`, uid1, uid2)
	require.JSONEq(t, expectedResp, string(resp.Json))
}
