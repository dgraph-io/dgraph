//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgo/v250"
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
