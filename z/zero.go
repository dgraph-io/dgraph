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
)

// StateResponse represents the structure of the JSON object returned by calling
// the /state endpoint in zero.
type StateResponse struct {
	Groups map[string]struct {
		Members map[string]interface{} `json:"members"`
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
