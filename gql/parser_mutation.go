/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package gql

import (
	"errors"
	"fmt"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/x"
)

func ParseMutation(mutation string) (*api.Mutation, error) {
	lexer := lex.Lexer{Input: mutation}
	lexer.Run(lexInsideMutation)
	it := lexer.NewIterator()
	var mu *api.Mutation

	if !it.Next() {
		return nil, errors.New("Invalid mutation.")
	}
	item := it.Item()
	if item.Typ != itemLeftCurl {
		return nil, fmt.Errorf("Expected { at the start of block. Got: [%s]", item.Val)
	}
	mu = new(api.Mutation)

	for it.Next() {
		item := it.Item()
		if item.Typ == itemText {
			continue
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
func parseMutationOp(it *lex.ItemIterator, op string, mu *api.Mutation) error {
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
