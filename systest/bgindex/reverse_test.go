// +build systest

/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
)

func TestReverseIndex(t *testing.T) {
	total := 100000

	dg, err := getClient()
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}

	testutil.DropAll(t, dg)
	if err := dg.Alter(context.Background(), &api.Operation{
		Schema: "balance: [uid] .",
	}); err != nil {
		t.Fatalf("error in setting up schema :: %v\n", err)
	}

	if err := testutil.AssignUids(uint64(total * 10)); err != nil {
		t.Fatalf("error in assignig UIDs :: %v", err)
	}

	// first insert bank accounts
	fmt.Println("inserting edges")
	for i := 1; i < total; {
		bb := &bytes.Buffer{}
		for j := 0; j < 10000; j++ {
			_, err := bb.WriteString(fmt.Sprintf("<%v> <balance> <%v> .\n", i, i+1))
			if err != nil {
				t.Fatalf("error in mutation %v\n", err)
			}
			i++
		}
		dg.NewTxn().Mutate(context.Background(), &api.Mutation{
			CommitNow: true,
			SetNquads: bb.Bytes(),
		})
	}

	fmt.Println("building indexes in background")
	if err := dg.Alter(context.Background(), &api.Operation{
		Schema: "balance: [uid] @reverse .",
	}); err != nil {
		t.Fatalf("error in adding indexes :: %v\n", err)
	}

	numEdges := int64(total)
	updated := sync.Map{}
	mutateUID := func(uid int) {
		switch uid % 4 {
		case 0:
			if _, err := dg.NewTxn().Mutate(context.Background(), &api.Mutation{
				CommitNow: true,
				SetNquads: []byte(fmt.Sprintf(`<%v> <balance> <%v> .`, uid-2, uid)),
			}); err != nil && !errors.Is(err, dgo.ErrAborted) {
				log.Fatalf("error in mutation :: %v\n", err)
			} else if errors.Is(err, dgo.ErrAborted) {
				return
			}
			updated.Store(uid, nil)
		case 1:
			v := atomic.AddInt64(&numEdges, 1)
			if _, err := dg.NewTxn().Mutate(context.Background(), &api.Mutation{
				CommitNow: true,
				SetNquads: []byte(fmt.Sprintf(`<%v> <balance> <%v> .`, v-1, v)),
			}); err != nil {
				t.Fatalf("error in insertion :: %v\n", err)
			}
		case 2:
			if _, err := dg.NewTxn().Mutate(context.Background(), &api.Mutation{
				CommitNow: true,
				DelNquads: []byte(fmt.Sprintf(`<%v> <balance> <%v> .`, uid-1, uid)),
			}); err != nil && !errors.Is(err, dgo.ErrAborted) {
				log.Fatalf("error in mutation :: %v\n", err)
			} else if errors.Is(err, dgo.ErrAborted) {
				return
			}
			updated.Store(uid, nil)
		case 3:
			if _, err := dg.NewTxn().Mutate(context.Background(), &api.Mutation{
				CommitNow: true,
				SetNquads: []byte(fmt.Sprintf(`<%v> <balance> <%v> .
											  <%v> <balance> <%v> .`,
					uid+1, uid, uid-2, uid)),
			}); err != nil && !errors.Is(err, dgo.ErrAborted) {
				log.Fatalf("error in mutation :: %v\n", err)
			} else if errors.Is(err, dgo.ErrAborted) {
				return
			}
			updated.Store(uid, nil)
		}
	}

	// perform mutations until ctrl+c
	var swg sync.WaitGroup
	var counter uint64
	quit := make(chan struct{})
	runLoop := func() {
		defer swg.Done()
		for {
			select {
			case <-quit:
				return
			default:
				mutateUID(rand.Intn(total) + 1)
				atomic.AddUint64(&counter, 1)
			}
		}
	}

	swg.Add(101)
	for i := 0; i < 100; i++ {
		go runLoop()
	}
	go printStats(&counter, quit, &swg)
	checkSchemaUpdate(`{ q(func: uid(0x01)) { ~balance { uid }}}`, dg)
	close(quit)
	swg.Wait()
	fmt.Println("mutations done")

	// check values now
	checkUID := func(i int) error {
		q := fmt.Sprintf(`{ q(func: uid(%v)) { ~balance { uid }}}`, i)
		resp, err := dg.NewReadOnlyTxn().Query(context.Background(), q)
		if err != nil {
			t.Fatalf("error in query :: %v\n", err)
		}
		var data struct {
			Q []struct {
				Balance []struct {
					UID string
				} `json:"~balance"`
			}
		}
		if err := json.Unmarshal(resp.Json, &data); err != nil {
			t.Fatalf("error in json.Unmarshal :: %v", err)
		}

		_, ok := updated.Load(i)
		switch {
		case !ok || i > total || i%4 == 1:
			if len(data.Q) != 1 || len(data.Q[0].Balance) != 1 {
				return fmt.Errorf("length not equal, no mod, got: %+v", data)
			}
			v1, err := strconv.ParseInt(data.Q[0].Balance[0].UID, 0, 64)
			if err != nil {
				return err
			}
			if int(v1) != i-1 {
				return fmt.Errorf("value not equal, got: %+v", data)
			}
		case i%4 == 0:
			if len(data.Q) != 1 || len(data.Q[0].Balance) != 2 {
				return fmt.Errorf("length not equal, got: %+v", data)
			}
			v1, err := strconv.ParseInt(data.Q[0].Balance[0].UID, 0, 64)
			if err != nil {
				return err
			}
			v2, err := strconv.ParseInt(data.Q[0].Balance[1].UID, 0, 64)
			if err != nil {
				return err
			}
			l := []int{int(v1), int(v2)}
			sort.Ints(l)
			if l[0] != i-2 || l[1] != i-1 {
				return fmt.Errorf("value not equal, got: %+v", data)
			}
		case i%4 == 2:
			if len(data.Q) != 0 {
				return fmt.Errorf("length not equal, del, got: %+v", data)
			}
		case i%4 == 3:
			if len(data.Q) != 1 || len(data.Q[0].Balance) != 3 {
				return fmt.Errorf("length not equal, got: %+v", data)
			}
			v1, err := strconv.ParseInt(data.Q[0].Balance[0].UID, 0, 64)
			if err != nil {
				return err
			}
			v2, err := strconv.ParseInt(data.Q[0].Balance[1].UID, 0, 64)
			if err != nil {
				return err
			}
			v3, err := strconv.ParseInt(data.Q[0].Balance[2].UID, 0, 64)
			if err != nil {
				return err
			}
			l := []int{int(v1), int(v2), int(v3)}
			sort.Ints(l)
			if l[0] != i-2 || l[1] != i-1 || l[2] != i+1 {
				return fmt.Errorf("value not equal, got: %+v", data)
			}
		}

		return nil
	}

	type pair struct {
		uid int
		err string
	}
	ch := make(chan pair, numEdges)

	fmt.Println("starting to query")
	var wg sync.WaitGroup
	var count uint64
	for i := 2; i <= int(numEdges); i += 100 {
		wg.Add(1)
		go func(j int) {
			defer wg.Done()
			for k := j; k < j+100 && k <= int(numEdges); k++ {
				if err := checkUID(k); err != nil {
					ch <- pair{k, err.Error()}
				}
				atomic.AddUint64(&count, 1)
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			time.Sleep(2 * time.Second)
			cur := atomic.LoadUint64(&count)
			fmt.Printf("%v/%v done\n", cur, numEdges-1)
			if cur+1 == uint64(numEdges) {
				break
			}
		}
	}()
	wg.Wait()

	close(ch)
	for p := range ch {
		t.Fatalf("failed for %v, :: %v\n", p.uid, p.err)
	}
}
