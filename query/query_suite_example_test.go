//go:build integration || all

/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/matthewmcneely/dockertest/v3"
	"github.com/matthewmcneely/dockertest/v3/docker"
	"github.com/pkg/errors"

	//_ "github.com/stretchr/testify/suite" // Needed for VSCode to recognize individual TestXXX's
	"github.com/stretchr/testify/suite"
)

type QuerySuite struct {
	suite.Suite
	client *dgo.Dgraph

	pool    *dockertest.Pool
	network *docker.Network
	alpha   *dockertest.Resource
	zero    *dockertest.Resource
}

func (s *QuerySuite) SetupSuite() {

	var err error
	s.pool, err = dockertest.NewPool(os.Getenv("DOCKER_URL"))
	if err != nil {
		s.Fail("Could not connect to docker: %s", err)
	}
	s.network, err = s.pool.Client.CreateNetwork(docker.CreateNetworkOptions{
		Name: "dgraph-internal",
	})
	s.Require().NoError(err)

	s.zero, err = s.pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   "dgraph/dgraph",
		Tag:          "local",
		NetworkID:    s.network.ID,
		Hostname:     "zero",
		Name:         "zero",
		ExposedPorts: []string{"6080"},
		Cmd:          []string{"dgraph", "zero", "--my=zero:5080"},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(s.zero.GetPort("6080/tcp"))

	s.alpha, err = s.pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "dgraph/dgraph",
		Tag:        "local",
		NetworkID:  s.network.ID,
		Hostname:   "alpha",
		Name:       "alpha",
		Cmd: []string{
			"dgraph",
			"alpha",
			"--my=alpha:7080",
			"--zero=zero:5080",
			"--security",
			"whitelist=0.0.0.0/0",
			"--profile_mode",
			"block",
			"--block_rate",
			"10",
		},
	}, func(hc *docker.HostConfig) {
		hc.AutoRemove = true
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(s.alpha.GetPort("8080/tcp"))
	s.Require().NotEmpty(s.alpha.GetPort("9080/tcp"))

	grpcAddr := fmt.Sprintf("localhost:%s", s.alpha.GetPort("9080/tcp"))
	// hack, set the package global client var here
	client, err = testutil.DgraphClient(grpcAddr)
	s.Require().NoError(err)
	// hack, set the package global zero admin http endpoint here
	testutil.SockAddrZeroHttp = fmt.Sprintf("localhost:%s", s.zero.GetPort("6080/tcp"))

	var health []struct {
		Status string `json:"status"`
	}

	err = s.pool.Retry(func() error {
		resp, err := http.Get("http://localhost:" + s.alpha.GetPort("8080/tcp") + "/health")
		if err != nil {
			return err
		}
		err = json.NewDecoder(resp.Body).Decode(&health)
		if err != nil {
			return err
		}
		if len(health) != 1 {
			return errors.New("unexpected health record count")
		}
		if health[0].Status != "healthy" {
			return errors.New("status not healthy")
		}
		return nil
	})
	fmt.Println("Cluster ready...")
	s.Require().NoError(err)

	populateCluster()
}

func (s *QuerySuite) TearDownSuite() {
	if s.pool != nil {
		s.NoError(s.pool.Client.StopContainer(s.alpha.Container.ID, 0))
		s.NoError(s.pool.Client.StopContainer(s.zero.Container.ID, 0))
	}
	if s.network != nil {
		s.NoError(s.pool.Client.RemoveNetwork(s.network.ID))
	}
}

func TestQuerySuite(t *testing.T) {
	s := new(QuerySuite)
	suite.Run(t, s)
}

// from root, go test github.com/dgraph-io/dgraph/query -run ^TestQuerySuite$ -testify.m ^\(TestGetUID\)$ -race -tags integration -count=1 -v
func (s *QuerySuite) TestGetUID() {
	query := `
		 {
			 me(func: uid(0x01)) {
				 name
				 uid
				 gender
				 alive
				 friend {
					 uid
					 name
				 }
			 }
		 }
	 `
	js := processQueryNoErr(s.T(), query)
	s.Require().JSONEq(`{"data": {"me":[{"uid":"0x1","alive":true,"friend":[{"uid":"0x17","name":"Rick Grimes"},{"uid":"0x18","name":"Glenn Rhee"},{"uid":"0x19","name":"Daryl Dixon"},{"uid":"0x1f","name":"Andrea"},{"uid":"0x65"}],"gender":"female","name":"Michonne"}]}}`,
		js)
}

func (s *QuerySuite) TestQueryEmptyDefaultNames() {
	query := `{
	   people(func: eq(name, "")) {
		 uid
		 name
	   }
	 }`
	js := processQueryNoErr(s.T(), query)
	// only two empty names should be retrieved as the other one is empty in a particular lang.
	s.Require().JSONEq(`{"data":{"people": [{"uid":"0xdac","name":""}, {"uid":"0xdae","name":""}]}}`,
		js)
}

func (s *QuerySuite) TestQueryEmptyDefaultNameWithLanguage() {
	query := `{
	   people(func: eq(name, "")) {
		 name@ko:en:hi
	   }
	 }`
	js := processQueryNoErr(s.T(), query)
	s.Require().JSONEq(`{"data":{"people": [{"name@ko:en:hi":"상현"},{"name@ko:en:hi":"Amit"}]}}`,
		js)
}
