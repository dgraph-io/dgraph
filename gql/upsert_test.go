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

package gql

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidBlockErr(t *testing.T) {
	query := `
query {
  me(func: eq(age, 34)) {
    uid
    friend {
      uid
      age
    }
  }
}`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid block: [query]")
}

func TestExtraRightCurlErr(t *testing.T) {
	query := `
upsert {
  query {
    me(func: eq(age, 34)) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
}
`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Too many right curl")
}

func TestNoMutationErr(t *testing.T) {
	query := `
upsert {
  query {
    me(func: eq(age, 34)) {
      uid
      friend {
        uid age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Empty mutation block")
}

func TestMultipleQueryErr(t *testing.T) {
	query := `
upsert {
  mutation {
    set {
      "_:user1" <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) {
      uid
      friend {
        uid
        age
      }
    }
  }

  query {
    me2(func: eq(age, 34)) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Multiple query ops inside upsert block")
}

func TestEmptyUpsertErr(t *testing.T) {
	query := `upsert {}`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Empty mutation block")
}

func TestNoRightCurlErr(t *testing.T) {
	query := `upsert {`
	_, err := ParseMutation(query)
	require.Contains(t, err.Error(), "Unclosed upsert block")
}

func TestIncompleteBlockErr(t *testing.T) {
	query := `
upsert {
  mutation {
    set {
      "_:user1" <age> "45" .
    }
  }

  query {
    me(func: eq(age, "{
`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unexpected end of input")
}

func TestMissingQueryErr(t *testing.T) {
	query := `
upsert {
  mutation {
    set {
      "_:user1" <age> "45" .
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Query op not found in upsert block")
}

func TestUpsertWithFragment(t *testing.T) {
	query := `
upsert {
  mutation {
    set {
      "_:user1" <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) {
      ...fragmentA
      friend {
        ...fragmentA
        age
      }
    }
  }

  fragment fragmentA {
    uid
  }
}
`
	_, err := ParseMutation(query)
	require.Nil(t, err)
}

func TestUpsertEx1(t *testing.T) {
	query := `
upsert {
  mutation {
    set {
      "_:user1" <age> "45" .
    }
  }

  query {
    me(func: eq(age, "{")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Nil(t, err)
}

func TestUpsertWithSpaces(t *testing.T) {
	query := `
upsert

{
  mutation

  {
    set
    {
      "_:user1" <age> "45" .

      # This is a comment
      "_:user1" <name> "{vishesh" .
    }}

  query

  {
    me(func: eq(age, "{")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Nil(t, err)
}

func TestUpsertWithBlankNode(t *testing.T) {
	query := `
upsert {
  query {
    me(func: eq(age, 34)) {
      uid
      friend {
        uid
        age
      }
    }
  }

  mutation {
    set {
      "_:user1" <age> "45" .
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Nil(t, err)
}

func TestUpsertMutationThenQuery(t *testing.T) {
	query := `
upsert {
  mutation {
    set {
      "_:user1" <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Nil(t, err)
}

func TestUpsertWithFilter(t *testing.T) {
	query := `upsert {
  mutation {
    set {
      uid(a) <age> "45"
      uid(b) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) @filter(ge(name, "user")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Nil(t, err)
}

// Helper function to test parsing of if condition
func parseIfCondition(cond string) (*FilterTree, error) {
	cond = "{a(func: uid(0x01)) " + strings.Replace(cond, "@if", "@filter", 1) + "{uid}}"
	r, err := Parse(Request{Str: cond})
	if err == nil {
		return r.Query[0].Filter, nil
	}

	return nil, err
}

func TestConditionalUpsertWithNewlines(t *testing.T) {
	query := `upsert {
  mutation @if(eq(len(m), 1)
               AND
               gt(len(f), 0)) {
    set {
      uid(m) <age> "45" .
      uid(f) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) @filter(ge(name, "user")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	mu, err := ParseMutation(query)
	require.Nil(t, err)
	_, err = parseIfCondition(mu.Cond)
	require.Nil(t, err)
}

func TestConditionalUpsertFuncTree(t *testing.T) {
	query := `upsert {
  mutation @if( ( eq(len(m), 1)
                  OR
                  lt(90, len(h)))
                AND
                gt(len(f), 0)) {
    set {
      uid(m) <age> "45" .
      uid(f) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) @filter(ge(name, "user")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	mu, err := ParseMutation(query)
	require.Nil(t, err)
	_, err = parseIfCondition(mu.Cond)
	require.Nil(t, err)
}

func TestConditionalUpsertMultipleFuncArg(t *testing.T) {
	query := `upsert {
  mutation @if( ( eq(len(m), len(t))
                  OR
                  lt(90, len(h)))
                AND
                gt(len(f), 0)) {
    set {
      uid(m) <age> "45" .
      uid(f) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) @filter(ge(name, "user")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	mu, err := ParseMutation(query)
	require.Nil(t, err)
	_, err = parseIfCondition(mu.Cond)
	require.Contains(t, err.Error(), "Multiple functions as arguments not allowed")
}

func TestConditionalUpsertErrMissingRightRound(t *testing.T) {
	query := `upsert {
  mutation @if(eq(len(m, 1)
               AND
               gt(len(f), 0)) {
    set {
      uid(m) <age> "45" .
      uid(f) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) @filter(ge(name, "user")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Contains(t, err.Error(), "Matching brackets not found")
}

func TestConditionalUpsertErrUnclosed(t *testing.T) {
	query := `upsert {
  mutation @if(eq(len(m), 1) AND gt(len(f), 0))`
	_, err := ParseMutation(query)
	require.Contains(t, err.Error(), "Unclosed mutation action")
}

func TestConditionalUpsertErrInvalidIf(t *testing.T) {
	query := `upsert {
  mutation @if`
	_, err := ParseMutation(query)
	require.Contains(t, err.Error(), "Matching brackets not found")
}

func TestConditionalUpsertErrWrongIf(t *testing.T) {
	query := `upsert {
  mutation @fi( ( eq(len(m), 1)
                  OR
                  lt(len(h), 90))
                AND
                gt(len(f), 0)) {
    set {
      uid(m) <age> "45" .
      uid(f) <age> "45" .
    }
  }

  query {
    me(func: eq(age, 34)) @filter(ge(name, "user")) {
      uid
      friend {
        uid
        age
      }
    }
  }
}
`
	_, err := ParseMutation(query)
	require.Contains(t, err.Error(), "Expected @if, found [@fi]")
}

func TestIfDirectiveNoIf(t *testing.T) {
	ft, err := parseIfCondition("")
	require.Nil(t, ft)
	require.Nil(t, err)
}

func TestIfDirectiveWithComment(t *testing.T) {
	cond := ` @if( ( eq(len(m), 1)
  # This is a comment
                  OR
                  lt(90, len(h)))
                AND
                gt(len(f), 0))`
	_, err := parseIfCondition(cond)
	require.Nil(t, err)
}

func TestIfDirectiveWithSameLineComment(t *testing.T) {
	cond := ` @if( ( eq(len(m), 1)
                  OR # This is another comment
                  lt(90, len(h)) )
                AND
                gt(len(f), 0))`
	_, err := parseIfCondition(cond)
	require.Nil(t, err)
}

func TestIfDirectiveWithAfterIfComment(t *testing.T) {
	cond := ` @if # This comment is okay too
                ((eq(len(m), 1)
                  OR # This is another comment
                  lt(90, len(h)) )
                AND
                gt(len(f), 0)) `
	_, err := parseIfCondition(cond)
	require.Nil(t, err)
}

func TestIfDirectiveErrMissingAt(t *testing.T) {
	cond := ` if( ( eq(len(m), 1)
                  OR
                  lt(90, len(h)))
                AND
                gt(len(f), 0))`
	_, err := parseIfCondition(cond)
	require.Contains(t, err.Error(), "Expecting argument name. Got: lex.Item [11] \"(\"")
}

func TestIfDirectiveErrWhiteSpace(t *testing.T) {
	cond := ` `
	ft, err := parseIfCondition(cond)
	require.Nil(t, err)
	require.Nil(t, ft)
}

func TestIfDirectiveNoParseErr(t *testing.T) {
	cond := ` @if( ( eq(len(m), 1)
                  OR
                  lt(90, len(h)))
                AND
                gt(len(h), 0)) @if()`
	_, err := parseIfCondition(cond)
	require.Contains(t, err.Error(), "Repeated filter or if at root")
}

// TODO(Aman): test fails, This is a bad error message.
func TestIfDirectiveErrMissingRightRound(t *testing.T) {
	cond := `@if( ( eq(len(m), 1)
                  OR
                  lt(90, len(h)))
                AND
                gt(len(h), 0)`
	_, err := parseIfCondition(cond)
	require.Contains(t, err.Error(), "Unrecognized character inside a func: U+007B '{'")
}
