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
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type Member struct {
	Addr       string `json:"addr"`
	GroupID    int    `json:"groupId"`
	ID         string `json:"id"`
	LastUpdate string `json:"lastUpdate"`
	Leader     bool   `json:"leader"`
}

// StateResponse represents the structure of the JSON object returned by calling
// the /state endpoint in zero.
type StateResponse struct {
	Zeros map[string]struct {
		Id string `json:"id"`
	} `json:"zeros"`
	Groups map[string]struct {
		Members map[string]Member `json:"members"`
		Tablets map[string]struct {
			GroupID   int    `json:"groupId"`
			Predicate string `json:"predicate"`
		} `json:"tablets"`
	} `json:"groups"`
	Removed []struct {
		Addr    string `json:"addr"`
		GroupID int    `json:"groupId"`
		ID      string `json:"id"`
	} `json:"removed"`
}

// GetState queries the /state endpoint in zero and returns the response.
func GetState() (*StateResponse, error) {
	resp, err := http.Get("http://" + SockAddrZeroHttp + "/state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if bytes.Contains(b, []byte("Error")) {
		return nil, errors.Errorf("Failed to get state: %s", string(b))
	}

	var st StateResponse
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// GetStateHttps queries the /state endpoint in zero and returns the response.
func GetStateHttps(tlsConfig *tls.Config) (*StateResponse, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	resp, err := client.Get("https://" + SockAddrZeroHttp + "/state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if bytes.Contains(b, []byte("Error")) {
		return nil, errors.Errorf("Failed to get state: %s", string(b))
	}

	var st StateResponse
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// GetClientToGroup returns a dgraph client connected to an alpha in the given group.
func GetClientToGroup(gid string) (*dgo.Dgraph, error) {
	state, err := GetState()
	if err != nil {
		return nil, err
	}

	group, ok := state.Groups[gid]
	if !ok {
		return nil, errors.Errorf("group %s does not exist", gid)
	}

	if len(group.Members) == 0 {
		return nil, errors.Errorf("the group %s has no members", gid)
	}

	// Select the first member found in the iteration.
	var member Member
	for _, m := range group.Members {
		member = m
		break
	}
	parts := strings.Split(member.Addr, ":")
	if len(parts) != 2 {
		return nil, errors.Errorf("the member has an invalid address: %v", member.Addr)
	}

	addr := ContainerAddr(parts[0], 9080)
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return dgo.NewDgraphClient(api.NewDgraphClient(conn)), nil
}

func GetNodesInGroup(gid string) ([]string, error) {
	state, err := GetState()
	if err != nil {
		return nil, err
	}

	group, ok := state.Groups[gid]
	if !ok {
		return nil, errors.Errorf("group %s does not exist", gid)
	}

	if len(group.Members) == 0 {
		return nil, errors.Errorf("the group %s has no members", gid)
	}

	nodes := make([]string, 0)
	for id := range group.Members {
		nodes = append(nodes, id)
	}
	return nodes, nil
}
