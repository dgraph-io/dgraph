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
	"github.com/vektah/gqlparser/gqlerror"
)

type gqlableError struct {
	gqlErr *gqlerror.Error
	cause  error
}

func (gqlerr *gqlableError) Error() string {
	return gqlerr.gqlErr.Error() + " because " + gqlerr.cause.Error()
}

// AsGQLErrors formats an error as a list of GraphQL errors.
// A gqlerror.List gets returned as is, a gqlerror.Error gets returned as a one
// item list, GQLWrap'd errors have their messages formatted from all underlying causes,
// and all other errors get printed into a gqlerror.Error.  A nil input results
// in nil output.
func AsGQLErrors(err error) gqlerror.List {
	if err == nil {
		return nil
	}

	switch e := err.(type) {
	case *gqlerror.Error:
		return gqlerror.List{e}
	case gqlerror.List:
		return e
	case *gqlableError:
		return gqlerror.List{&gqlerror.Error{
			Message:   e.Error(),
			Locations: e.gqlErr.Locations,
			// include things like Path when we use it
		}}
	default:
		return gqlerror.List{gqlerror.Errorf(e.Error())}
	}
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

	wrapped := &gqlableError{
		gqlErr: gqlerror.Errorf(format, args...),
		cause:  err,
	}

	if gqlErr, ok := err.(*gqlableError); ok {
		wrapped.gqlErr.Locations = gqlErr.gqlErr.Locations
	} else if gqlErr, ok := err.(*gqlerror.Error); ok {
		wrapped.gqlErr.Locations = gqlErr.Locations
	}

	return wrapped
}

// GQLWrapLocationf wraps an error as a GraphQL error and includes location
// information in the GraphQL error.
func GQLWrapLocationf(err error, line int, col int, format string, args ...interface{}) error {
	wrapped := GQLWrapf(err, format, args...)
	if wrapped == nil {
		return nil
	}

	var gqlable *gqlableError
	var ok bool
	if gqlable, ok = wrapped.(*gqlableError); !ok {
		panic("GQLWrapf didn't result in a gqlableError")
	}
	gqlable.gqlErr.Locations =
		append(gqlable.gqlErr.Locations, gqlerror.Location{Line: line, Column: col})

	return gqlable
}

// Cause returns the underlying cause of an error.  If it's a GQLWrapf'd
// error, the wraping is unwound to the orriginal cause, otherwise, the
// cause is err.
func Cause(err error) error {
	if gqlErr, ok := err.(*gqlableError); ok {
		return Cause(gqlErr.cause)
	}
	return err
}
