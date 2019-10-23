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
	"fmt"

	"github.com/dgraph-io/dgraph/gql"
)

const (
	errNoConnection healthStatus = "ErrNoConnection"
	noGraphQLSchema healthStatus = "NoGraphQLSchema"
	healthy         healthStatus = "Healthy"
)

type healthStatus string

type healthResolver struct {
	status healthStatus
}

var statusMessage = map[healthStatus]string{
	errNoConnection: "Unable to contact Dgraph",
	noGraphQLSchema: "Dgraph connection established but there's no GraphQL schema.",
	healthy:         "Dgraph connection established and serving GraphQL schema.",
}

func (hr *healthResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	s := hr.status
	return []byte(fmt.Sprintf(`{"health":[{"message":"%s","status":"%s"}]}`, statusMessage[s], string(s))), nil
}
