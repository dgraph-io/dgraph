/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package z

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/x"
)

func DgraphClientNoDropAll(serviceAddr string) *dgo.Dgraph {
	conn, err := grpc.Dial(serviceAddr, grpc.WithInsecure())
	x.Check(err)

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	x.Check(err)

	return dg
}

func DropAll(t *testing.T, dg *dgo.Dgraph) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	x.Check(err)

	nodes := DbNodeCount(t, dg)
	// the only node left should be the groot node
	require.Equal(t, 1, nodes)
}

func DbNodeCount(t *testing.T, dg *dgo.Dgraph) int {
	resp, err := dg.NewTxn().Query(context.Background(), `
		{
			q(func: has(_predicate_)) {
				count(uid)
			}
		}
	`)
	x.Check(err)

	type count struct {
		Count int
	}
	type root struct {
		Q []count
	}
	var response root
	err = json.Unmarshal(resp.GetJson(), &response)
	x.Check(err)
	return response.Q[0].Count
}

type LoginParams struct {
	Endpoint   string
	UserID     string
	Passwd     string
	RefreshJwt string
}

// CurlLogin sends a curl request to the curlLoginEndpoint
// and returns the access JWT and refresh JWT extracted from
// the curl command output
func CurlLogin(params *LoginParams) (string, string, error) {
	// login with alice's account using curl
	args := []string{"-X", "POST", params.Endpoint}

	if len(params.RefreshJwt) > 0 {
		args = append(args,
			"-H", fmt.Sprintf(`X-Dgraph-RefreshJWT:%s`, params.RefreshJwt))
	} else {
		args = append(args,
			"-H", fmt.Sprintf(`X-Dgraph-User:%s`, params.UserID),
			"-H", fmt.Sprintf(`X-Dgraph-Password:%s`, params.Passwd))
	}

	userLoginCmd := exec.Command("curl", args...)
	out, err := userLoginCmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("login through curl failed: %v", err)
	}

	var outputJson map[string]map[string]string
	if err := json.Unmarshal(out, &outputJson); err != nil {
		return "", "", fmt.Errorf("unable to unmarshal the output to get JWTs: %v", err)
	}

	data, found := outputJson["data"]
	if !found {
		return "", "", fmt.Errorf("data entry found in the output: %v", err)
	}

	newAccessJwt, found := data["accessJWT"]
	if !found {
		return "", "", fmt.Errorf("no access JWT found in the output")
	}
	newRefreshJwt, found := data["refreshJWT"]
	if !found {
		return "", "", fmt.Errorf("no refresh JWT found in the output")
	}

	return newAccessJwt, newRefreshJwt, nil
}
