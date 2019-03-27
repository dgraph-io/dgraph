/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
)

func TestParseMutationError(t *testing.T) {
	query := `
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
	`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Equal(t, `Expected { at the start of block. Got: [mutation]`, err.Error())
}

func TestParseMutationError2(t *testing.T) {
	query := `
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
	`
	_, err := ParseMutation(query)
	require.Error(t, err)
	require.Equal(t, `Expected { at the start of block. Got: [set]`, err.Error())
}

func TestParseMutationAndQueryWithComments(t *testing.T) {
	query := `
	# Mutation
		mutation {
			# Set block
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			# Delete block
			delete {
				<name> <is> <something-else> .
			}
		}
		# Query starts here.
		query {
			me(func: uid( 0x5)) { # now mention children
				name		# Name
				hometown # hometown of the person
			}
		}
	`
	_, err := Parse(Request{Str: query})
	require.Error(t, err)
}

func TestParseMutation(t *testing.T) {
	m := `
		{
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		}
	`
	mu, err := ParseMutation(m)
	require.NoError(t, err)
	require.NotNil(t, mu)
	sets, err := parseNquads(mu.SetNquads)
	require.NoError(t, err)
	require.EqualValues(t, &api.NQuad{
		Subject: "name", Predicate: "is", ObjectId: "something"},
		sets[0])
	require.EqualValues(t, &api.NQuad{
		Subject: "hometown", Predicate: "is", ObjectId: "san/francisco"},
		sets[1])
	dels, err := parseNquads(mu.DelNquads)
	require.NoError(t, err)
	require.EqualValues(t, &api.NQuad{
		Subject: "name", Predicate: "is", ObjectId: "something-else"},
		dels[0])
}

func TestParseMutationTooManyBlocks(t *testing.T) {
	tests := []struct {
		m      string
		errStr string
	}{
		{m: `
			{
				set { _:a1 <reg> "a1 content" . }
			}{
				set { _:b2 <reg> "b2 content" . }
			}`,
			errStr: "Unexpected { after the end of the block.",
		},
		{m: `{set { _:a1 <reg> "a1 content" . }} something`,
			errStr: "Invalid operation type: something after the end of the block",
		},
		{m: `
			# comments are ok
			{
				set { _:a1 <reg> "a1 content" . } # comments are ok
			} # comments are ok`,
		},
	}
	for _, tc := range tests {
		mu, err := ParseMutation(tc.m)
		if tc.errStr != "" {
			require.Contains(t, err.Error(), tc.errStr)
			require.Nil(t, mu)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestParseMutationTxn1(t *testing.T) {
	m := `
	txn {
		query {
			me(func: eq(email, "someone@gmail.com")) {
				v as uid
			}
		}

		mutation {
			set {
				uid(v) <name> "Some One" .
				uid(v) <email> "someone@gmail.com" .
			}
		}
	}`
	mu, err := ParseMutation(m)
	require.NoError(t, err)
	require.NotNil(t, mu)
	query := `{
			me(func: eq(email, "someone@gmail.com")) {
				v as uid
			}
		}`
	require.EqualValues(t, query, mu.TxnQuery)
	sets, err := parseNquads(mu.SetNquads)
	require.NoError(t, err)
	require.EqualValues(t,
		&api.NQuad{
			VarName:     &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "v"}},
			Predicate:   "name",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Some One"}},
		},
		sets[0])
	require.EqualValues(t,
		&api.NQuad{
			VarName:     &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "v"}},
			Predicate:   "email",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "someone@gmail.com"}},
		},
		sets[1])
}

func TestParseMutationTxn2(t *testing.T) {
	m := `
	txn {
		mutation {
			set {
				uid(v) <name> "Some One" .
				uid(v) <email> "someone@gmail.com" .
			}
		}
	}`
	mu, err := ParseMutation(m)
	require.NoError(t, err)
	require.NotNil(t, mu)
	sets, err := parseNquads(mu.SetNquads)
	require.NoError(t, err)
	require.EqualValues(t,
		&api.NQuad{
			VarName:     &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "v"}},
			Predicate:   "name",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Some One"}},
		},
		sets[0])
	require.EqualValues(t,
		&api.NQuad{
			VarName:     &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "v"}},
			Predicate:   "email",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "someone@gmail.com"}},
		},
		sets[1])
}

func TestParseMutationErr1(t *testing.T) {
	m := `
	txn {
		{
			set {
				<_:taco> <name> "Some One" .
				<_:taco> <email> "someone@gmail.com" .
			}
		}
	}`
	_, err := ParseMutation(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Invalid operation type: set`)
}

func TestParseMutationErr2(t *testing.T) {
	m := `
	txn {
		query {}
	}`
	_, err := ParseMutation(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Unexpected "}" inside of txn block.`)
}

func TestParseMutationErr3(t *testing.T) {
	m := `
	txn {
		query {}
		query {}
	}`
	_, err := ParseMutation(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Too many query blocks in txn`)
}

func TestParseMutationErr4(t *testing.T) {
	m := `
	txn {
		query {}
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		`
	_, err := ParseMutation(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Invalid mutation.`)
}

func TestParseMutationErr5(t *testing.T) {
	m := `
	txn {
		query {
		mutation {
			set {
				<name> <is> <something> .
				<hometown> <is> <san/francisco> .
			}
			delete {
				<name> <is> <something-else> .
			}
		`
	_, err := ParseMutation(m)
	require.Error(t, err)
	require.Contains(t, err.Error(), `Invalid character '}' inside query text`)
}
