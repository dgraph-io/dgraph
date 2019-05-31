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
	"github.com/pkg/errors"
)

func ParseMutation(mutation string) (*api.Mutation, error) {
	lexer := lex.NewLexer(mutation)
	lexer.Run(lexInsideMutation)
	it := lexer.NewIterator()
	var mu api.Mutation

	if !it.Next() {
		return nil, errors.New("Invalid mutation")
	}
	item := it.Item()
	if item.Typ != itemLeftCurl {
		return nil, errors.Errorf("Expected { at the start of block. Got: [%s]", item.Val)
	}

	for it.Next() {
		item := it.Item()
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemRightCurl {
			// mutations must be enclosed in a single block.
			if it.Next() && it.Item().Typ != lex.ItemEOF {
				return nil, errors.Errorf("Unexpected %s after the end of the block.", it.Item().Val)
			}
			return &mu, nil
		}
		if item.Typ == itemMutationOp {
			if err := parseMutationOp(it, item.Val, &mu); err != nil {
				return nil, err
			}
		}
	}
	return nil, errors.Errorf("Invalid mutation.")
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
				return errors.Errorf("Too many left curls in set mutation.")
			}
			parse = true
		}
		if item.Typ == itemMutationContent {
			if !parse {
				return errors.Errorf("Mutation syntax invalid.")
			}
			if op == "set" {
				mu.SetNquads = []byte(item.Val)
			} else if op == "delete" {
				mu.DelNquads = []byte(item.Val)
			} else if op == "schema" {
				return errors.Errorf("Altering schema not supported through http client.")
			} else if op == "dropall" {
				return errors.Errorf("Dropall not supported through http client.")
			} else {
				return errors.Errorf("Invalid mutation operation.")
			}
		}
		if item.Typ == itemRightCurl {
			return nil
		}
	}
	return errors.Errorf("Invalid mutation formatting.")
}
