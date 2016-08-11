/*
 * Copyright 2015 Manish R Jain <manishrjain@gmaicom>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gql

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/dgraph-io/dgraph/lex"
)

// GraphQuery stores the parsed Query in a tree format. This gets converted to
// internally used query.SubGraph before processing the query.
type GraphQuery struct {
	UID      uint64
	XID      string
	Attr     string
	First    int
	Offset   int
	After    uint64
	Children []*GraphQuery
}

// Mutation stores the strings corresponding to set and delete operations.
type Mutation struct {
	Set string
	Del string
}

// pair denotes the key value pair that is part of the GraphQL query root in parenthesis.
type pair struct {
	Key string
	Val string
}

// run is used to run the lexer until we encounter nil state.
func run(l *lex.Lexer) {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.Items) // No more tokens.
}

// Parse initializes and runs the lexer. It also constructs the GraphQuery subgraph
// from the lexed items.
func Parse(input string) (gq *GraphQuery, mu *Mutation, rerr error) {
	l := &lex.Lexer{}
	l.Init(input)
	go run(l)

	mu = nil
	gq = nil
	for item := range l.Items {
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemOpType {
			if item.Val == "mutation" {
				if mu != nil {
					return nil, nil, errors.New("Only one mutation block allowed.")
				}
				mu, rerr = getMutation(l)
				if rerr != nil {
					return nil, nil, rerr
				}
			}
		}
		if item.Typ == itemLeftCurl {
			if gq == nil {
				gq, rerr = getRoot(l)
				if rerr != nil {
					return nil, nil, rerr
				}
			} else {
				if err := godeep(l, gq); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return gq, mu, nil
}

// getMutation function parses and stores the set and delete operation in Mutation.
func getMutation(l *lex.Lexer) (mu *Mutation, rerr error) {
	for item := range l.Items {
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemLeftCurl {
			mu = new(Mutation)
		}
		if item.Typ == itemRightCurl {
			return mu, nil
		}
		if item.Typ == itemMutationOp {
			if err := parseMutationOp(l, item.Val, mu); err != nil {
				return nil, err
			}
		}
	}
	return nil, errors.New("Invalid mutation.")
}

// parseMutationOp parses and stores set or delete operation string in Mutation.
func parseMutationOp(l *lex.Lexer, op string, mu *Mutation) error {
	if mu == nil {
		return errors.New("Mutation is nil.")
	}

	parse := false
	for item := range l.Items {
		if item.Typ == itemText {
			continue
		}
		if item.Typ == itemLeftCurl {
			if parse {
				return errors.New("Too many left curls in set mutation.")
			}
			parse = true
		}
		if item.Typ == itemMutationContent {
			if !parse {
				return errors.New("Mutation syntax invalid.")
			}
			if op == "set" {
				mu.Set = item.Val
			} else if op == "delete" {
				mu.Del = item.Val
			} else {
				return errors.New("Invalid mutation operation.")
			}
		}
		if item.Typ == itemRightCurl {
			return nil
		}
	}
	return errors.New("Invalid mutation formatting.")
}

// parseArguments parses the arguments part of the GraphQL query root.
func parseArguments(l *lex.Lexer) (result []pair, rerr error) {
	for {
		var p pair

		// Get key
		item := <-l.Items
		if item.Typ == itemArgName {
			p.Key = item.Val

		} else if item.Typ == itemRightRound {
			break

		} else {
			return result, fmt.Errorf("Expecting argument name. Got: %v", item)
		}

		// Get value
		item = <-l.Items
		if item.Typ != itemArgVal {
			return result, fmt.Errorf("Expecting argument value. Got: %v", item)
		}

		p.Val = item.Val
		result = append(result, p)
	}
	return result, nil
}

// getRoot gets the root graph query object after parsing the args.
func getRoot(l *lex.Lexer) (gq *GraphQuery, rerr error) {
	gq = new(GraphQuery)
	item := <-l.Items
	if item.Typ != itemName {
		return nil, fmt.Errorf("Expected some name. Got: %v", item)
	}

	gq.Attr = item.Val
	item = <-l.Items
	if item.Typ != itemLeftRound {
		return nil, fmt.Errorf("Expected variable start. Got: %v", item)
	}

	args, err := parseArguments(l)
	if err != nil {
		return nil, err
	}
	for _, p := range args {
		if p.Key == "_uid_" {
			gq.UID, rerr = strconv.ParseUint(p.Val, 0, 64)
			if rerr != nil {
				return nil, rerr
			}
		} else if p.Key == "_xid_" {
			gq.XID = p.Val
		} else {
			return nil, fmt.Errorf("Expecting _uid_ or _xid_. Got: %+v", p)
		}
	}

	return gq, nil
}

// godeep constructs the subgraph from the lexed items and a GraphQuery node.
func godeep(l *lex.Lexer, gq *GraphQuery) error {
	curp := gq // Used to track current node, for nesting.
	for item := range l.Items {
		if item.Typ == lex.ItemError {
			return errors.New(item.Val)
		}

		if item.Typ == lex.ItemEOF {
			return nil
		}

		if item.Typ == itemRightCurl {
			return nil
		}

		if item.Typ == itemName {
			child := new(GraphQuery)
			child.Attr = item.Val
			gq.Children = append(gq.Children, child)
			curp = child

		} else if item.Typ == itemLeftCurl {
			if err := godeep(l, curp); err != nil {
				return err
			}

		} else if item.Typ == itemLeftRound {
			args, err := parseArguments(l)
			if err != nil {
				return err
			}
			// Stores args in GraphQuery, will be used later while retrieving results.
			for _, p := range args {
				if p.Key == "first" {
					count, err := strconv.ParseInt(p.Val, 0, 32)
					if err != nil {
						return err
					}
					curp.First = int(count)
				}
				if p.Key == "offset" {
					count, err := strconv.ParseInt(p.Val, 0, 32)
					if err != nil {
						return err
					}
					if count < 0 {
						return errors.New("offset cannot be less than 0")
					}
					curp.Offset = int(count)
				}
				if p.Key == "after" {
					afterUid, err := strconv.ParseUint(p.Val, 0, 64)
					if err != nil {
						return err
					}
					curp.After = afterUid
				}
			}
		}
	}
	return nil
}
