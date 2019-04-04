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
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

const N = 10
const pred = "counter"

func incrementInLoop(t *testing.T, dg *dgo.Dgraph, M int) int {
	conf := viper.New()
	conf.Set("pred", "counter.val")

	var max int
	for i := 0; i < M; i++ {
		cnt, err := process(dg, conf)
		if err != nil {
			if strings.Index(err.Error(), "Transaction has been aborted") >= 0 {
				// pass
			} else {
				t.Logf("Error while incrementing: %v\n", err)
			}
		} else {
			if cnt.Val > max {
				max = cnt.Val
			}
		}
	}
	t.Logf("Last value written by increment in loop: %d", max)
	return max
}

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
			max := incrementInLoop(t, dg, N)
			storeMax(max)
		}()
	}
	wg.Wait()
	return max
}

func read(t *testing.T, dg *dgo.Dgraph, expected int) {
	conf := viper.New()
	conf.Set("pred", "counter.val")
	conf.Set("ro", true)
	cnt, err := process(dg, conf)
	require.NoError(t, err)
	ts := cnt.startTs
	t.Logf("Readonly stage counter: %+v\n", cnt)

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < N; i++ {
				cnt, err := process(dg, conf)
				if err != nil {
					t.Logf("Error while reading: %v\n", err)
				} else {
					require.Equal(t, expected, cnt.Val)
					require.True(t, cnt.startTs >= ts, "the timestamp should never decrease")
				}
			}
		}()
	}
	wg.Wait()
}

func readBestEffort(t *testing.T, dg *dgo.Dgraph, pred string, M int) {
	conf := viper.New()
	conf.Set("pred", pred)
	conf.Set("be", true)
	var last int
	for i := 0; i < M; i++ {
		cnt, err := process(dg, conf)
		if err != nil {
			t.Errorf("Error while reading: %v", err)
		} else {
			if last > cnt.Val {
				t.Errorf("Current %d < Last %d", cnt.Val, last)
			}
			last = cnt.Val
		}
	}
	t.Logf("Last value read by best effort: %d", last)
}

func setup(t *testing.T) *dgo.Dgraph {
	dg := z.DgraphClientWithGroot(z.SockAddr)
	ctx := context.Background()
	op := api.Operation{DropAll: true}

	// The following piece of code shows how one can set metadata with
	// auth-token, to allow Alter operation, if the server requires it.
	md := metadata.New(nil)
	md.Append("auth-token", "mrjn2")
	ctx = metadata.NewOutgoingContext(ctx, md)
	x.Check(dg.Alter(ctx, &op))

	conf := viper.New()
	conf.Set("pred", "counter.val")
	cnt, err := process(dg, conf)
	if err != nil {
		t.Logf("Error while reading: %v\n", err)
	} else {
		t.Logf("Initial value: %d\n", cnt.Val)
	}

	return dg
}

func TestIncrement(t *testing.T) {
	dg := setup(t)
	val := increment(t, dg)
	t.Logf("Increment stage done. Got value: %d\n", val)
	read(t, dg, val)
	t.Logf("Read stage done with value: %d\n", val)
	val = increment(t, dg)
	t.Logf("Increment stage done. Got value: %d\n", val)
	read(t, dg, val)
	t.Logf("Read stage done with value: %d\n", val)
}

func TestBestEffort(t *testing.T) {
	dg := setup(t)

	var done int32
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			incrementInLoop(t, dg, 5)
			if atomic.LoadInt32(&done) > 0 {
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(time.Second)
		readBestEffort(t, dg, "counter.val", 1000)
		atomic.AddInt32(&done, 1)
	}()
	wg.Wait()
	t.Logf("Write/Best-Effort read stage OK.")
}

func TestBestEffortOnly(t *testing.T) {
	dg := setup(t)
	readBestEffort(t, dg, fmt.Sprintf("counter.val.%d", rand.Int()), 1)
	time.Sleep(time.Second)

	doneCh := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			readBestEffort(t, dg, fmt.Sprintf("counter.val.%d", rand.Int()), 1)
		}
		doneCh <- struct{}{}
	}()

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		t.FailNow()
	case <-doneCh:
	}
	t.Logf("Best-Effort only reads with multiple preds OK.")
}

func TestBestEffortTs(t *testing.T) {
	dg := setup(t)
	pred := "counter.val"
	incrementInLoop(t, dg, 1)
	readBestEffort(t, dg, pred, 1)
	txn := dg.NewReadOnlyTxn().BestEffort()
	_, err := queryCounter(txn, pred)
	require.NoError(t, err)

	incrementInLoop(t, dg, 1)        // Increment the MaxAssigned ts at Alpha.
	_, err = queryCounter(txn, pred) // The timestamp here shouldn't change.
	require.NoError(t, err)
}
