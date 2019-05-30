package gql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMutationTxnBlock1(t *testing.T) {
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

func TestMutationTxnBlock2(t *testing.T) {
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

func TestMutationTxnBlock3(t *testing.T) {
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

func TestMutationTxnBlock4(t *testing.T) {
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

func TestMutationTxnBlock5(t *testing.T) {
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

func TestMutationTxnBlock6(t *testing.T) {
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

func TestMutationTxnBlock7(t *testing.T) {
	query := `upsert {}`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty mutation block")
}

func TestMutationTxnBlock8(t *testing.T) {
	query := `upsert {`
	_, err := ParseMutation(query)
	require.Contains(t, err.Error(), "Unclosed upsert block")
}

func TestMutationTxnBlock9(t *testing.T) {
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

func TestMutationTxnBlock10(t *testing.T) {
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

func TestMutationTxnBlock11(t *testing.T) {
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

func TestMutationTxnBlock12(t *testing.T) {
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
