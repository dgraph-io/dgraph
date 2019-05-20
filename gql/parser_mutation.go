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
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/x"
)

// ParseMutation parses a block of text into a mutation.
// Returns an object with a mutation or a transaction query
// with mutation, otherwise returns nil with an error.
// TODO (Aman): add examples.
func ParseMutation(mutation string) (*api.Mutation, error) {
	lexer := lex.NewLexer(mutation)
	lexer.Run(lexInsideMutation)
	it := lexer.NewIterator()
	var mu api.Mutation
	var inTxn bool

	if !it.Next() {
		return nil, x.Errorf("Invalid mutation")
	}
	item := it.Item()
	// Inside txn{ ... } block.
	// Here we switch into txn mode and try to fetch any query inside a txn block.
	// If no query text is found, this txn is a no-op.
	if item.Typ == itemMutationTxn {
		var err error
		// Get the query text: txn{ query { ... }}
		mu.CondQuery, err = parseMutationCondQuery(it)
		if err != nil {
			return nil, err
		}
		inTxn = true
		item = it.Item()
		// fallthrough to regular mutation parsing.
	}
	if item.Typ != itemLeftCurl {
		return nil, x.Errorf("Expected { at the start of block. Got: [%s]", item.Val)
	}

	for it.Next() {
		item := it.Item()
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemRightCurl {
			// mutations must be enclosed in a single block.
			if !inTxn && it.Next() && it.Item().Typ != lex.ItemEOF {
				return nil, x.Errorf("Unexpected %s after the end of the block.", it.Item().Val)
			}
			return &mu, nil
		}
		if item.Typ == itemMutationOp {
			if err := parseMutationOp(it, item.Val, &mu); err != nil {
				return nil, err
			}
		}
	}
	return nil, x.Errorf("Invalid mutation")
}

// parseMutationCondQuery gets the text inside a txn query block. It is possible that there's
// no query to be found, in that case it's the caller's responsbility to fail.
// Returns the query text if any is found, otherwise an empty string with error.
func parseMutationCondQuery(it *lex.ItemIterator) (string, error) {
	var query string
	var parse bool
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemLeftCurl:
			continue
		case itemMutationOpContent:
			if !parse {
				return "", x.Errorf("Invalid query block.")
			}
			query = item.Val
		case itemMutationTxnOp:
			if item.Val == "query" {
				if parse {
					return "", x.Errorf("Too many query blocks in txn")
				}
				parse = true
				continue
			}
			// TODO: mutation conditionals
			if item.Val != "mutation" {
				return "", x.Errorf("Invalid txn operator %q.", item.Val)
			}
			if !it.Next() {
				return "", x.Errorf("Invalid mutation block")
			}
			return query, nil
		default:
			return "", x.Errorf("Unexpected %q inside of txn block.", item.Val)
		}
	}
	return query, nil
}

// parseMutationOp parses and stores set or delete operation string in Mutation.
func parseMutationOp(it *lex.ItemIterator, op string, mu *api.Mutation) error {
	parse := false
	for it.Next() {
		item := it.Item()
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemLeftCurl {
			if parse {
				return x.Errorf("Too many left curls in set mutation.")
			}
			parse = true
		}
		if item.Typ == itemMutationOpContent {
			if !parse {
				return x.Errorf("Mutation syntax invalid.")
			}
			if op == "set" {
				mu.SetNquads = []byte(item.Val)
			} else if op == "delete" {
				mu.DelNquads = []byte(item.Val)
			} else if op == "schema" {
				return x.Errorf("Altering schema not supported through http client.")
			} else if op == "dropall" {
				return x.Errorf("Dropall not supported through http client.")
			} else {
				return x.Errorf("Invalid mutation operation.")
			}
		}
		if item.Typ == itemRightCurl {
			return nil
		}
	}
	return x.Errorf("Invalid mutation formatting.")
}
