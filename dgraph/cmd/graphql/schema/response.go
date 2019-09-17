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

package schema

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/vektah/gqlparser/gqlerror"
)

// GraphQL spec on response is here:
// https://graphql.github.io/graphql-spec/June2018/#sec-Response

// GraphQL spec on errors is here:
// https://graphql.github.io/graphql-spec/June2018/#sec-Errors

// Response represents a GraphQL response
type Response struct {
	// Dgraph response type (x.go) is similar, should I be leaning on that?
	// ATM, no, cause I'm trying to follow the spec really closely, e.g:
	// - spec error format is different to x.errRes
	// - I think we should mostly return 200 status code
	// - for spec we need to return errors and data in same response

	Errors     gqlerror.List
	Data       bytes.Buffer
	Extensions *Extensions
}

// Extensions : GraphQL specifies allowing "extensions" in results, but the
// format is up to the implementation.
type Extensions struct {
	RequestID string `json:"requestID,omitempty"`
}

// ErrorResponsef returns a Response containing a single GraphQL error with a
// message obtained by Sprintf-ing the argugments
func ErrorResponsef(format string, args ...interface{}) *Response {
	return &Response{
		Errors: gqlerror.List{gqlerror.Errorf(format, args...)},
	}
}

// ErrorResponse formats an error as a list of GraphQL errors and builds
// a response with that error list and no data.
func ErrorResponse(err error) *Response {
	return &Response{
		Errors: AsGQLErrors(err),
	}
}

// AddData adds p to r's data buffer.  If p is empty, the call has no effect.
// If r.Data is empty before the call, then r.Data becomes {p}
// If r.Data contains data it always looks like {f,g,...}, and
// adding to that results in {f,g,...,p}
func (r *Response) AddData(p []byte) {
	if r == nil || len(p) == 0 {
		return
	}

	if r.Data.Len() > 0 {
		// The end of the buffer is always the closing `}`
		r.Data.Truncate(r.Data.Len() - 1)
		r.Data.WriteRune(',')
	}

	if r.Data.Len() == 0 {
		r.Data.WriteRune('{')
	}

	r.Data.Write(p)
	r.Data.WriteRune('}')
}

// WriteTo writes the GraphQL response as unindented JSON to w
// and returns the number of bytes written and error, if any.
func (r *Response) WriteTo(w io.Writer) (int64, error) {
	if r == nil {
		i, err := w.Write([]byte(
			`{ "errors": [ { "message": "Internal error - no response to write." } ], ` +
				` "data": null }`))
		return int64(i), err
	}

	js, err := json.Marshal(struct {
		Errors     gqlerror.List   `json:"errors,omitempty"`
		Data       json.RawMessage `json:"data,omitempty"`
		Extensions *Extensions     `json:"extensions,omitempty"`
	}{
		Errors:     r.Errors,
		Data:       r.Data.Bytes(),
		Extensions: r.Extensions,
	})

	if err != nil {
		msg := "Internal error - failed to marshal a valid JSON response"
		glog.Errorf("%+v", errors.Wrap(err, msg))
		js = []byte(`{ "errors": [ { "message": "` + msg + `" } ], "data": null }`)
	}

	i, err := w.Write(js)
	return int64(i), err
}
