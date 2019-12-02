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
	"fmt"

	"github.com/dgraph-io/dgraph/x"
	"github.com/vektah/gqlparser/gqlerror"
)

type gqlableError struct {
	gqlErr *x.GqlError
	cause  error
}

func (gqlable *gqlableError) Error() string {
	var buf bytes.Buffer
	buf.WriteString(gqlable.gqlErr.Message)
	buf.WriteString(" because ")
	switch cause := gqlable.cause.(type) {
	case *x.GqlError:
		// Avoid writing locations into the error string.
		buf.WriteString(cause.Message)
	default:
		buf.WriteString(cause.Error())
	}
	return buf.String()
}

func (gqlable *gqlableError) locations() []x.Location {
	switch cause := gqlable.cause.(type) {
	case *gqlableError:
		return append(cause.locations(), gqlable.gqlErr.Locations...)
	case *x.GqlError:
		return append(cause.Locations, gqlable.gqlErr.Locations...)
	default:
		return gqlable.gqlErr.Locations
	}
}

// AsGQLErrors formats an error as a list of GraphQL errors.
// A []*x.GqlError gets returned as is, an x.GqlError  gets returned as a one
// item list, GQLWrap'd errors have their messages formatted from all underlying causes,
// and all other errors get printed into a x.GqlError .  A nil input results
// in nil output.
func AsGQLErrors(err error) x.GqlErrorList {
	if err == nil {
		return nil
	}

	switch e := err.(type) {
	case *gqlerror.Error:
		return x.GqlErrorList{toGqlError(e)}
	case *x.GqlError:
		return x.GqlErrorList{e}
	case gqlerror.List:
		return toGqlErrorList(e)
	case x.GqlErrorList:
		return e
	case *gqlableError:
		return x.GqlErrorList{&x.GqlError{
			Message:   e.Error(),
			Locations: e.locations(),
			Path:      e.gqlErr.Path,
		}}
	default:
		return x.GqlErrorList{&x.GqlError{Message: e.Error()}}
	}
}

func toGqlError(err *gqlerror.Error) *x.GqlError {
	return &x.GqlError{
		Message:   err.Message,
		Locations: convertLocations(err.Locations),
		Path:      err.Path,
	}
}

func toGqlErrorList(errs gqlerror.List) x.GqlErrorList {
	var result x.GqlErrorList
	for _, err := range errs {
		result = append(result, toGqlError(err))
	}
	return result
}

func convertLocations(locs []gqlerror.Location) []x.Location {
	var result []x.Location
	for _, loc := range locs {
		result = append(result, x.Location{Line: loc.Line, Column: loc.Column})
	}
	return result
}

// GQLWrapf takes an existing error and wraps it as a GraphQL error.
// If err is already a GraphQL error, any location information is kept in the
// new error. err is saved as the underlying error (recoverable with
// Cause() - same style as pkg/errors).  If err is nil, GQLWrapf returns nil.
//
// Wrapping GraphQL errors like this allows us to bubble errors up the stack
// and add context, location and path info to them as we go.
func GQLWrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	return &gqlableError{
		gqlErr: &x.GqlError{Message: fmt.Sprintf(format, args...)},
		cause:  err,
	}
}

// GQLWrapLocationf wraps an error as a GraphQL error and includes location
// information in the GraphQL error.
func GQLWrapLocationf(err error, loc x.Location, format string, args ...interface{}) error {
	wrapped := GQLWrapf(err, format, args...)
	if wrapped == nil {
		return nil
	}

	var gqlable *gqlableError
	var ok bool
	if gqlable, ok = wrapped.(*gqlableError); !ok {
		panic("GQLWrapf didn't result in a gqlableError")
	}
	gqlable.gqlErr.Locations = append(gqlable.gqlErr.Locations, loc)

	return gqlable
}

// Cause returns the underlying cause of an error.  If it's a GQLWrapf'd
// error, the wraping is unwound to the original cause, otherwise, the
// cause is err.
func Cause(err error) error {
	if gqlErr, ok := err.(*gqlableError); ok {
		return Cause(gqlErr.cause)
	}
	return err
}
