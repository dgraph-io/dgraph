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
	"time"

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
	Tracing   *Trace `json:"tracing,omitempty"`
}

// Trace : Apollo Tracing is a GraphQL extension for tracing resolver performance.
// https://github.com/apollographql/apollo-tracing
// Not part of the standard itself, it gets reported in GraphQL "extensions".
// It's for reporting tracing data through all the resolvers in a GraphQL query.
// Our results aren't as 'deep' as a traditional GraphQL server in that the Dgraph
// layer resolves in a single step, rather than iteratively.  So we'll report on
// all the top level queries/mutations.
//
// Currently, only reporting in the GraphQL result, but also planning to allow
// exposing to Apollo Engine as per:
// https://www.apollographql.com/docs/references/setup-analytics/#engine-reporting-endpoint
type Trace struct {
	// (comments from Apollo Tracing spec)

	Version int64 `json:"version"`

	// Timestamps in RFC 3339 format.
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`

	// Duration in nanoseconds, relative to the request start, as an integer.
	Duration int64 `json:"duration"`

	Parsing    *OffsetDuration  `json:"parsing,omitempty"`
	Validation *OffsetDuration  `json:"validation,omitempty"`
	Execution  []*ResolverTrace `json:"execution,omitempty"`
}

// An OffsetDuration records the offset start and duration of GraphQL parsing/validation.
type OffsetDuration struct {
	// (comments from Apollo Tracing spec)

	// Offset in nanoseconds, relative to the request start, as an integer
	StartOffset int64 `json:"startOffset"`

	// Duration in nanoseconds, relative to start of operation, as an integer.
	Duration int64 `json:"duration"`
}

// LabeledOffsetDuration is an OffsetDuration with a string label.
type LabeledOffsetDuration struct {
	Label string `json:"label"`
	OffsetDuration
}

// A ResolverTrace is a trace of one resolver.  In a traditional GraphQL server,
// resolving say a query, would result in a ResolverTrace for the query itself
// (with duration spanning the time to resolve the entire query) and traces for
// every field in the query (with duration for just that field).
//
// Dgraph GraphQL layer resolves Queries in a single step, so each query has only
// one associated ResolverTrace.  Mutations require two steps - the mutation itself
// and the following query.  So mutations will have a ResolverTrace with duration
// spanning the entire mutation (including the query part), and a trace of the query.
// To give insight into what's actually happening there, the Dgraph time is also
// recorded.  So for a mutation you can see total duration, mutation duration,
// query duration and also amount of time spent by the API orchestrating the
// mutation/query.
type ResolverTrace struct {
	// (comments from Apollo Tracing spec)

	// the response path of the current resolver - same format as path in error
	// result format specified in the GraphQL specification
	Path       []interface{} `json:"path"`
	ParentType string        `json:"parentType"`
	FieldName  string        `json:"fieldName"`
	ReturnType string        `json:"returnType"`

	// Offset relative to request start and total duration or resolving
	OffsetDuration

	// Dgraph isn't in Apollo tracing.  It records the offsets and times
	// of Dgraph operations for the query/mutation (including network latency)
	// in nanoseconds.
	Dgraph []*LabeledOffsetDuration `json:"dgraph"`
}

// A TimerFactory makes offset timers that can be used to fill out an OffsetDuration.
type TimerFactory interface {
	NewOffsetTimer(storeTo *OffsetDuration) OffsetTimer
}

// An OffsetTimer is used to fill out an OffsetDuration.  Start starts the timer
// and calculates the offset.  Stop calculates the duration.
type OffsetTimer interface {
	Start()
	Stop()
}

type timerFactory struct {
	offsetFrom time.Time
}

type offsetTimer struct {
	offsetFrom time.Time
	start      time.Time
	backing    *OffsetDuration
}

// NewOffsetTimerFactory creates a new TimerFactory given offsetFrom as the
// reference time to calculate the OffsetDuration.StartOffset from.
func NewOffsetTimerFactory(offsetFrom time.Time) TimerFactory {
	return &timerFactory{offsetFrom: offsetFrom}
}

func (tf *timerFactory) NewOffsetTimer(storeTo *OffsetDuration) OffsetTimer {
	return &offsetTimer{
		offsetFrom: tf.offsetFrom,
		backing:    storeTo,
	}
}

func (ot *offsetTimer) Start() {
	ot.start = time.Now()
	ot.backing.StartOffset = ot.start.Sub(ot.offsetFrom).Nanoseconds()
}

func (ot *offsetTimer) Stop() {
	ot.backing.Duration = time.Since(ot.start).Nanoseconds()
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

	var js []byte
	var err error

	js, err = json.Marshal(struct {
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
