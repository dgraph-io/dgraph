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
	"bytes"
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/pkg/errors"
)

type ComposeCluster struct{}

func NewComposeCluster() *ComposeCluster {
	return &ComposeCluster{}
}

func (c *ComposeCluster) Client() (*dgo.Dgraph, error) {
	return nil, errNotImplemented
}

func (c *ComposeCluster) AdminPost(body []byte, cred *Credential) ([]byte, error) {
	url := "http://" + testutil.SockAddrHttp + "/admin"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, errors.Wrapf(err, "error building req for endpoint [%v]", url)
	}
	req.Header.Add("Content-Type", "application/json")

	// assuming ACL is enabled
	token, err := httpLogin(url, cred)
	if err != nil {
		return nil, err
	}
	req.Header.Add("X-Dgraph-AccessToken", token)

	return doReq(req)
}

func (c *ComposeCluster) AlphasHealth() ([]string, error) {
	return nil, errNotImplemented
}

func (c *ComposeCluster) AssignUids(num uint64) error {
	return testutil.AssignUids(num)
}

func (c *ComposeCluster) GetVersion() string {
	return localVersion
}

func (c *ComposeCluster) MakeGQLRequestWithAccessJwtAndTLS(t *testing.T, params *testutil.GraphQLParams,
	tls *tls.Config, accessToken, adminUrl string) *testutil.GraphQLResponse {
	return testutil.MakeGQLRequestWithAccessJwtAndTLS(t, params, tls, accessToken)
}
