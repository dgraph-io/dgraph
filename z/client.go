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
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

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

// HttpLogin sends a HTTP request to the server
// and returns the access JWT and refresh JWT extracted from
// the HTTP response
func HttpLogin(params *LoginParams) (string, string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("POST", params.Endpoint, nil)
	if err != nil {
		return "", "", fmt.Errorf("unable to create request: %v", err)
	}

	if len(params.RefreshJwt) > 0 {
		req.Header.Add("X-Dgraph-RefreshJWT", params.RefreshJwt)
	} else {
		req.Header.Add("X-Dgraph-User", params.UserID)
		req.Header.Add("X-Dgraph-Password", params.Passwd)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("login through curl failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("unable to read from response: %v", err)
	}

	var outputJson map[string]map[string]string
	if err := json.Unmarshal(respBody, &outputJson); err != nil {
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

// GrootHttpLogin logins using the groot account with the default password
// and returns the access JWT
func GrootHttpLogin(endpoint string) string {
	accessJwt, _, err := HttpLogin(&LoginParams{
		Endpoint: endpoint,
		UserID:   x.GrootId,
		Passwd:   "password",
	})
	x.Check(err)
	return accessJwt
}

type CancelFunc func()

const DgraphAlphaPort = 9180

func GetDgraphClient() (*dgo.Dgraph, CancelFunc) {
	return GetDgraphClientOnPort(DgraphAlphaPort)
}

func GetDgraphClientOnPort(alphaPort int) (*dgo.Dgraph, CancelFunc) {
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", alphaPort), grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}

	dc := api.NewDgraphClient(conn)
	dg, cancel := dgo.NewDgraphClient(dc), func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error while closing connection:%v", err)
		}
	}

	ctx := context.Background()
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err = dg.Login(ctx, x.GrootId, "password")
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}
	return dg, cancel
}
