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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// DgraphClient is intended to be called from TestMain() to establish a Dgraph connection shared
// by all tests, so there is no testing.T instance for it to use.
func DgraphClientDropAll(serviceAddr string) *dgo.Dgraph {
	dg := DgraphClient(serviceAddr)
	var err error
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}
	x.CheckfNoTrace(err)

	return dg
}

func DgraphClientWithGroot(serviceAddr string) *dgo.Dgraph {
	dg := DgraphClient(serviceAddr)

	var err error
	ctx := context.Background()
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err = dg.Login(ctx, x.GrootId, "password")
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}
	x.CheckfNoTrace(err)

	return dg
}

// DgraphClientNoDropAll is intended to be called from TestMain() to establish a Dgraph connection
// shared by all tests, so there is no testing.T instance for it to use.
func DgraphClient(serviceAddr string) *dgo.Dgraph {
	conn, err := grpc.Dial(serviceAddr, grpc.WithInsecure())
	x.CheckfNoTrace(err)

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	x.CheckfNoTrace(err)

	return dg
}

func DgraphClientWithCerts(serviceAddr string, conf *viper.Viper) (*dgo.Dgraph, error) {
	tlsCfg, err := x.LoadClientTLSConfig(conf)
	if err != nil {
		return nil, err
	}

	dialOpts := []grpc.DialOption{}
	if tlsCfg != nil {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	}
	conn, err := grpc.Dial(serviceAddr, dialOpts...)
	if err != nil {
		return nil, err
	}
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	return dg, nil
}

func DropAll(t *testing.T, dg *dgo.Dgraph) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err)

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
	require.NoError(t, err)

	type count struct {
		Count int
	}
	type root struct {
		Q []count
	}
	var response root
	err = json.Unmarshal(resp.GetJson(), &response)
	require.NoError(t, err)

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
	loginPayload := api.LoginRequest{}
	if len(params.RefreshJwt) > 0 {
		loginPayload.RefreshToken = params.RefreshJwt
	} else {
		loginPayload.Userid = params.UserID
		loginPayload.Password = params.Passwd
	}

	body, err := json.Marshal(&loginPayload)
	if err != nil {
		return "", "", fmt.Errorf("unable to marshal body: %v", err)
	}

	req, err := http.NewRequest("POST", params.Endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", "", fmt.Errorf("unable to create request: %v", err)
	}

	client := &http.Client{}
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
func GrootHttpLogin(endpoint string) (string, string) {
	accessJwt, refreshJwt, err := HttpLogin(&LoginParams{
		Endpoint: endpoint,
		UserID:   x.GrootId,
		Passwd:   "password",
	})
	x.Check(err)
	return accessJwt, refreshJwt
}
