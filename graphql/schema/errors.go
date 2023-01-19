/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"fmt"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/gqlparser/v2/ast"
	"github.com/dgraph-io/gqlparser/v2/gqlerror"
)

// AsGQLErrors formats an error as a list of GraphQL errors.
// A []*x.GqlError (x.GqlErrorList) gets returned as is, an x.GqlError gets returned as a one
// item list, and all other errors get printed into a x.GqlError .  A nil input results
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
	default:
		return x.GqlErrorList{&x.GqlError{Message: e.Error()}}
	}
}

func toGqlError(err *gqlerror.Error) *x.GqlError {
	return &x.GqlError{
		Message:   err.Message,
		Locations: convertLocations(err.Locations),
		Path:      convertPath(err.Path),
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

func convertPath(path ast.Path) []interface{} {
	pathElements := []ast.PathElement(path)
	var result []interface{}
	for _, p := range pathElements {
		result = append(result, p)
	}
	return result
}

// GQLWrapf takes an existing error and wraps it as a GraphQL error.
// If err is already a GraphQL error, any location information is kept in the
// new error.  If err is nil, GQLWrapf returns nil.
//
// Wrapping GraphQL errors like this allows us to bubble errors up the stack
// and add context, location and path info to them as we go.
func GQLWrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}

	switch err := err.(type) {
	case *x.GqlError:
		return x.GqlErrorf("%s because %s", fmt.Sprintf(format, args...), err.Message).
			WithLocations(err.Locations...).
			WithPath(err.Path)
	case x.GqlErrorList:
		var errs x.GqlErrorList
		for _, e := range err {
			errs = append(errs, GQLWrapf(e, format, args...).(*x.GqlError))
		}
		return errs
	default:
		return x.GqlErrorf("%s because %s", fmt.Sprintf(format, args...), err.Error())
	}
}

// GQLWrapLocationf wraps an error as a GraphQL error and includes location
// information in the GraphQL error.
func GQLWrapLocationf(err error, loc x.Location, format string, args ...interface{}) error {
	wrapped := GQLWrapf(err, format, args...)
	if wrapped == nil {
		return nil
	}

	switch wrapped := wrapped.(type) {
	case *x.GqlError:
		return wrapped.WithLocations(loc)
	case x.GqlErrorList:
		for _, e := range wrapped {
			_ = e.WithLocations(loc)
		}
	}
	return wrapped
}

// AppendGQLErrs builds a list of GraphQL errors from err1 and err2, if both
// are nil, the result is nil.
func AppendGQLErrs(err1, err2 error) error {
	if err1 == nil && err2 == nil {
		return nil
	}
	if err1 == nil {
		return AsGQLErrors(err2)
	}
	if err2 == nil {
		return AsGQLErrors(err1)
	}
	return append(AsGQLErrors(err1), AsGQLErrors(err2)...)
}

// SetPathIfEmpty sets error's path with the given path item as the only item in path,
// only if initially the error had no path.
func SetPathIfEmpty(err error, pathItem interface{}) error {
	gqlErrs := AsGQLErrors(err)
	for _, e := range gqlErrs {
		if len(e.Path) == 0 {
			e.Path = []interface{}{pathItem}
		}
	}
	return gqlErrs
}

// PrependPath adds the given path item as the first item in the error's path list.
func PrependPath(err error, pathItem interface{}) error {
	gqlErrs := AsGQLErrors(err)
	for _, e := range gqlErrs {
		e.Path = append([]interface{}{pathItem}, e.Path...)
	}
	return gqlErrs
}
