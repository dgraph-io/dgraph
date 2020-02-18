/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"fmt"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type drainingResolver struct {
	mutation schema.Mutation
	enable   bool
}

type drainingInput struct {
	Enable bool
}

func (dr *drainingResolver) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {
	glog.Info("Got draining request through GraphQL admin API")

	dr.mutation = m
	input, err := getDrainingInput(m)
	if err != nil {
		return nil, nil, err
	}

	dr.enable = input.Enable
	x.UpdateDrainingMode(input.Enable)
	return nil, nil, nil
}

func (dr *drainingResolver) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	return nil, nil
}

func (dr *drainingResolver) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string]interface{}, error) {

	return nil, nil, nil
}

func (dr *drainingResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	buf := writeResponse(dr.mutation, "Success",
		fmt.Sprintf("draining mode has been set to %v", dr.enable))
	return buf, nil
}

func getDrainingInput(m schema.Mutation) (*drainingInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input drainingInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
