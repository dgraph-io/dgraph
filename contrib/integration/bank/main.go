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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	users      = flag.Int("users", 100, "Number of accounts.")
	conc       = flag.Int("txns", 3, "Number of concurrent transactions per client.")
	dur        = flag.String("dur", "1m", "How long to run the transactions.")
	alpha      = flag.String("alpha", "localhost:9080", "Address of Dgraph alpha.")
	verbose    = flag.Bool("verbose", true, "Output all logs in verbose mode.")
	login      = flag.Bool("login", true, "Login as groot. Used for ACL-enabled cluster.")
	slashToken = flag.String("slash-token", "", "Slash GraphQL API token")
)

var startBal = 10

type account struct {
	Uid string `json:"uid"`
	Key int    `json:"key,omitempty"`
	Bal int    `json:"bal,omitempty"`
	Typ string `json:"typ"`
}

type state struct {
	aborts int32
	runs   int32
}

func (s *state) createAccounts(dg *dgo.Dgraph) {
	op := api.Operation{DropAll: true}
	x.Check(dg.Alter(context.Background(), &op))

	op.DropAll = false
	op.Schema = `
	key: int @index(int) @upsert .
	bal: int .
	typ: string @index(exact) @upsert .
	`
	x.Check(dg.Alter(context.Background(), &op))

	var all []account
	for i := 1; i <= *users; i++ {
		a := account{
			Key: i,
			Bal: startBal,
			Typ: "ba",
		}
		all = append(all, a)
	}
	data, err := json.Marshal(all)
	x.Check(err)

	txn := dg.NewTxn()
	defer func() {
		if err := txn.Discard(context.Background()); err != nil {
			log.Fatalf("Discarding transaction failed: %+v\n", err)
		}
	}()

	var mu api.Mutation
	mu.SetJson = data
	if *verbose {
		log.Printf("mutation: %s\n", mu.SetJson)
	}
	_, err = txn.Mutate(context.Background(), &mu)
	x.Check(err)
	x.Check(txn.Commit(context.Background()))
}

func (s *state) runTotal(dg *dgo.Dgraph) error {
	query := `
		{
			q(func: eq(typ, "ba")) {
				uid
				key
				bal
			}
		}
	`
	txn := dg.NewReadOnlyTxn()
	defer func() {
		if err := txn.Discard(context.Background()); err != nil {
			log.Fatalf("Discarding transaction failed: %+v\n", err)
		}
	}()

	resp, err := txn.Query(context.Background(), query)
	if err != nil {
		return err
	}

	m := make(map[string][]account)
	if err := json.Unmarshal(resp.Json, &m); err != nil {
		return err
	}
	accounts := m["q"]
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].Key < accounts[j].Key
	})
	var total int
	for _, a := range accounts {
		total += a.Bal
	}
	if *verbose {
		log.Printf("Read: %v. Total: %d\n", accounts, total)
	}
	if len(accounts) > *users {
		log.Fatalf("len(accounts) = %d", len(accounts))
	}
	if total != *users*startBal {
		log.Fatalf("Total = %d", total)
	}
	return nil
}

func (s *state) findAccount(txn *dgo.Txn, key int) (account, error) {
	query := fmt.Sprintf(`{ q(func: eq(key, %d)) { key, uid, bal, typ }}`, key)
	resp, err := txn.Query(context.Background(), query)
	if err != nil {
		return account{}, err
	}
	m := make(map[string][]account)
	if err := json.Unmarshal(resp.Json, &m); err != nil {
		log.Fatal(err)
	}
	accounts := m["q"]
	if len(accounts) > 1 {
		log.Printf("Query: %s. Response: %s\n", query, resp.Json)
		log.Fatal("Found multiple accounts")
	}
	if len(accounts) == 0 {
		if *verbose {
			log.Printf("Unable to find account for K_%02d. JSON: %s\n", key, resp.Json)
		}
		return account{Key: key, Typ: "ba"}, nil
	}
	return accounts[0], nil
}

func (s *state) runTransaction(dg *dgo.Dgraph, buf *bytes.Buffer) error {
	w := bufio.NewWriter(buf)
	fmt.Fprintf(w, "==>\n")
	defer func() {
		fmt.Fprintf(w, "---\n")
		_ = w.Flush()
	}()

	ctx := context.Background()
	txn := dg.NewTxn()
	defer func() {
		if err := txn.Discard(context.Background()); err != nil {
			log.Fatalf("Discarding transaction failed: %+v\n", err)
		}
	}()

	var sk, sd int
	for {
		sk = rand.Intn(*users + 1)
		sd = rand.Intn(*users + 1)
		if sk == 0 || sd == 0 { // Don't touch zero.
			continue
		}
		if sk != sd {
			break
		}
	}

	src, err := s.findAccount(txn, sk)
	if err != nil {
		return err
	}
	dst, err := s.findAccount(txn, sd)
	if err != nil {
		return err
	}
	if src.Key == dst.Key {
		return nil
	}

	amount := rand.Intn(10)
	if src.Bal-amount <= 0 {
		amount = src.Bal
	}
	fmt.Fprintf(w, "Moving [$%d, K_%02d -> K_%02d]. Src:%+v. Dst: %+v\n",
		amount, src.Key, dst.Key, src, dst)
	src.Bal -= amount
	dst.Bal += amount
	var mu api.Mutation
	if len(src.Uid) > 0 {
		// If there was no src.Uid, then don't run any mutation.
		if src.Bal == 0 {
			pb, err := json.Marshal(src)
			x.Check(err)
			mu.DeleteJson = pb
			fmt.Fprintf(w, "Deleting K_%02d: %s\n", src.Key, mu.DeleteJson)
		} else {
			data, err := json.Marshal(src)
			x.Check(err)
			mu.SetJson = data
		}
		_, err := txn.Mutate(ctx, &mu)
		if err != nil {
			fmt.Fprintf(w, "Error while mutate: %v", err)
			return err
		}
	}

	mu = api.Mutation{}
	data, err := json.Marshal(dst)
	x.Check(err)
	mu.SetJson = data
	assigned, err := txn.Mutate(ctx, &mu)
	if err != nil {
		fmt.Fprintf(w, "Error while mutate: %v", err)
		return err
	}

	if err := txn.Commit(ctx); err != nil {
		return err
	}
	if len(assigned.GetUids()) > 0 {
		fmt.Fprintf(w, "CREATED K_%02d: %+v for %+v\n", dst.Key, assigned.GetUids(), dst)
		for _, uid := range assigned.GetUids() {
			dst.Uid = uid
		}
	}
	fmt.Fprintf(w, "MOVED [$%d, K_%02d -> K_%02d]. Src:%+v. Dst: %+v\n",
		amount, src.Key, dst.Key, src, dst)
	return nil
}

func (s *state) loop(dg *dgo.Dgraph, wg *sync.WaitGroup) {
	defer wg.Done()
	dur, err := time.ParseDuration(*dur)
	if err != nil {
		log.Fatal(err)
	}
	end := time.Now().Add(dur)

	var buf bytes.Buffer
	for i := 0; ; i++ {
		if i%5 == 0 {
			if err := s.runTotal(dg); err != nil {
				log.Printf("Error while runTotal: %v", err)
			}
			continue
		}

		buf.Reset()
		err := s.runTransaction(dg, &buf)
		if *verbose {
			log.Printf("Final error: %v. %s", err, buf.String())
		}
		if err != nil {
			atomic.AddInt32(&s.aborts, 1)
		} else {
			r := atomic.AddInt32(&s.runs, 1)
			if r%100 == 0 {
				a := atomic.LoadInt32(&s.aborts)
				fmt.Printf("Runs: %d. Aborts: %d\n", r, a)
			}
			if time.Now().After(end) {
				return
			}
		}
	}
}

type authorizationCredentials struct {
	token string
}

func (a *authorizationCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"Authorization": a.token}, nil
}

func (a *authorizationCredentials) RequireTransportSecurity() bool {
	return true
}

func grpcConnection(one string) (*grpc.ClientConn, error) {
	if slashToken == nil || *slashToken == "" {
		return grpc.Dial(one, grpc.WithInsecure())
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	return grpc.Dial(
		one,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			RootCAs:    pool,
			ServerName: strings.Split(one, ":")[0],
		})),
		grpc.WithPerRPCCredentials(&authorizationCredentials{*slashToken}),
	)
}

func main() {
	flag.Parse()

	all := strings.Split(*alpha, ",")
	x.AssertTrue(len(all) > 0)

	var clients []*dgo.Dgraph
	for _, one := range all {
		conn, err := grpcConnection(one)
		if err != nil {
			log.Fatal(err)
		}
		dc := api.NewDgraphClient(conn)
		dg := dgo.NewDgraphClient(dc)
		if *login {
			// login as groot to perform the DropAll operation later
			x.Check(dg.Login(context.Background(), "groot", "password"))
		}
		clients = append(clients, dg)
	}

	s := state{}
	s.createAccounts(clients[0])

	var wg sync.WaitGroup
	for i := 0; i < *conc; i++ {
		for _, dg := range clients {
			wg.Add(1)
			go s.loop(dg, &wg)
		}
	}
	wg.Wait()
	fmt.Println()
	fmt.Println("Total aborts", s.aborts)
	fmt.Println("Total success", s.runs)
	if err := s.runTotal(clients[0]); err != nil {
		log.Fatal(err)
	}
}
