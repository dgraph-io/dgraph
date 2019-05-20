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
	require.EqualValues(t, query, mu.CondQuery)
	sets, err := parseNquads(mu.SetNquads)
	require.NoError(t, err)
	require.EqualValues(t,
		&api.NQuad{
			SubjectVar:  "v",
			Predicate:   "name",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Some One"}},
		},
		sets[0])
	require.EqualValues(t,
		&api.NQuad{
			SubjectVar:  "v",
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
			SubjectVar:  "v",
			Predicate:   "name",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Some One"}},
		},
		sets[0])
	require.EqualValues(t,
		&api.NQuad{
			SubjectVar:  "v",
			Predicate:   "email",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "someone@gmail.com"}},
		},
		sets[1])
}

func TestParseMutationTxn3(t *testing.T) {
	m := `
	txn {
		mutation {
			set {
				<0x1> <name> "Some One" .
				<0x1> <email> "someone@gmail.com" .
				<0x1> <friend> uid(v) .
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
			Subject:     "0x1",
			Predicate:   "name",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Some One"}},
		},
		sets[0])
	require.EqualValues(t,
		&api.NQuad{
			Subject:     "0x1",
			Predicate:   "email",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "someone@gmail.com"}},
		},
		sets[1])
	require.EqualValues(t,
		&api.NQuad{
			Subject:   "0x1",
			Predicate: "friend",
			ObjectVar: "v",
		},
		sets[2])
}

func TestParseMutationTxn4(t *testing.T) {
	m := `
	txn {
		mutation {
			set {
				uid(v) <name> "Some One" .
				uid(v) <friend> uid(w) .
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
			SubjectVar:  "v",
			Predicate:   "name",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Some One"}},
		},
		sets[0])
	require.EqualValues(t,
		&api.NQuad{
			SubjectVar: "v",
			Predicate:  "friend",
			ObjectVar:  "w",
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
	require.Contains(t, err.Error(), `Unbalanced '}' found inside query text`)
}
