/*
 * Copyright 2017-2019 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// propposeAndWaitEmulator emulates proposeAndWait. It has has one function(propose) inside it,
// which returns errInternalRetry 50% of the time. Rest of the time it just sleeps for 1 second
// to emulate successful response.
func propposeAndWaitEmulator() error {
	// succeed/fail with equal probability.
	propose := func(timeout time.Duration) error {
		num := int(rand.Int31n(10))
		if num%2 == 0 {
			return errInternalRetry
		}

		// Sleep for 1 second, to emulate successful behaviour.
		time.Sleep(1 * time.Second)
		return nil
	}

	runPropose := func(i int) error {
		if err := limiter.incr(context.Background(), i); err != nil {
			return err
		}
		defer limiter.decr(i)
		return propose(newTimeout(i))
	}

	for i := 0; i < 3; i++ {
		if err := runPropose(i); err != errInternalRetry {
			return err
		}
	}
	return errUnableToServe
}

// This test tests for deadlock in rate limiter. It tried some fixed number of proposals in
// multiple goroutines. At the end it matches if sum of completed and aborted proposals is
// equal to tried proposals or not.
func TestLimiter(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	toTry := int64(1000) // total proposals count to propose.
	var currentCount, pending, completed, aborted int64

	limiter = rateLimiter{c: sync.NewCond(&sync.Mutex{}), max: 256}
	go limiter.c.Broadcast()

	go func() {
		now := time.Now()
		for range time.Tick(1 * time.Second) {
			fmt.Println("Seconds elapsed :", int64(time.Since(now).Seconds()),
				"Total proposals: ", atomic.LoadInt64(&currentCount),
				"Pending proposal: ", atomic.LoadInt64(&pending),
				"Completed Proposals: ", atomic.LoadInt64(&completed),
				"Aboted Proposals: ", atomic.LoadInt64(&aborted),
				"IOU: ", limiter.iou)
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(no int) {
			defer wg.Done()

			for {
				if atomic.AddInt64(&currentCount, 1) > toTry {
					break
				}
				atomic.AddInt64(&pending, 1)
				if err := propposeAndWaitEmulator(); err != nil {
					atomic.AddInt64(&aborted, 1)
				} else {
					atomic.AddInt64(&completed, 1)
				}
				atomic.AddInt64(&pending, -1)
			}
		}(i)
	}
	wg.Wait()

	// After trying all the proposals, (completed + aborted) should be equal to  tried proposal.
	require.True(t, toTry == completed+aborted,
		fmt.Sprintf("Tried: %d, Compteted: %d, Aboted: %d", toTry, completed, aborted))
}
