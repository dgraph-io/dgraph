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

// ParseMutation parses a block into a mutation. Returns an object with a mutation or
// an upsert block with mutation, otherwise returns nil with an error.
func ParseMutation(mutation string) (mu *api.Mutation, err error) {
	lexer := lex.NewLexer(mutation)
	lexer.Run(lexIdentifyBlock)
	if err := lexer.ValidateResult(); err != nil {
		return nil, err
	}

	it := lexer.NewIterator()
	if !it.Next() {
		return nil, errors.Errorf("Invalid mutation")
	}

	item := it.Item()
	switch item.Typ {
	case itemUpsertBlock:
		if mu, err = parseUpsertBlock(it); err != nil {
			return nil, err
		}
	case itemLeftCurl:
		if mu, err = parseMutationBlock(it); err != nil {
			return nil, err
		}
	default:
		return nil, errors.Errorf("Unexpected token: [%s]", item.Val)
	}

	// mutations must be enclosed in a single block.
	if it.Next() && it.Item().Typ != lex.ItemEOF {
		return nil, errors.Errorf("Unexpected %s after the end of the block", it.Item().Val)
	}

	return mu, nil
}

// parseUpsertBlock parses the upsert block
func parseUpsertBlock(it *lex.ItemIterator) (*api.Mutation, error) {
	var mu *api.Mutation
	var queryText string
	var queryFound bool

	// ===>upsert<=== {...}
	if !it.Next() {
		return nil, errors.Errorf("Unexpected end of upsert block")
	}

	// upsert ===>{<=== ....}
	item := it.Item()
	if item.Typ != itemLeftCurl {
		return nil, errors.Errorf("Expected { at the start of block. Got: [%s]", item.Val)
	}

	for it.Next() {
		item = it.Item()
		switch {
		// upsert {... ===>}<===
		case item.Typ == itemRightCurl:
			if mu == nil {
				return nil, errors.Errorf("Empty mutation block")
			} else if !queryFound {
				return nil, errors.Errorf("Query op not found in upsert block")
			} else {
				mu.Query = queryText
				return mu, nil
			}

		// upsert { mutation{...} ===>query<==={...}}
		case item.Typ == itemUpsertBlockOp && item.Val == "query":
			if queryFound {
				return nil, errors.Errorf("Multiple query ops inside upsert block")
			}
			queryFound = true
			if !it.Next() {
				return nil, errors.Errorf("Unexpected end of upsert block")
			}
			item = it.Item()
			if item.Typ != itemUpsertBlockOpContent {
				return nil, errors.Errorf("Expecting brace, found '%s'", item.Val)
			}
			queryText += item.Val

		// upsert { ===>mutation<=== {...} query{...}}
		case item.Typ == itemUpsertBlockOp && item.Val == "mutation":
			if !it.Next() {
				return nil, errors.Errorf("Unexpected end of upsert block")
			}

			// upsert ===>@<===if(...) {....}
			if it.Item().Typ == itemAt {
				if err := parseIfDirective(it); err != nil {
					return nil, err
				}

				// upsert @if(...===>)<=== {....}
				if !it.Next() {
					return nil, errors.Errorf("Unexpected end of upsert block")
				}
			}

			// upsert @if(...) ===>{<=== ....}
			var err error
			if mu, err = parseMutationBlock(it); err != nil {
				return nil, err
			}

		// upsert { mutation{...} ===>fragment<==={...}}
		case item.Typ == itemUpsertBlockOp && item.Val == "fragment":
			if !it.Next() {
				return nil, errors.Errorf("Unexpected end of upsert block")
			}
			item = it.Item()
			if item.Typ != itemUpsertBlockOpContent {
				return nil, errors.Errorf("Expecting brace, found '%s'", item.Val)
			}
			queryText += "fragment" + item.Val

		default:
			return nil, errors.Errorf("Unexpected token in upsert block [%s]", item.Val)
		}
	}

	return nil, errors.Errorf("Invalid upsert block")
}

// parseIfDirective parses the @if directive in mutation
func parseIfDirective(it *lex.ItemIterator) error {
	if !it.Next() {
		return errors.Errorf("Unexpected end of @if directive in upsert block")
	}

	// upsert @===>if<===(...) {....}
	item := it.Item()
	if item.Typ != itemName || item.Val != "if" {
		return errors.Errorf("Expected [if], Got: [%s]", item.Val)
	}

	// TODO(Aman): Construct Tree/Graph to evaluate if condition.
	depth := 0
	for it.Next() {
		item = it.Item()
		switch item.Typ {
		case itemLeftRound:
			depth++
		case itemRightRound:
			depth--
			if depth == 0 {
				return nil
			}
		case itemName:
			continue
		case itemComma:
			continue
		default:
			return errors.Errorf("Unexpected token in @if directive [%s]", item.Val)
		}
	}

	return errors.Errorf("Invalid @if directive in upsert block")
}

// parseMutationBlock parses the mutation block
func parseMutationBlock(it *lex.ItemIterator) (*api.Mutation, error) {
	var mu api.Mutation

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
		if item.Typ == itemMutationOpContent {
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
