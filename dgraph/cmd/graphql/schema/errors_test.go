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
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/gqlerror"
)

// Note: the formatting of these will change after https://github.com/dgraph-io/dgraph/pull/3728.
// ATM, we are using github.com/vektah/gqlparser/gqlerror GraphQL errors which doesn't print
// Error() so nice.  After #3728, we'll add our own Error() around our GraphQL error and this
// will print without the "input: " etc

func TestGQLWrapf(t *testing.T) {
	tests := []struct {
		err error
		msg string
		req string
	}{
		{err: errors.New("An error occured"),
			msg: "mutation failed",
			req: "input: mutation failed because An error occured"},
		{err: GQLWrapf(errors.New("A Dgraph error occured"), "couldn't check ID type"),
			msg: "delete mutation failed",
			req: "input: delete mutation failed because input: couldn't check ID type because A Dgraph error occured"},
		{err: gqlerror.Errorf("of bad GraphQL input"),
			msg: "couldn't generate query",
			req: "input: couldn't generate query because input: of bad GraphQL input"},
		{err: gqlerror.ErrorLocf("F", 1, 2, "input bad here"),
			msg: "couldn't generate query",
			req: "input:1: couldn't generate query because F:1: input bad here"},
	}

	for _, tt := range tests {
		got := GQLWrapf(tt.err, tt.msg).Error()
		assert.Equal(t, tt.req, got)
	}
}

func TestGQLWrapf_nil(t *testing.T) {
	require.Nil(t, GQLWrapf(nil, "nothing"))
}

func TestGQLWrapf_IsAlwaysgqlableError(t *testing.T) {
	tests := []struct {
		err error
	}{
		{GQLWrapf(errors.New("An error occured"), "mutation failed")},
		{GQLWrapf(GQLWrapf(errors.New("A Dgraph error occured"), "couldn't check ID type"),
			"delete mutation failed")},
		{GQLWrapf(gqlerror.Errorf("of bad GraphQL input"), "couldn't generate query")},
	}

	for _, tt := range tests {
		_, ok := tt.err.(*gqlableError)
		require.True(t, ok)
	}
}

func TestAsGQLErrors(t *testing.T) {
	tests := []struct {
		err error
		req string
	}{
		{err: errors.New("An error occured"),
			req: `[{"message": "An error occured"}]`},
		{err: GQLWrapf(errors.New("A Dgraph error occured"), "couldn't check ID type"),
			req: `[{"message": "input: couldn't check ID type because A Dgraph error occured"}]`},
		{err: gqlerror.Errorf("A GraphQL error"),
			req: `[{"message": "A GraphQL error"}]`},
		{err: gqlerror.ErrorLocf("", 1, 2, "A GraphQL error at a location"),
			req: `[{"message": "A GraphQL error at a location", "locations": [{"column":2, "line":1}]}]`},
		{err: gqlerror.List{gqlerror.Errorf("A GraphQL error"), gqlerror.Errorf("Another GraphQL error")},
			req: `[{"message":"A GraphQL error"}, {"message":"Another GraphQL error"}]`},
	}

	for _, tt := range tests {
		gqlErrs, err := json.Marshal(AsGQLErrors(tt.err))
		require.NoError(t, err)

		assert.JSONEq(t, tt.req, string(gqlErrs))
	}
}

func TestAsGQLErrors_nil(t *testing.T) {
	require.Nil(t, AsGQLErrors(nil))
}
