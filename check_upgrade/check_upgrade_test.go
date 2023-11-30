//go:build integration

/*
 * Copyright 2024 Dgraph Labs, Inc. and Contributors
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

package checkupgrade

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v230/protos/api"
	"github.com/dgraph-io/dgraph/dgraphapi"
	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func TestCheckUpgrade(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithACL(time.Hour).WithVersion("57aa5c4ac")
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.GalaxyNamespace))

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.GalaxyNamespace))

	rdfs := ` 
		_:a <dgraph.xid> "user1" .
	    _:a <dgraph.type> "dgraph.type.User"  .
		_:b <dgraph.xid> "user1" .
		_:b <dgraph.type> "dgraph.type.User"  .`

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.NoError(t, err)

	var nss []uint64
	for i := 0; i < 5; i++ {
		ns, err := hc.AddNamespace()
		require.NoError(t, err)
		require.NoError(t, gc.LoginIntoNamespace(context.Background(), "groot", "password", ns))
		mu = &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
		_, err = gc.Mutate(mu)
		require.NoError(t, err)
		nss = append(nss, ns)
	}

	conf1 := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).WithACL(time.Hour)
	c1, err := dgraphtest.NewLocalCluster(conf1)
	require.NoError(t, err)
	defer func() { c1.Cleanup(t.Failed()) }()
	require.NoError(t, c1.Start())
	alphaHttp, err := c.GetAlphaHttpPublicPort()
	require.NoError(t, err)

	alphaGrpc, err := c.GetAlphaGrpcPublicPort()
	require.NoError(t, err)

	args := []string{
		"checkupgrade",
		"--grpc_port", "localhost:" + alphaGrpc,
		"--http_port", "localhost:" + alphaHttp,
		"--dgUser", "groot",
		"--password", "password",
	}

	cmd := exec.Command(filepath.Join(c1.GetTempDir(), "dgraph"), args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	actualOutput := string(out)

	expectedOutputPattern := `Found duplicate users in namespace: #\d+\ndgraph\.xid user1 , Uids: \[\d+x\d+ \d+x\d+\]\n`
	match, err := regexp.MatchString(expectedOutputPattern, actualOutput)
	require.NoError(t, err)

	if !match {
		t.Errorf("Output does not match expected pattern.\nExpected pattern:\n%s\n\nGot:\n%s",
			expectedOutputPattern, actualOutput)
	}
}
