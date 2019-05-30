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

package graphql

import (
	"encoding/json"
	"fmt"

	"github.com/vektah/gqlparser/gqlerror"
)

// complete error format yet to be decided.
// GraphQL spec on errors is here https://graphql.github.io/graphql-spec/June2018/#sec-Errors
//

type GraphQLResponse struct {
	Errors gqlerror.List   `json:"errors,omitempty"`
	Data   json.RawMessage `json:"data"`
}

func ErrorResponse(messagef string, args ...interface{}) *GraphQLResponse {
	return &GraphQLResponse{
		Errors: gqlerror.List{{Message: fmt.Sprintf(messagef, args...)}},
	}
}
