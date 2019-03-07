/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package counter

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

const N = 10
const pred = "counter"

func increment(t *testing.T, dg *dgo.Dgraph) int {
	var max int
	var mu sync.Mutex
	storeMax := func(a int) {
		mu.Lock()
		if max < a {
			max = a
		}
		mu.Unlock()
	}

	var wg sync.WaitGroup
	// N goroutines, process N times each goroutine.
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < N; i++ {
				cnt, err := process(dg, false, pred)
				if err != nil {
					if strings.Index(err.Error(), "Transaction has been aborted") >= 0 {
						// pass
					} else {
						t.Logf("Error while incrementing: %v\n", err)
					}
				} else {
					storeMax(cnt.Val)
				}
			}
		}()
	}
	wg.Wait()
	return max
}

func read(t *testing.T, dg *dgo.Dgraph, expected int) {
	cnt, err := process(dg, true, pred)
	require.NoError(t, err)
	ts := cnt.startTs
	t.Logf("Readonly stage counter: %+v\n", cnt)

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < N; i++ {
				cnt, err := process(dg, true, pred)
				if err != nil {
					t.Logf("Error while reading: %v\n", err)
				} else {
					require.Equal(t, expected, cnt.Val)
					require.Equal(t, ts, cnt.startTs)
				}
			}
		}()
	}
	wg.Wait()
}

func TestIncrement(t *testing.T) {
	dg := z.DgraphClientWithGroot(":9180")
	ctx := context.Background()
	op := api.Operation{DropAll: true}

	// The following piece of code shows how one can set metadata with
	// auth-token, to allow Alter operation, if the server requires it.
	md := metadata.New(nil)
	md.Append("auth-token", "mrjn2")
	ctx = metadata.NewOutgoingContext(ctx, md)
	x.Check(dg.Alter(ctx, &op))

	cnt, err := process(dg, false, pred)
	if err != nil {
		t.Logf("Error while reading: %v\n", err)
	} else {
		t.Logf("Initial value: %d\n", cnt.Val)
	}

	val := increment(t, dg)
	t.Logf("Increment stage done. Got value: %d\n", val)
	read(t, dg, val)
	t.Logf("Read stage done with value: %d\n", val)
	val = increment(t, dg)
	t.Logf("Increment stage done. Got value: %d\n", val)
	read(t, dg, val)
	t.Logf("Read stage done with value: %d\n", val)
}
