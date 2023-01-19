/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// proposeAndWaitEmulator emulates proposeAndWait. It has one function(propose) inside it,
// which returns errInternalRetry 50% of the time. Rest of the time it just sleeps for 1 second
// to emulate successful response, if sleep is true. It also expects maxRetry as argument, which
// is max number of times propose should be called for each errInternalRetry.
func proposeAndWaitEmulator(l *rateLimiter, r *rand.Rand, maxRetry int, sleep bool) error {
	// succeed/fail with equal probability.
	propose := func(timeout time.Duration) error {
		num := int(r.Int31n(10))
		if num%2 == 0 {
			return errInternalRetry
		}

		// Sleep for 1 second, to emulate successful behaviour.
		if sleep {
			time.Sleep(1 * time.Second)
		}
		return nil
	}

	runPropose := func(i int) error {
		if err := l.incr(context.Background(), i); err != nil {
			return err
		}
		defer l.decr(i)
		return propose(newTimeout(i))
	}

	for i := 0; i < maxRetry; i++ {
		if err := runPropose(i); err != errInternalRetry {
			return err
		}
	}
	return errUnableToServe
}

// This test tests for deadlock in rate limiter. It tried some fixed number of proposals in
// multiple goroutines. At the end it matches if sum of completed and aborted proposals is
// equal to tried proposals or not.
func TestLimiterDeadlock(t *testing.T) {
	toTry := int64(3000) // total proposals count to propose.
	var currentCount, pending, completed, aborted int64

	l := &rateLimiter{c: sync.NewCond(&sync.Mutex{}), max: 256}
	go l.bleed()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	go func() {
		now := time.Now()
		for range ticker.C {
			l.c.L.Lock()
			fmt.Println("Seconds elapsed :", int64(time.Since(now).Seconds()),
				"Total proposals: ", atomic.LoadInt64(&currentCount),
				"Pending proposal: ", atomic.LoadInt64(&pending),
				"Completed Proposals: ", atomic.LoadInt64(&completed),
				"Aborted Proposals: ", atomic.LoadInt64(&aborted),
				"IOU: ", l.iou)
			l.c.L.Unlock()
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(no int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			for {
				if atomic.AddInt64(&currentCount, 1) > toTry {
					break
				}
				atomic.AddInt64(&pending, 1)
				if err := proposeAndWaitEmulator(l, r, 3, true); err != nil {
					atomic.AddInt64(&aborted, 1)
				} else {
					atomic.AddInt64(&completed, 1)
				}
				atomic.AddInt64(&pending, -1)
			}
		}(i)
	}
	wg.Wait()
	ticker.Stop()

	// After trying all the proposals, (completed + aborted) should be equal to  tried proposal.
	require.True(t, toTry == completed+aborted,
		fmt.Sprintf("Tried: %d, Compteted: %d, Aborted: %d", toTry, completed, aborted))
}

func BenchmarkRateLimiter(b *testing.B) {
	ious := []int{256}
	retries := []int{3}
	var failed, success uint64

	for _, iou := range ious {
		for _, retry := range retries {
			b.Run(fmt.Sprintf("IOU:%d-Retry:%d", iou, retry), func(b *testing.B) {
				l := &rateLimiter{c: sync.NewCond(&sync.Mutex{}), max: iou}
				go l.bleed()

				// var success, failed uint64
				b.RunParallel(func(pb *testing.PB) {
					r := rand.New(rand.NewSource(time.Now().UnixNano()))
					for pb.Next() {
						if err := proposeAndWaitEmulator(l, r, retry, false); err != nil {
							atomic.AddUint64(&failed, 1)
						} else {
							atomic.AddUint64(&success, 1)
						}
					}
				})

				fmt.Println("IOU:", iou, "Max Retries:", retry, "Success:",
					success, "Failed:", failed)
			})
		}
	}
}
