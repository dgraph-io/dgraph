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
