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
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

func TestMain(m *testing.M) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
		WithReplicas(1).WithACL(time.Hour).WithVersion("0c9f60156")
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	defer c.Cleanup()
	x.Panic(c.Start())

	dg, cleanup, err := c.Client()
	x.Panic(err)
	defer cleanup()
	x.Panic(dg.LoginIntoNamespace(context.Background(), dgraphtest.DefaultUser, dgraphtest.DefaultPassword, 0))

	client = dg.Dgraph
	dc = c
	populateCluster()
	x.Panic(c.Upgrade("v23.0.0-beta1", dgraphtest.BackupRestore))

	dg, cleanup, err = c.Client()
	x.Panic(err)
	defer cleanup()
	x.Panic(dg.LoginIntoNamespace(context.Background(), dgraphtest.DefaultUser, dgraphtest.DefaultPassword, 0))

	client = dg.Dgraph
	dc = c
	_ = m.Run()
}
