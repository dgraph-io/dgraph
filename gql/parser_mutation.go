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
	"github.com/golang/glog"
)

func ParseMutation(mutation string) (*api.Mutation, error) {
	lexer := lex.NewLexer(mutation)
	lexer.Run(lexInsideMutation)
	it := lexer.NewIterator()
	var mu api.Mutation

	var inTxn bool
	for it.Next() {
		item := it.Item()
		glog.Infof("*** ParseMutation ITEM: %+v", item)
		switch item.Typ {
		case itemText:
		case itemMutationTxn:
			glog.Infof("**** itemMutationTxn: %+v", item)
			if inTxn {
				return nil, x.Errorf("Too many txn blocks")
			}
			if !it.Next() { // eat '{'
				return nil, x.Errorf("Empty txn block")
			}
		case itemMutationTxnOp:
			glog.Infof("**** itemMutationTxnOp: %+v", item)
			peek, ok := it.PeekOne()
			if !ok {
				return nil, x.Errorf("Invalid query block")
			}
			glog.Infof("**** itemMutationTxnOp ITEM: %+v", peek)
			if peek.Typ != itemLeftCurl {
				return nil, x.Errorf("Expected { at the start of block. Got: [%s]!", item.Val)
			}
			if err := parseTxnQueryOp(it); err != nil {
				return nil, err
			}

		case itemMutationOp:
			glog.Infof("**** itemMutationOp: %+v", item)
			item := it.Item()
			if item.Typ != itemLeftCurl {
				return nil, x.Errorf("Expected { at the start of block. Got: [%s]", item.Val)
			}
			if err := parseMutationOp(it, item.Val, &mu); err != nil {
				return nil, err
			}

		case itemRightCurl:
			// mutations must be enclosed in a single block.
			if it.Next() && it.Item().Typ != lex.ItemEOF {
				return nil, x.Errorf("Unexpected %s after the end of the block.", it.Item().Val)
			}
			return &mu, nil
		}
	}
	return nil, x.Errorf("Invalid mutation.")
}

func parseTxnQueryOp(it *lex.ItemIterator) error {
	// var parse bool
	for it.Next() {
		item := it.Item()
		glog.Infof("QUERY ITEM: %+v", item)
		if item.Typ == itemText {
			continue
		}
		// if item.Typ == itemLeftCurl {
		// 	if parse {
		// 		return nil, x.Errorf("Too many left curls in set mutation.")
		// 	}
		// 	parse = true
		// }
		if item.Typ == itemMutationContent {
			// 	if !parse {
			// 		return nil, x.Errorf("Mutation syntax invalid.")
			// 	}
			glog.Infof("*** itemMutationContent: %+v", item)
			return nil
		}
	}
	return x.Errorf("Invalid query formatting.")
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
