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
	"fmt"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/x"
)

// ParseMutation parses a block into a mutation. Returns an object with a mutation or
// a txn block with mutation, otherwise returns nil with an error.
func ParseMutation(mutation string) (*api.Mutation, error) {
	lexer := lex.NewLexer(mutation)
	lexer.Run(lexIdentifyBlock)
	if err := lexer.ValidateResult(); err != nil {
		return nil, err
	}

	it := lexer.NewIterator()
	if !it.Next() {
		return nil, x.Errorf("Invalid mutation")
	}

	item := it.Item()
	switch item.Typ {
	case itemMutationTxnBLock:
		return ParseTxnBlock(it)
	case itemLeftCurl:
		return ParseMutationBlock(it)
	default:
		return nil, x.Errorf("Expected { at the start of block. Got: [%s]", item.Val)
	}
}

// ParseTxnBlock parses the txn block
func ParseTxnBlock(it *lex.ItemIterator) (*api.Mutation, error) {
	var mu *api.Mutation
	var res Result

	// ==>txn<=== {...}
	item := it.Item()
	if item.Typ != itemMutationTxnBLock {
		return nil, x.Errorf("Expected txn block. Got [%s]", item.Val)
	}

	// txn ===>{<=== ....}
	it.Next()
	item = it.Item()
	if item.Typ != leftCurl {
		return nil, x.Errorf("Expected { at the start of block. Got: [%s]", item.Val)
	}

	for it.Next() {
		item = it.Item()
		switch item.Typ {
		// txn {... ===>}<===
		case rightCurl:
			// TODO
			fmt.Printf("ready to return, but don't know what to do with res: %+v\n", res)
			return mu, nil
		// txn { ===>mutation<=== {...} query{...}}
		// txn { mutation{...} ===>query<==={...}}
		case itemMutationTxnBlockOp:
			var err error
			if item.Val == "query" {
				if res, err = ParseQuery(it, nil); err != nil {
					return nil, err
				}
			} else if item.Val == "mutation" {
				if mu, err = ParseMutationBlock(it); err != nil {
					return nil, err
				}
			} else {
				return nil, x.Errorf("should not reach here")
			}
		default:
			return nil, x.Errorf("unexpected token in txn block [%s]", item.Val)
		}
	}

	return nil, x.Errorf("Invalid txn block")
}

// ParseMutationBlock parses the mutation block
func ParseMutationBlock(it *lex.ItemIterator) (*api.Mutation, error) {
	var mu api.Mutation

	item := it.Item()
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
			if it.Next() && it.Item().Typ != lex.ItemEOF {
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
	return nil, x.Errorf("Invalid mutation.")
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
