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
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/gqlparser/v2/gqlerror"
)

func TestGQLWrapf_Error(t *testing.T) {
	tests := map[string]struct {
		err  error
		msg  string
		args []interface{}
		req  string
	}{
		"wrap one error": {err: errors.New("An error occurred"),
			msg: "mutation failed",
			req: "mutation failed because An error occurred"},
		"wrap multiple errors": {
			err: GQLWrapf(errors.New("A Dgraph error occurred"), "couldn't check ID type"),
			msg: "delete mutation failed",
			req: "delete mutation failed because couldn't check ID type because " +
				"A Dgraph error occurred"},
		"wrap an x.GqlError": {err: x.GqlErrorf("of bad GraphQL input"),
			msg: "couldn't generate query",
			req: "couldn't generate query because of bad GraphQL input"},
		"wrap and format": {err: errors.New("an error occurred"),
			msg:  "couldn't generate %s for %s",
			args: []interface{}{"query", "you"},
			req:  "couldn't generate query for you because an error occurred"},
		"wrap a list": {
			err: x.GqlErrorList{
				x.GqlErrorf("an error occurred"),
				x.GqlErrorf("something bad happend"),
			},
			msg: "couldn't do it",
			req: "couldn't do it because an error occurred\n" +
				"couldn't do it because something bad happend"},
	}

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tcase.req, GQLWrapf(tcase.err, tcase.msg, tcase.args...).Error())
		})
	}
}

func TestGQLWrapLocationf_Error(t *testing.T) {

	tests := map[string]struct {
		err  error
		msg  string
		args []interface{}
		loc  x.Location
		req  string
	}{
		"wrap one error": {err: errors.New("An error occurred"),
			msg: "mutation failed",
			loc: x.Location{Line: 1, Column: 2},
			req: "mutation failed because An error occurred (Locations: [{Line: 1, Column: 2}])"},
		"wrap multiple errors": {
			err: GQLWrapf(errors.New("A Dgraph error occurred"), "couldn't check ID type"),
			msg: "delete mutation failed",
			loc: x.Location{Line: 1, Column: 2},
			req: "delete mutation failed because couldn't check ID type because " +
				"A Dgraph error occurred (Locations: [{Line: 1, Column: 2}])"},
		"wrap an x.GqlError with location": {
			err: x.GqlErrorf("of bad GraphQL input").WithLocations(x.Location{Line: 1, Column: 8}),
			msg: "couldn't generate query",
			loc: x.Location{Line: 1, Column: 2},
			req: "couldn't generate query because of bad GraphQL input " +
				"(Locations: [{Line: 1, Column: 8}, {Line: 1, Column: 2}])"},
		"wrap and format": {err: errors.New("an error occurred"),
			msg:  "couldn't generate %s for %s",
			args: []interface{}{"query", "you"},
			loc:  x.Location{Line: 1, Column: 2},
			req: "couldn't generate query for you because an error occurred " +
				"(Locations: [{Line: 1, Column: 2}])"},
		"wrap a list": {
			err: x.GqlErrorList{
				x.GqlErrorf("an error occurred"),
				x.GqlErrorf("something bad happend").WithLocations(x.Location{Line: 1, Column: 8}),
			},
			msg: "couldn't do it",
			loc: x.Location{Line: 1, Column: 2},
			req: "couldn't do it because an error occurred (Locations: [{Line: 1, Column: 2}])\n" +
				"couldn't do it because something bad happend " +
				"(Locations: [{Line: 1, Column: 8}, {Line: 1, Column: 2}])"},
	}

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t,
				tcase.req,
				GQLWrapLocationf(tcase.err, tcase.loc, tcase.msg, tcase.args...).Error())
		})
	}
}

func TestGQLWrapf_nil(t *testing.T) {
	require.Nil(t, GQLWrapf(nil, "nothing"))
}

func TestAsGQLErrors(t *testing.T) {
	tests := map[string]struct {
		err error
		req string
	}{
		"just an error": {err: errors.New("An error occurred"),
			req: `[{"message": "An error occurred"}]`},
		"wrap an error": {
			err: GQLWrapf(errors.New("A Dgraph error occurred"), "couldn't check ID type"),
			req: `[{"message": "couldn't check ID type because A Dgraph error occurred"}]`},
		"an x.GqlError": {err: x.GqlErrorf("A GraphQL error"),
			req: `[{"message": "A GraphQL error"}]`},
		"an x.GqlError with a location": {err: x.GqlErrorf("A GraphQL error at a location").
			WithLocations(x.Location{Line: 1, Column: 2}),
			req: `[{
				"message": "A GraphQL error at a location",
				"locations": [{"column":2, "line":1}]}]`},
		"wrap an x.GqlError with a location": {
			err: GQLWrapf(x.GqlErrorf("this error has a location").
				WithLocations(x.Location{Line: 1, Column: 2}), "this error didn't need a location"),
			req: `[{
				"message": "this error didn't need a location because this error has a location",
				"locations": [{"column":2, "line":1}]}]`},
		"GQLWrapLocationf": {err: GQLWrapLocationf(x.GqlErrorf("this error didn't have a location"),
			x.Location{Line: 1, Column: 8},
			"there's one location"),
			req: `[{
				"message": "there's one location because this error didn't have a location",
				"locations": [{"column":8, "line":1}]}]`},
		"GQLWrapLocationf wrapping a location": {
			err: GQLWrapLocationf(x.GqlErrorf("this error also had a location").
				WithLocations(x.Location{Line: 1, Column: 2}), x.Location{Line: 1, Column: 8},
				"there's two locations"),
			req: `[{
				"message": "there's two locations because this error also had a location",
				"locations": [{"column":2, "line":1}, {"column":8, "line":1}]}]`},
		"an x.GqlErrorList": {
			err: x.GqlErrorList{
				x.GqlErrorf("A GraphQL error"),
				x.GqlErrorf("Another GraphQL error").WithLocations(x.Location{Line: 1, Column: 2})},
			req: `[
				{"message":"A GraphQL error"},
				{"message":"Another GraphQL error", "locations": [{"column":2, "line":1}]}]`},
		"a gql parser error": {
			err: gqlerror.Errorf("A GraphQL error"),
			req: `[{"message": "A GraphQL error"}]`},
		"a gql parser error with a location": {
			err: &gqlerror.Error{
				Message:   "A GraphQL error",
				Locations: []gqlerror.Location{{Line: 1, Column: 2}}},
			req: `[{"message": "A GraphQL error", "locations": [{"column":2, "line":1}]}]`},
		"a list of gql parser errors": {
			err: gqlerror.List{
				gqlerror.Errorf("A GraphQL error"), gqlerror.Errorf("Another GraphQL error")},
			req: `[{"message":"A GraphQL error"}, {"message":"Another GraphQL error"}]`},
	}

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			gqlErrs, err := json.Marshal(AsGQLErrors(tcase.err))
			require.NoError(t, err)

			assert.JSONEq(t, tcase.req, string(gqlErrs))
		})
	}
}

func TestAsGQLErrors_nil(t *testing.T) {
	require.Nil(t, AsGQLErrors(nil))
}

func TestAppendGQLErrs(t *testing.T) {
	tests := map[string]struct {
		err1 error
		err2 error
		req  string
	}{
		"two errors": {
			err1: errors.New("An error occurred"),
			err2: errors.New("Another error"),
			req:  `[{"message": "An error occurred"}, {"message": "Another error"}]`,
		},
		"left nil": {
			err1: nil,
			err2: errors.New("An error occurred"),
			req:  `[{"message": "An error occurred"}]`,
		},
		"right nil": {
			err1: errors.New("An error occurred"),
			err2: nil,
			req:  `[{"message": "An error occurred"}]`,
		},
		"both nil": {
			err1: nil,
			err2: nil,
			req:  "null",
		},
	}

	for name, tcase := range tests {
		t.Run(name, func(t *testing.T) {
			gqlErrs, err := json.Marshal(AppendGQLErrs(tcase.err1, tcase.err2))
			require.NoError(t, err)

			assert.JSONEq(t, tcase.req, string(gqlErrs))
		})
	}
}
