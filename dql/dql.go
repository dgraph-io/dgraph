/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package dql

import (
	"github.com/dgraph-io/dgo/v250/protos/api"

	"github.com/hypermodeinc/dgraph/v25/lex"
)

func ParseDQL(dqlQuery string) (*api.Request, error) {
	var lexer lex.Lexer
	lexer.Reset(dqlQuery)
	lexer.Run(lexTopLevel)
	if err := lexer.ValidateResult(); err != nil {
		return nil, err
	}

	it := lexer.NewIterator()
	if !it.Next() {
		return nil, it.Errorf("Invalid mutation")
	}

	item := it.Item()
	switch item.Typ {
	case itemUpsertBlock:
		it.Prev()
		return parseUpsertBlock(it)

	default:
		it.Next()
		item = it.Item()
		if item.Typ == itemMutationOp {
			it.Prev()
			it.Prev()
			mu, err := parseMutationBlock(it)
			if err != nil {
				return nil, err
			}
			return &api.Request{Mutations: []*api.Mutation{mu}}, nil
		}

		it.Prev()
		it.Prev()
		return &api.Request{Query: dqlQuery}, nil
	}
}
