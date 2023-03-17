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
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
)

func TestMain(m *testing.M) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).WithReplicas(3).
		WithACL(20 * time.Second).WithEncryption().WithVersion("0c9f60156")
	c, err := dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	defer c.Cleanup()
	c.Start()

	// setup the global Cluster var
	dc = c

	// setup client
	dg, err := c.Client()
	if err != nil {
		panic(err)
	}
	client = dg

	// do mutations
	populateCluster()

	// upgrade
	x.Panic(c.Upgrade("v23.0.0-beta1"))

	// setup the client again
	dg, err = c.Client()
	if err != nil {
		panic(err)
	}
	client = dg

	// Run tests
	_ = m.Run()
}
