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
	"os"
	"os/exec"
	"strconv"
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

// socket addr = IP address and port number
var (

	// SockAddr is the address to the gRPC endpoint of the alpha used during tests.
	SockAddr string
	// SockAddrHttp is the address to the HTTP of alpha used during tests.
	SockAddrHttp string
	// SockAddrZero is the address to the gRPC endpoint of the zero used during tests.
	SockAddrZero string
	// SockAddrZeroHttp is the address to the HTTP endpoint of the zero used during tests.
	SockAddrZeroHttp string
)

// This allows running (most) tests against dgraph running on the default ports, for example.
// Only the GRPC ports are needed and the others are deduced.
func init() {
	var grpcPort int

	getPort := func(envVar string, dfault int) int {
		p := os.Getenv(envVar)
		if p == "" {
			return dfault
		}
		port, _ := strconv.Atoi(p)
		return port
	}

	grpcPort = getPort("TEST_PORT_ALPHA", 9180)
	SockAddr = fmt.Sprintf("localhost:%d", grpcPort)
	SockAddrHttp = fmt.Sprintf("localhost:%d", grpcPort-1000)

	grpcPort = getPort("TEST_PORT_ZERO", 5180)
	SockAddrZero = fmt.Sprintf("localhost:%d", grpcPort)
	SockAddrZeroHttp = fmt.Sprintf("localhost:%d", grpcPort+1000)
}

// DgraphClientDropAll creates a Dgraph client and drops all existing data.
// It is intended to be called from TestMain() to establish a Dgraph connection shared
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

// DgraphClientWithGroot creates a Dgraph client with groot permissions set up.
// It is intended to be called from TestMain() to establish a Dgraph connection shared
// by all tests, so there is no testing.T instance for it to use.
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

// DgraphClient creates a Dgraph client.
// It is intended to be called from TestMain() to establish a Dgraph connection shared
// by all tests, so there is no testing.T instance for it to use.
func DgraphClient(serviceAddr string) *dgo.Dgraph {
	conn, err := grpc.Dial(serviceAddr, grpc.WithInsecure())
	x.CheckfNoTrace(err)

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	x.CheckfNoTrace(err)

	return dg
}

// DgraphClientWithCerts creates a Dgraph client with TLS configured using the given
// viper configuration.
// It is intended to be called from TestMain() to establish a Dgraph connection shared
// by all tests, so there is no testing.T instance for it to use.
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

// DropAll drops all the data in the Dgraph instance associated with the given client.
func DropAll(t *testing.T, dg *dgo.Dgraph) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	require.NoError(t, err)
}

// RetryQuery will retry a query until it succeeds or a non-retryable error is received.
func RetryQuery(dg *dgo.Dgraph, q string) (*api.Response, error) {
	for {
		resp, err := dg.NewTxn().Query(context.Background(), q)
		if err != nil && strings.Contains(err.Error(), "Please retry") {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		return resp, err
	}
}

// RetryMutation will retry a mutation until it succeeds or a non-retryable error is received.
// The mutation should have CommitNow set to true.
func RetryMutation(dg *dgo.Dgraph, mu *api.Mutation) error {
	for {
		_, err := dg.NewTxn().Mutate(context.Background(), mu)
		if err != nil && (strings.Contains(err.Error(), "Please retry") ||
			strings.Contains(err.Error(), "Tablet isn't being served by this instance")) {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		return err
	}
}

// LoginParams stores the information needed to perform a login request.
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
		var errOutputJson map[string]interface{}
		if err := json.Unmarshal(respBody, &errOutputJson); err == nil {
			if _, ok := errOutputJson["errors"]; ok {
				return "", "", fmt.Errorf("response error: %v", string(respBody))
			}
		}
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

type FailureConfig struct {
	ShouldFail   bool
	CurlErrMsg   string
	DgraphErrMsg string
}

type ErrorEntry struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Output struct {
	Data   map[string]interface{} `json:"data"`
	Errors []ErrorEntry           `json:"errors"`
}

func verifyOutput(t *testing.T, bytes []byte, failureConfig *FailureConfig) {
	output := Output{}
	require.NoError(t, json.Unmarshal(bytes, &output),
		"unable to unmarshal the curl output")

	if failureConfig.ShouldFail {
		require.True(t, len(output.Errors) > 0, "no error entry found")
		if len(failureConfig.DgraphErrMsg) > 0 {
			errorEntry := output.Errors[0]
			require.True(t, strings.Contains(errorEntry.Message, failureConfig.DgraphErrMsg),
				fmt.Sprintf("the failure msg\n%s\nis not part of the curl error output:%s\n",
					failureConfig.DgraphErrMsg, errorEntry.Message))
		}
	} else {
		require.True(t, len(output.Data) > 0,
			fmt.Sprintf("no data entry found in the output:%+v", output))
	}
}

// VerifyCurlCmd executes the curl command with the given arguments and verifies
// the result against the expected output.
func VerifyCurlCmd(t *testing.T, args []string,
	failureConfig *FailureConfig) {
	queryCmd := exec.Command("curl", args...)

	output, err := queryCmd.Output()
	if len(failureConfig.CurlErrMsg) > 0 {
		// the curl command should have returned an non-zero code
		require.Error(t, err, "the curl command should have failed")
		if ee, ok := err.(*exec.ExitError); ok {
			require.True(t, strings.Contains(string(ee.Stderr), failureConfig.CurlErrMsg),
				"the curl output does not contain the expected output")
		}
	} else {
		require.NoError(t, err, "the curl command should have succeeded")
		verifyOutput(t, output, failureConfig)
	}
}
