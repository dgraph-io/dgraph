//go:build upgrade

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

func TestMain(m *testing.M) {
	mutate := func(c dgraphtest.Cluster) {
		dg, cleanup, err := c.Client()
		x.Panic(err)
		defer cleanup()
		x.Panic(dg.LoginIntoNamespace(context.Background(), dgraphtest.DefaultUser, dgraphtest.DefaultPassword, 0))

		client = dg.Dgraph
		dc = c
		populateCluster()
	}

	query := func(c dgraphtest.Cluster) {
		dg, cleanup, err := c.Client()
		x.Panic(err)
		defer cleanup()
		x.Panic(dg.LoginIntoNamespace(context.Background(), dgraphtest.DefaultUser, dgraphtest.DefaultPassword, 0))

		client = dg.Dgraph
		dc = c
		if m.Run() != 0 {
			panic("tests failed")
		}
	}

	runTest := func(before, after string) {
		conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).
			WithReplicas(3).WithACL(time.Hour).WithVersion(before)
		c, err := dgraphtest.NewLocalCluster(conf)
		x.Panic(err)
		defer c.Cleanup()
		x.Panic(c.Start())

		hc, err := c.HTTPClient()
		x.Panic(err)
		x.Panic(hc.LoginIntoNamespace(dgraphtest.DefaultUser, dgraphtest.DefaultPassword, 0))

		mutate(c)
		x.Panic(c.Upgrade(after, dgraphtest.BackupRestore))
		query(c)
	}

	for _, cv := range dgraphtest.UpgradeCombos {
		log.Printf("running: backup in [%v], restore in [%v]", cv[0], cv[1])
		runTest(cv[0], cv[1])
	}
}
