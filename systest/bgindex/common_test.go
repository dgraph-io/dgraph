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
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo/v210"
)

func printStats(counter *uint64, quit <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-quit:
			return
		case <-time.After(2 * time.Second):
		}

		fmt.Println("mutations:", atomic.LoadUint64(counter))
	}
}

// blocks until query returns no error.
func waitForSchemaUpdate(query string, dg *dgo.Dgraph) {
	for {
		time.Sleep(2 * time.Second)
		_, err := dg.NewReadOnlyTxn().Query(context.Background(), query)
		if err != nil {
			continue
		}

		return
	}
}
