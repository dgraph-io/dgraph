//go:build integration2

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors *
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

package acl

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/dgraphtest"
	"github.com/dgraph-io/dgraph/v24/x"
)

func TestACLJwtAlgo(t *testing.T) {
	for _, algo := range jwt.GetAlgorithms() {
		if algo == "none" || algo == "" {
			continue
		}

		t.Run(fmt.Sprintf("running cluster with algorithm: %v", algo), func(t *testing.T) {
			conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
				WithReplicas(1).WithACL(2 * time.Second).WithAclAlg(jwt.GetSigningMethod(algo))
			c, err := dgraphtest.NewLocalCluster(conf)
			require.NoError(t, err)
			defer func() { c.Cleanup(t.Failed()) }()
			require.NoError(t, c.Start())

			gc, cleanup, err := c.Client()
			require.NoError(t, err)
			defer cleanup()
			require.NoError(t, gc.LoginIntoNamespace(context.Background(),
				dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

			// op with Grpc client
			_, err = gc.Query(`{q(func: uid(0x1)) {uid}}`)
			require.NoError(t, err)

			// wait for the token to expire
			time.Sleep(3 * time.Second)
			_, err = gc.Query(`{q(func: uid(0x1)) {uid}}`)
			require.NoError(t, err)

			hc, err := c.HTTPClient()
			require.NoError(t, err)
			require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
				dgraphapi.DefaultPassword, x.GalaxyNamespace))

			// op with HTTP client
			require.NoError(t, hc.Backup(c, true, dgraphtest.DefaultBackupDir))
		})
	}
}
