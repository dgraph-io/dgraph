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

// FIXME:
// based on https://github.com/99designs/gqlgen/blob/1617ff28daba04a67413ba9696c7650e718aa080/graphql/response.go#L14
//
// complete error format yet to be decided.
// GraphQL spec on errors is here https://graphql.github.io/graphql-spec/June2018/#sec-Errors
//

// Errors are intentionally serialized first based on the advice in
// https://github.com/facebook/graphql/commit/7b40390d48680b15cb93e02d46ac5eb249689876#diff-757cea6edf0288677a9eea4cfc801d87R107
// and https://github.com/facebook/graphql/pull/384

type Response struct {
	Errors gqlerror.List   `json:"errors,omitempty"`
	Data   json.RawMessage `json:"data"`
}

func ErrorResponse(messagef string, args ...interface{}) *Response {
	return &Response{
		Errors: gqlerror.List{{Message: fmt.Sprintf(messagef, args...)}},
	}
}
