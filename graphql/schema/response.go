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
	"fmt"
	"io"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// GraphQL spec on response is here:
// https://graphql.github.io/graphql-spec/June2018/#sec-Response

// GraphQL spec on errors is here:
// https://graphql.github.io/graphql-spec/June2018/#sec-Errors

// Response represents a GraphQL response
type Response struct {
	Errors     x.GqlErrorList
	Data       bytes.Buffer
	Extensions *Extensions
}

// Extensions : GraphQL specifies allowing "extensions" in results, but the
// format is up to the implementation.
type Extensions struct {
	RequestID string `json:"requestID,omitempty"`
}

// ErrorResponse formats an error as a list of GraphQL errors and builds
// a response with that error list and no data.  Because it doesn't add data, it
// should be used before starting execution - GraphQL spec requires no data if an
// error is detected before execution begins.
func ErrorResponse(err error, requestID string) *Response {
	return &Response{
		Errors:     AsGQLErrors(err),
		Extensions: &Extensions{RequestID: requestID},
	}
}

// WithError generates GraphQL errors from err and records those in r.
func (r *Response) WithError(err error) {
	r.Errors = append(r.Errors, AsGQLErrors(err)...)
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
		x.Check2(r.Data.WriteRune(','))
	}

	if r.Data.Len() == 0 {
		x.Check2(r.Data.WriteRune('{'))
	}

	x.Check2(r.Data.Write(p))
	x.Check2(r.Data.WriteRune('}'))
}

// WriteTo writes the GraphQL response as unindented JSON to w
// and returns the number of bytes written and error, if any.
func (r *Response) WriteTo(w io.Writer) (int64, error) {
	if r == nil {
		i, err := w.Write([]byte(
			`{ "errors": [{"message": "Internal error - no response to write."}], ` +
				` "data": null, "extensions": { "requestID": "unknown request ID" } }`))
		return int64(i), err
	}

	js, err := json.Marshal(struct {
		Errors     []*x.GqlError   `json:"errors,omitempty"`
		Data       json.RawMessage `json:"data,omitempty"`
		Extensions *Extensions     `json:"extensions,omitempty"`
	}{
		Errors:     r.Errors,
		Data:       r.Data.Bytes(),
		Extensions: r.Extensions,
	})

	if err != nil {
		var reqID string
		if r.Extensions != nil {
			reqID = r.Extensions.RequestID
		} else {
			reqID = "unknown request ID"
		}
		msg := "Internal error - failed to marshal a valid JSON response"
		glog.Errorf("[%s] %+v", reqID, errors.Wrap(err, msg))
		js = []byte(fmt.Sprintf(
			`{ "errors": [{"message": "%s"}], "data": null, "extensions": { "requestID": "%s" } }`,
			msg, reqID))
	}

	i, err := w.Write(js)
	return int64(i), err
}
