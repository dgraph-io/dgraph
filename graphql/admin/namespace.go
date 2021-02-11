/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

package admin

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

type namespaceInput struct {
	NamespaceId int
}

func resolveAddNamespace(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	var ns uint64
	var err error
	if ns, err = (&edgraph.Server{}).CreateNamespace(ctx); err != nil {
		return resolve.EmptyResult(m, err), false
	}
	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): map[string]interface{}{
			"namespaceId": json.Number(strconv.Itoa(int(ns))),
			"message":     "Created namespace successfully",
		}},
		nil,
	), true
}

func resolveDeleteNamespace(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	req, err := getNamespaceInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	if err = (&edgraph.Server{}).DeleteNamespace(ctx, uint64(req.NamespaceId)); err != nil {
		return resolve.EmptyResult(m, err), false
	}
	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): map[string]interface{}{
			"namespaceId": json.Number(strconv.Itoa(req.NamespaceId)),
			"message":     "Deleted namespace successfully",
		}},
		nil,
	), true
}

func getNamespaceInput(m schema.Mutation) (*namespaceInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input namespaceInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
