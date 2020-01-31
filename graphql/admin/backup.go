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

package admin

import (
	"context"
	"encoding/json"
	"fmt"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/golang/glog"
)

type backupResolver struct {
	// admin *adminServer

	// mutation schema.Mutation
}

type backupInput struct {
	Destination  string
	AccessKey    string
	SecretKey    string
	SessionToken string
	Anonymous    bool
	ForceFull    bool
}

func (br *backupResolver) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {

	glog.Info("Got backup request")

	input, err := getBackupInput(m)
	if err != nil {
		return nil, nil, err
	}
	fmt.Printf("input: %+v\n", input)

	// Return error if request fails here.
	return nil, nil, nil
}

func (br *backupResolver) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	return nil, nil
}

func (br *backupResolver) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string]interface{}, error) {

	return nil, nil, nil
}

func (br *backupResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	return nil, nil
}

func getBackupInput(m schema.Mutation) (*backupInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input backupInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
