/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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

package edgraph

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

func makeNquad(sub, pred string, val *api.Value) *api.NQuad {
	return &api.NQuad{
		Subject:     sub,
		Predicate:   pred,
		ObjectValue: val,
	}
}

func makeNquadEdge(sub, pred, obj string) *api.NQuad {
	return &api.NQuad{
		Subject:   sub,
		Predicate: pred,
		ObjectId:  obj,
	}
}

func makeOp(schema string) *api.Operation {
	return &api.Operation{
		Schema: schema,
	}
}

func TestParseNQuads(t *testing.T) {
	nquads := `
		_:a <predA> "A" .
		_:b <predB> "B" .
		# this line is a comment
		_:a <join> _:b .
	`
	nqs, _, err := chunker.ParseRDFs([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", "predA", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "A"}}),
		makeNquad("_:b", "predB", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "B"}}),
		makeNquadEdge("_:a", "join", "_:b"),
	}, nqs)
}

func TestValNquads(t *testing.T) {
	nquads := `uid(m) <name> val(f) .`
	_, _, err := chunker.ParseRDFs([]byte(nquads))
	require.NoError(t, err)
}

func TestParseNQuadsWindowsNewline(t *testing.T) {
	nquads := "_:a <predA> \"A\" .\r\n_:b <predB> \"B\" ."
	nqs, _, err := chunker.ParseRDFs([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", "predA", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "A"}}),
		makeNquad("_:b", "predB", &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "B"}}),
	}, nqs)
}

func TestParseNQuadsDelete(t *testing.T) {
	nquads := `_:a * * .`
	nqs, _, err := chunker.ParseRDFs([]byte(nquads))
	require.NoError(t, err)
	require.Equal(t, []*api.NQuad{
		makeNquad("_:a", x.Star, &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}}),
	}, nqs)
}

func TestValidateKeys(t *testing.T) {
	tests := []struct {
		name    string
		nquad   string
		noError bool
	}{
		{name: "test 1", nquad: `_:alice <knows> "stuff" ( "key 1" = 12 ) .`, noError: false},
		{name: "test 2", nquad: `_:alice <knows> "stuff" ( "key	1" = 12 ) .`, noError: false},
		{name: "test 3", nquad: `_:alice <knows> "stuff" ( ~key1 = 12 ) .`, noError: false},
		{name: "test 4", nquad: `_:alice <knows> "stuff" ( "~key1" = 12 ) .`, noError: false},
		{name: "test 5", nquad: `_:alice <~knows> "stuff" ( "key 1" = 12 ) .`, noError: false},
		{name: "test 6", nquad: `_:alice <~knows> "stuff" ( "key	1" = 12 ) .`, noError: false},
		{name: "test 7", nquad: `_:alice <~knows> "stuff" ( key1 = 12 ) .`, noError: false},
		{name: "test 8", nquad: `_:alice <~knows> "stuff" ( "key1" = 12 ) .`, noError: false},
		{name: "test 9", nquad: `_:alice <~knows> "stuff" ( "key	1" = 12 ) .`, noError: false},
		{name: "test 10", nquad: `_:alice <knows> "stuff" ( key1 = 12 , "key 2" = 13 ) .`, noError: false},
		{name: "test 11", nquad: `_:alice <knows> "stuff" ( "key1" = 12, key2 = 13 , "key	3" = "a b" ) .`, noError: false},
		{name: "test 12", nquad: `_:alice <knows~> "stuff" ( key1 = 12 ) .`, noError: false},
		{name: "test 13", nquad: `_:alice <knows> "stuff" ( key1 = 12 ) .`, noError: true},
		{name: "test 14", nquad: `_:alice <knows@some> "stuff" .`, noError: true},
		{name: "test 15", nquad: `_:alice <knows@some@en> "stuff" .`, noError: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nq, _, err := chunker.ParseRDFs([]byte(tc.nquad))
			require.NoError(t, err)

			err = validateKeys(nq[0])
			if tc.noError {
				require.NoError(t, err, "Unexpected error for: %+v", nq)
			} else {
				require.Error(t, err, "Expected an error: %+v", nq)
			}
		})
	}
}

func TestParseSchemaFromAlterOperation(t *testing.T) {
	md := metadata.New(map[string]string{"namespace": "123"})
	ctx := metadata.NewIncomingContext(context.Background(), md)
	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	ps, err := badger.OpenManaged(badger.DefaultOptions(dir))
	x.Check(err)
	schema.Init(ps)

	tests := []struct {
		name    string
		schema  string
		noError bool
	}{
		{
			name: "test no duplicate predicates and types",
			schema: `
  				name: string @index(fulltext, term) .
  				age: int @index(int) @upsert .
  				friend: [uid] @count @reverse .
			`,
			noError: true,
		},
		{
			name: "test duplicate predicates error",
			schema: `
				name: string @index(fulltext, term) .
				age: int @index(int) @upsert .
				friend: [uid] @count @reverse .
				friend: [uid] @count @reverse .
			`,
			noError: false,
		},
		{
			name: "test duplicate predicates error 2",
			schema: `
				name: string @index(fulltext, term) .
				age: int @index(int) @upsert .
				friend: [uid] @count @reverse .
				friend: int @index(int) @count @reverse .
			`,
			noError: false,
		},
		{
			name: "test duplicate types error",
			schema: `
				name: string .
				age: int .
				type Person {
					name
				}
				type Person {
					age
				}
			`,
			noError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			op := makeOp(tc.schema)

			_, err := parseSchemaFromAlterOperation(ctx, op)

			if tc.noError {
				require.NoError(t, err, "Unexpected error for: %+v", tc.schema)
			} else {
				require.Error(t, err, "Expected an error: %+v", tc.schema)
			}
		})
	}

}
