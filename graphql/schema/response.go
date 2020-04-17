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

// Extensions represents GraphQL extensions
type Extensions struct {
	TouchedUids uint64 `json:"touched_uids,omitempty"`
}

// Merge merges ext with e
func (e *Extensions) Merge(ext *Extensions) {
	if e == nil || ext == nil {
		return
	}

	e.TouchedUids += ext.TouchedUids
}

// GraphQL spec on response is here:
// https://graphql.github.io/graphql-spec/June2018/#sec-Response

// GraphQL spec on errors is here:
// https://graphql.github.io/graphql-spec/June2018/#sec-Errors

// GraphQL spec on extensions says just this:
// The response map may also contain an entry with key extensions. This entry, if set, must have a
// map as its value. This entry is reserved for implementors to extend the protocol however they
// see fit, and hence there are no additional restrictions on its contents.

// Response represents a GraphQL response
type Response struct {
	Errors     x.GqlErrorList
	Data       bytes.Buffer
	Extensions *Extensions
}

// ErrorResponse formats an error as a list of GraphQL errors and builds
// a response with that error list and no data.  Because it doesn't add data, it
// should be used before starting execution - GraphQL spec requires no data if an
// error is detected before execution begins.
func ErrorResponse(err error) *Response {
	return &Response{
		Errors: AsGQLErrors(err),
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

	if r.Data.Len() == 0 {
		x.Check2(r.Data.Write(p))
		return
	}

	// The end of the buffer is always the closing `}`
	r.Data.Truncate(r.Data.Len() - 1)
	x.Check2(r.Data.WriteRune(','))

	x.Check2(r.Data.Write(p[1 : len(p)-1]))
	x.Check2(r.Data.WriteRune('}'))
}

// MergeExtensions merges the extensions given in ext to r.
// If r.Extensions is nil before the call, then r.Extensions becomes ext.
// Otherwise, r.Extensions gets merged with ext.
func (r *Response) MergeExtensions(ext *Extensions) {
	if r == nil {
		return
	}

	if r.Extensions == nil {
		r.Extensions = ext
		return
	}

	r.Extensions.Merge(ext)
}

// WriteTo writes the GraphQL response as unindented JSON to w
// and returns the number of bytes written and error, if any.
func (r *Response) WriteTo(w io.Writer) (int64, error) {
	js, err := json.Marshal(r.Output())

	if err != nil {
		msg := "Internal error - failed to marshal a valid JSON response"
		glog.Errorf("%+v", errors.Wrap(err, msg))
		js = []byte(fmt.Sprintf(
			`{ "errors": [{"message": "%s"}], "data": null }`, msg))
	}

	i, err := w.Write(js)
	return int64(i), err
}

// Output returns json interface of the response
func (r *Response) Output() interface{} {
	if r == nil {
		return struct {
			Errors json.RawMessage `json:"errors,omitempty"`
			Data   json.RawMessage `json:"data,omitempty"`
		}{
			Errors: []byte(`[{"message": "Internal error - no response to write."}]`),
			Data:   []byte("null"),
		}
	}
	return struct {
		Errors     []*x.GqlError   `json:"errors,omitempty"`
		Data       json.RawMessage `json:"data,omitempty"`
		Extensions *Extensions     `json:"extensions,omitempty"`
	}{
		Errors:     r.Errors,
		Data:       r.Data.Bytes(),
		Extensions: r.Extensions,
	}
}
