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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"google.golang.org/grpc"
)

// StateResponse represents the structure of the JSON object returned by calling
// the /state endpoint in zero.
type StateResponse struct {
	Groups map[string]struct {
		Members map[string]struct {
			Addr       string `json:"addr"`
			GroupID    int    `json:"groupId"`
			ID         string `json:"id"`
			LastUpdate string `json:"lastUpdate"`
			Leader     bool   `json:"leader"`
		} `json:"members"`
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
		return nil, fmt.Errorf("Failed to get state: %s", string(b))
	}

	var st StateResponse
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func GetClientToGroup(groupId string) (*dgo.Dgraph, error) {
	state, err := z.GetState()
	if err != nil {
		return nil, err
	}

	group, ok := state.Groups[groupId]
	if !ok {
		return nil, fmt.Errorf("group %s does not exist", groupId)
	}

	if len(group.Members) == 0 {
		return nil, fmt.Errorf("the group %s has no members", groupId)
	}

	member := group.Members["1"]
	parts := strings.Split(member.Addr, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("the member has an invalid address: %v", member.Addr)
	}
	// internalPort is used for communication between alpha nodes
	internalPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("unable to parse the port number from %s", parts[1])
	}

	// externalPort is for handling connections from clients
	externalPort := internalPort + 2000

	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", externalPort), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return dgo.NewDgraphClient(api.NewDgraphClient(conn)), nil
}
