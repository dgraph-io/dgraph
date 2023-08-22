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
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
)

func (hc *HTTPClient) AddNamespace() (uint64, error) {
	const createNs = `mutation {
		addNamespace {
			namespaceId
			message
		}
	}`

	params := GraphQLParams{Query: createNs}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return 0, err
	}

	var result struct {
		AddNamespace struct {
			NamespaceId uint64 `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, errors.Wrap(err, "error unmarshalling response")
	}
	if strings.Contains(result.AddNamespace.Message, "Created namespace successfully") {
		return result.AddNamespace.NamespaceId, nil
	}
	return 0, errors.New(result.AddNamespace.Message)
}

func (hc *HTTPClient) DeleteNamespace(nsID uint64) (uint64, error) {
	const deleteReq = `mutation deleteNamespace($namespaceId: Int!) {
		deleteNamespace(input: {namespaceId: $namespaceId}) {
			namespaceId
			message
		}
	}`

	params := GraphQLParams{
		Query:     deleteReq,
		Variables: map[string]interface{}{"namespaceId": nsID},
	}
	resp, err := hc.RunGraphqlQuery(params, true)
	if err != nil {
		return 0, err
	}

	var result struct {
		DeleteNamespace struct {
			NamespaceId uint64 `json:"namespaceId"`
			Message     string `json:"message"`
		}
	}

	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, errors.Wrap(err, "error unmarshalling CreateNamespaceWithRetry() response")
	}
	if strings.Contains(result.DeleteNamespace.Message, "Deleted namespace successfully") {
		return result.DeleteNamespace.NamespaceId, nil
	}
	return 0, errors.New(result.DeleteNamespace.Message)
}
