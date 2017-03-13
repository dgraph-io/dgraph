/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package schema

import (
	"io/ioutil"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// Parse parses the schema file.
func parseFile(file string, gid uint32) (rerr error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return x.Errorf("Error reading file: %v", err)
	}
	return ParseBytes(b, gid)
}

// ParseBytes parses the byte array which holds the schema. We will reset
// all the globals.
// Overwrites schema blindly - called only during initilization in testing
func ParseBytes(schema []byte, gid uint32) (rerr error) {
	reset()
	s := string(schema)

	l := lex.NewLexer(s).Run(lexText)

	it := l.NewIterator()
	if rerr = processScalars(it, gid); rerr != nil {
		return rerr
	}
	return nil
}

func parseScalarPair(it *lex.ItemIterator, predicate string,
	allowIndex bool) (*typesp.Schema, error) {
	it.Next()
	if next := it.Item(); next.Typ != itemColon {
		return nil, x.Errorf("Missing colon")
	}

	it.Next()
	next := it.Item()
	if next.Typ != itemText {
		return nil, x.Errorf("Missing Type")
	}
	typ := next.Val
	t, ok := types.TypeForName(typ)
	if !ok {
		return nil, x.Errorf("Undefined Type")
	}

	// Check for index / reverse.
	for it.Next() {
		next = it.Item()
		if next.Typ == lex.ItemError {
			return nil, x.Errorf(next.Val)
		}
		if next.Typ == itemAt {
			it.Next()
			next = it.Item()
			if next.Typ != itemText {
				return nil, x.Errorf("Missing directive name")
			}
			switch next.Val {
			case "reverse":
				if t != types.UidID {
					return nil, x.Errorf("Cannot reverse for non-UID type")
				}
				return &typesp.Schema{ValueType: uint32(t), Reverse: true}, nil
			case "index":
				if !allowIndex {
					return nil, x.Errorf("@index not allowed")
				}
				if tokenizer, err := parseIndexDirective(it, predicate, t); err != nil {
					return nil, err
				} else {
					return &typesp.Schema{ValueType: uint32(t), Tokenizer: tokenizer}, nil
				}
			default:
				return nil, x.Errorf("Invalid index specification")
			}
		}
		it.Prev()
		break
	}
	return &typesp.Schema{ValueType: uint32(t)}, nil
}

// processScalars parses schema definitions line by line
func processScalars(it *lex.ItemIterator, gid uint32) error {
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemEOF:
			return nil
		case itemText:
			if err := processScalarPair(it, item.Val, true, gid); err != nil {
				return err
			}
		case lex.ItemError:
			return x.Errorf(item.Val)
		default:
			return x.Errorf("Unexpected token: %v", item)
		}
	}

	return nil
}

// processScalarPair processes "name: type (directive)" where name is already
// consumed and is provided as input in file during loading
func processScalarPair(it *lex.ItemIterator, predicate string, allowIndex bool, gid uint32) error {
	if schema, err := parseScalarPair(it, predicate, allowIndex); err != nil {
		return err
	} else {
		// Schema is already present for this predicate
		_, err := State().TypeOf(predicate)
		if err == nil {
			return x.Errorf("Multiple schema declarations for same predicate %s", predicate)
		}
		if group.BelongsTo(predicate) == gid {
			State().Set(predicate, schema)
		}
	}

	return nil
}

// parseIndexDirective works on "@index" or "@index(customtokenizer)".
func parseIndexDirective(it *lex.ItemIterator, predicate string,
	typ types.TypeID) ([]string, error) {
	var tokenizers []string
	var seen = make(map[string]bool)

	if typ == types.UidID {
		return tokenizers, x.Errorf("Indexing not allowed on predicate %s of type uid", predicate)
	}
	if !it.Next() {
		// Nothing to read.
		return []string{tok.Default(typ).Name()}, nil
	}
	next := it.Item()
	if next.Typ != itemLeftRound {
		it.Prev() // Backup.
		return []string{tok.Default(typ).Name()}, nil
	}

	// Look for tokenizers.
	for {
		it.Next()
		next = it.Item()
		if next.Typ == itemRightRound {
			break
		}
		if next.Typ != itemText {
			return tokenizers, x.Errorf("Expected directive arg but got: %v", next)
		}
		// Look for custom tokenizer.
		tokenizer := tok.GetTokenizer(next.Val).Name()
		if _, ok := seen[tokenizer]; !ok {
			tokenizers = append(tokenizers, tokenizer)
			seen[tokenizer] = true
		}
	}
	return tokenizers, nil
}

// Parse parses a schema string and returns the schema representation for it.
func Parse(s string) ([]*typesp.Schema, error) {
	var schemas []*typesp.Schema
	l := lex.NewLexer(s).Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemEOF:
			return schemas, nil
		case itemText:
			if schema, err := parseScalarPair(it, item.Val, true); err != nil {
				return nil, err
			} else {
				schemas = append(schemas, schema)
			}
		case lex.ItemError:
			return nil, x.Errorf(item.Val)
		default:
			return nil, x.Errorf("Unexpected token: %v", item)
		}
	}
	return nil, x.Errorf("Shouldn't reach here")
}
