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

package dgraphtest

import (
	"github.com/dgraph-io/dgo/v240"
	"github.com/dgraph-io/dgraph/v24/dgraphapi"
	"github.com/dgraph-io/dgraph/v24/testutil"
)

type ComposeCluster struct{}

func NewComposeCluster() *ComposeCluster {
	return &ComposeCluster{}
}

func (c *ComposeCluster) Client() (*dgraphapi.GrpcClient, func(), error) {
	client, err := testutil.DgraphClient(testutil.SockAddr)
	if err != nil {
		return nil, nil, err
	}

	return &dgraphapi.GrpcClient{Dgraph: client}, func() {}, nil
}

// HTTPClient creates an HTTP client
func (c *ComposeCluster) HTTPClient() (*dgraphapi.HTTPClient, error) {
	httpClient, err := dgraphapi.GetHttpClient(testutil.SockAddrHttp, testutil.SockAddrZeroHttp)
	if err != nil {
		return nil, err
	}
	httpClient.HttpToken = &dgraphapi.HttpToken{}
	return httpClient, nil
}

func (c *ComposeCluster) AlphasHealth() ([]string, error) {
	return nil, errNotImplemented
}

func (c *ComposeCluster) AlphasLogs() ([]string, error) {
	return nil, errNotImplemented
}

func (c *ComposeCluster) AssignUids(client *dgo.Dgraph, num uint64) error {
	return testutil.AssignUids(num)
}

func (c *ComposeCluster) GetVersion() string {
	return localVersion
}

func (c *ComposeCluster) GetEncKeyPath() (string, error) {
	return "", errNotImplemented
}

// GetRepoDir returns the repositroty directory of the cluster
func (c *ComposeCluster) GetRepoDir() (string, error) {
	return "", errNotImplemented
}
