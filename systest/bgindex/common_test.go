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
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
)

func getClient() (*dgo.Dgraph, error) {
	ports := []int{9180, 9182, 9183, 9184, 9185, 9186}
	conns := make([]api.DgraphClient, len(ports))
	for i, port := range ports {
		conn, err := grpc.Dial(fmt.Sprintf("localhost:%v", port), grpc.WithInsecure())
		if err != nil {
			return nil, err
		}

		conns[i] = api.NewDgraphClient(conn)
	}
	dg := dgo.NewDgraphClient(conns...)

	ctx := context.Background()
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err := dg.Login(ctx, "groot", "password")
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}

	return dg, nil
}

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

func checkSchemaUpdate(query string, dg *dgo.Dgraph) {
	for {
		time.Sleep(2 * time.Second)
		_, err := dg.NewReadOnlyTxn().Query(context.Background(), query)
		if err != nil {
			continue
		}

		return
	}
}
