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

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// socket addr = IP address and port number
var (
	// Instance is the instance name of the Alpha.
	DockerPrefix  string
	Instance      string
	MinioInstance string
	// SockAddr is the address to the gRPC endpoint of the alpha used during tests.
	SockAddr string
	// SockAddrHttp is the address to the HTTP of alpha used during tests.
	SockAddrHttp string
	// SockAddrZero is the address to the gRPC endpoint of the zero used during tests.
	SockAddrZero string
	// SockAddrZeroHttp is the address to the HTTP endpoint of the zero used during tests.
	SockAddrZeroHttp string
)

func AdminUrl() string {
	return "http://" + SockAddrHttp + "/admin"
}

func getContainer(name string) types.Container {
	cli, err := client.NewEnvClient()
	x.Check(err)

	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		log.Fatalf("While listing container: %v\n", err)
	}

	q := fmt.Sprintf("/%s_%s_", DockerPrefix, name)
	for _, c := range containers {
		for _, n := range c.Names {
			if !strings.HasPrefix(n, q) {
				continue
			}
			return c
		}
	}
	return types.Container{}
}

func ContainerAddr(name string, privatePort uint16) string {
	c := getContainer(name)
	for _, p := range c.Ports {
		if p.PrivatePort == privatePort {
			return "localhost:" + strconv.Itoa(int(p.PublicPort))
		}
	}
	return "localhost:" + strconv.Itoa(int(privatePort))
}

// This allows running (most) tests against dgraph running on the default ports, for example.
// Only the GRPC ports are needed and the others are deduced.
func init() {
	DockerPrefix = os.Getenv("TEST_DOCKER_PREFIX")

	MinioInstance = ContainerAddr("minio", 9001)
	Instance = fmt.Sprintf("%s_%s_1", DockerPrefix, "alpha1")
	SockAddr = ContainerAddr("alpha1", 9080)
	SockAddrHttp = ContainerAddr("alpha1", 8080)

	SockAddrZero = ContainerAddr("zero1", 5080)
	SockAddrZeroHttp = ContainerAddr("zero1", 6080)

	fmt.Printf("testutil: %q %s %s\n", DockerPrefix, SockAddr, SockAddrZero)
}

// DgraphClientDropAll creates a Dgraph client and drops all existing data.
// It is intended to be called from TestMain() to establish a Dgraph connection shared
// by all tests, so there is no testing.T instance for it to use.
func DgraphClientDropAll(serviceAddr string) (*dgo.Dgraph, error) {
	dg, err := DgraphClient(serviceAddr)
	if err != nil {
		return nil, err
	}

	for {
		// keep retrying until we succeed or receive a non-retriable error
		err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}

	return dg, err
}

// DgraphClientWithGroot creates a Dgraph client with groot permissions set up.
// It is intended to be called from TestMain() to establish a Dgraph connection shared
// by all tests, so there is no testing.T instance for it to use.
func DgraphClientWithGroot(serviceAddr string) (*dgo.Dgraph, error) {
	dg, err := DgraphClient(serviceAddr)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err = dg.Login(ctx, x.GrootId, "password")
		if err == nil || !(strings.Contains(err.Error(), "Please retry") ||
			strings.Contains(err.Error(), "user not found")) {

			break
		}
		time.Sleep(time.Second)
	}

	return dg, err
}

// DgraphClient creates a Dgraph client.
// It is intended to be called from TestMain() to establish a Dgraph connection shared
// by all tests, so there is no testing.T instance for it to use.
func DgraphClient(serviceAddr string) (*dgo.Dgraph, error) {
	conn, err := grpc.Dial(serviceAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	return dg, nil
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

// VerifyQueryResponse executes the given query and verifies that the response of the query is
// same as the expected response.
func VerifyQueryResponse(t *testing.T, dg *dgo.Dgraph, query, expectedResponse string) {
	resp, err := dg.NewReadOnlyTxn().Query(context.Background(), query)
	require.NoError(t, err)
	CompareJSON(t, expectedResponse, string(resp.GetJson()))
}

// RetryQuery will retry a query until it succeeds or a non-retryable error is received.
func RetryQuery(dg *dgo.Dgraph, q string) (*api.Response, error) {
	for {
		resp, err := dg.NewTxn().Query(context.Background(), q)
		if err != nil && (strings.Contains(err.Error(), "Please retry") ||
			strings.Contains(err.Error(), "connection closed") ||
			strings.Contains(err.Error(), "broken pipe")) {
			// Retry connection issues because some tests (e.g TestSnapshot) are stopping and
			// starting alphas.
			time.Sleep(10 * time.Millisecond)
			continue
		}

		return resp, err
	}
}

// RetryBadQuery will retry a query until it failse with a non-retryable error.
func RetryBadQuery(dg *dgo.Dgraph, q string) (*api.Response, error) {
	for {
		txn := dg.NewTxn()
		ctx := context.Background()
		resp, err := txn.Query(ctx, q)
		if err == nil || strings.Contains(err.Error(), "Please retry") {
			time.Sleep(10 * time.Millisecond)
			_ = txn.Discard(ctx)
			continue
		}

		_ = txn.Discard(ctx)
		return resp, err
	}
}

// RetryMutation will retry a mutation until it succeeds or a non-retryable error is received.
// The mutation should have CommitNow set to true.
func RetryMutation(dg *dgo.Dgraph, mu *api.Mutation) error {
	for {
		_, err := dg.NewTxn().Mutate(context.Background(), mu)
		if err != nil && (strings.Contains(err.Error(), "Please retry") ||
			strings.Contains(err.Error(), "Tablet isn't being served by this instance") ||
			strings.Contains(err.Error(), "connection closed")) {
			// Retry connection issues because some tests (e.g TestSnapshot) are stopping and
			// starting alphas.
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

	login := `mutation login($userId: String, $password: String, $refreshToken: String) {
		login(userId: $userId, password: $password, refreshToken: $refreshToken) {
			response {
				accessJWT
				refreshJWT
			}
		}
	}`

	gqlParams := GraphQLParams{
		Query: login,
		Variables: map[string]interface{}{
			"userId":       params.UserID,
			"password":     params.Passwd,
			"refreshToken": params.RefreshJwt,
		},
	}
	body, err := json.Marshal(gqlParams)
	if err != nil {
		return "", "", errors.Wrapf(err, "unable to marshal body")
	}

	req, err := http.NewRequest("POST", params.Endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", "", errors.Wrapf(err, "unable to create request")
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", errors.Wrapf(err, "login through curl failed")
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", errors.Wrapf(err, "unable to read from response")
	}

	var outputJson map[string]interface{}
	if err := json.Unmarshal(respBody, &outputJson); err != nil {
		var errOutputJson map[string]interface{}
		if err := json.Unmarshal(respBody, &errOutputJson); err == nil {
			if _, ok := errOutputJson["errors"]; ok {
				return "", "", errors.Errorf("response error: %v", string(respBody))
			}
		}
		return "", "", errors.Wrapf(err, "unable to unmarshal the output to get JWTs")
	}

	data, found := outputJson["data"].(map[string]interface{})
	if !found {
		return "", "", errors.Wrapf(err, "data entry found in the output")
	}

	l, found := data["login"].(map[string]interface{})
	if !found {
		return "", "", errors.Wrapf(err, "data entry found in the output")
	}

	response, found := l["response"].(map[string]interface{})
	if !found {
		return "", "", errors.Wrapf(err, "data entry found in the output")
	}

	newAccessJwt, found := response["accessJWT"].(string)
	if !found || newAccessJwt == "" {
		return "", "", errors.Errorf("no access JWT found in the output")
	}
	newRefreshJwt, found := response["refreshJWT"].(string)
	if !found || newRefreshJwt == "" {
		return "", "", errors.Errorf("no refresh JWT found in the output")
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

// CurlFailureConfig stores information about the expected failure of a curl test.
type CurlFailureConfig struct {
	ShouldFail   bool
	CurlErrMsg   string
	DgraphErrMsg string
}

type curlErrorEntry struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type curlOutput struct {
	Data   map[string]interface{} `json:"data"`
	Errors []curlErrorEntry       `json:"errors"`
}

func verifyOutput(t *testing.T, bytes []byte, failureConfig *CurlFailureConfig) {
	output := curlOutput{}
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
		require.True(t, output.Data != nil,
			fmt.Sprintf("no data entry found in the output:%+v", output))
	}
}

// VerifyCurlCmd executes the curl command with the given arguments and verifies
// the result against the expected output.
func VerifyCurlCmd(t *testing.T, args []string, failureConfig *CurlFailureConfig) {
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

// AssignUids talks to zero to assign the given number of uids.
func AssignUids(num uint64) error {
	_, err := http.Get(fmt.Sprintf("http://"+SockAddrZeroHttp+"/assign?what=uids&num=%d", num))
	return err
}
