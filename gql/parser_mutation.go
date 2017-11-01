/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package gql

import (
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

func ParseMutation(mutation string) (*protos.Mutation, error) {
	lexer := lex.Lexer{Input: mutation}
	lexer.Run(lexInsideMutation)
	it := lexer.NewIterator()
	var mu *protos.Mutation
	for it.Next() {
		item := it.Item()
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemLeftCurl {
			mu = new(protos.Mutation)
		}
		if item.Typ == itemRightCurl {
			return mu, nil
		}
		if item.Typ == itemMutationOp {
			if err := parseMutationOp(it, item.Val, mu); err != nil {
				return nil, err
			}
		}
	}
	return nil, x.Errorf("Invalid mutation.")
}

// parseMutationOp parses and stores set or delete operation string in Mutation.
func parseMutationOp(it *lex.ItemIterator, op string, mu *protos.Mutation) error {
	if mu == nil {
		return x.Errorf("Mutation is nil.")
	}

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
		if item.Typ == itemMutationContent {
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
