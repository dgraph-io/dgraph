package gql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMutationUpsertBlock1(t *testing.T) {
	query := `
	query {
		me(func: eq(age, 34)) {
			uid
			friend {
				uid
				age
			}
		}
	}
`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid block: [query]")
}

func TestMutationUpsertBlock2(t *testing.T) {
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

func TestMutationUpsertBlock3(t *testing.T) {
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
	require.Contains(t, err.Error(), "empty mutation block")
}

func TestMutationUpsertBlock4(t *testing.T) {
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
	require.Contains(t, err.Error(), "multiple query ops inside upsert block")
}

func TestMutationUpsertBlock5(t *testing.T) {
	query := `upsert {}`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty mutation block")
}

func TestMutationUpsertBlock6(t *testing.T) {
	query := `upsert {`
	_, err := ParseMutation(query)
	require.Contains(t, err.Error(), "Unclosed upsert block")
}

func TestMutationUpsertBlock7(t *testing.T) {
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

func TestMutationUpsertBlock8(t *testing.T) {
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
	require.Contains(t, err.Error(), "query op not found in upsert block")
}

func TestMutationUpsertBlock9(t *testing.T) {
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

func TestMutationUpsertBlock10(t *testing.T) {
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

func TestMutationUpsertBlock11(t *testing.T) {
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

func TestMutationUpsertBlock12(t *testing.T) {
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

func TestMutationUpsertBlock13(t *testing.T) {
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
