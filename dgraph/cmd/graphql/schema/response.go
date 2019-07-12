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

// GraphQL spec on errors is here:
// https://graphql.github.io/graphql-spec/June2018/#sec-Errors

// Response represents a GraphQL response
type Response struct {
	// Dgraph response type (x.go) is similar, should I be leaning on that?
	// ATM, no, cause I'm trying to follow the spec really closely, e.g:
	// - spec error format is different to x.errRes
	// - I think we should mostly return 200 status code
	// - for spec we need to return errors and data in same response
	//
	// see https://graphql.github.io/graphql-spec/June2018/#sec-Response
	//
	Errors gqlerror.List
	Data   bytes.Buffer
}

// ErrorResponsef returns a Response containing a single GraphQL error with a
// message obtained by Sprintf-ing the argugments
func ErrorResponsef(format string, args ...interface{}) *Response {
	return &Response{
		Errors: gqlerror.List{gqlerror.Errorf(format, args...)},
	}
}

// WithNullData sets the data response of r such that subsequent calls
// to r.WriteTo will write `"data": null`
func (r *Response) WithNullData() {
	r.Data.Reset()
	r.Data.WriteString(`null`)
}

// WriteTo writes the GraphQL response as unindented JSON to w
// and returns the number of bytes written and error, if any.
func (r *Response) WriteTo(w io.Writer) (int64, error) {
	var out bytes.Buffer

	out.WriteRune('{')
	if len(r.Errors) > 0 {
		js, err := json.Marshal(r.Errors)
		if err != nil {
			msg := "Server failed to marshal a valid JSON error response"
			glog.Errorf(msg, errors.Wrap(err, msg))
			out.WriteString("\"errors\": [ { \"message\": \"" + msg + "\" } ]")
			out.WriteRune('}')
			return out.WriteTo(w)
		}

		out.WriteString("\"errors\":")
		out.Write(js)
	}

	if r.Data.Len() > 0 {
		out.WriteString("\"data\": {")
		out.Write(r.Data.Bytes())
		out.WriteRune('}')
	}

	out.WriteRune('}')
	return out.WriteTo(w)
}
