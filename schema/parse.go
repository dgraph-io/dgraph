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

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/graphp"
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

func From(s *graphp.SchemaUpdate) typesp.Schema {
	if s.Directive == graphp.SchemaUpdate_REVERSE {
		return typesp.Schema{
			ValueType: s.ValueType,
			Directive: typesp.Schema_REVERSE}
	} else if s.Directive == graphp.SchemaUpdate_INDEX {
		return typesp.Schema{
			ValueType: s.ValueType,
			Directive: typesp.Schema_INDEX,
			Tokenizer: s.Tokenizer}
	}
	return typesp.Schema{ValueType: s.ValueType}
}

// ParseBytes parses the byte array which holds the schema. We will reset
// all the globals.
// Overwrites schema blindly - called only during initilization in testing
func ParseBytes(s []byte, gid uint32) (rerr error) {
	reset()
	updates, err := Parse(string(s))
	if err != nil {
		return err
	}

	for _, update := range updates {
		State().Set(update.Predicate, From(update))
	}
	return nil
}

func parseScalarPair(it *lex.ItemIterator, predicate string,
	allowIndex bool) (*graphp.SchemaUpdate, error) {
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
				return &graphp.SchemaUpdate{
					Predicate: predicate,
					ValueType: uint32(t),
					Directive: graphp.SchemaUpdate_REVERSE,
				}, nil
			case "index":
				if !allowIndex {
					return nil, x.Errorf("@index not allowed")
				}
				if tokenizer, err := parseIndexDirective(it, predicate, t); err != nil {
					return nil, err
				} else {
					return &graphp.SchemaUpdate{
						Predicate: predicate, ValueType: uint32(t),
						Directive: graphp.SchemaUpdate_INDEX,
						Tokenizer: tokenizer,
					}, nil
				}
			default:
				return nil, x.Errorf("Invalid index specification")
			}
		}
		it.Prev()
		break
	}
	return &graphp.SchemaUpdate{Predicate: predicate, ValueType: uint32(t)}, nil
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

	expectArg := true
	// Look for tokenizers.
	for {
		it.Next()
		next = it.Item()
		if next.Typ == itemRightRound {
			break
		}
		if next.Typ == itemComma {
			if expectArg {
				return nil, x.Errorf("Expected a tokenizer but got comma")
			}
			expectArg = true
			continue
		}
		if next.Typ != itemText {
			return tokenizers, x.Errorf("Expected directive arg but got: %v", next.Val)
		}
		if !expectArg {
			return tokenizers, x.Errorf("Expected a comma but got: %v", next)
		}
		// Look for custom tokenizer.
		tokenizer, has := tok.GetTokenizer(next.Val)
		if !has {
			return tokenizers, x.Errorf("Invalid tokenizer %s", next.Val)
		}
		if _, found := seen[tokenizer.Name()]; found {
			return tokenizers, x.Errorf("Duplicate tokenizers defined for pred %v", predicate)
		} else {
			tokenizers = append(tokenizers, tokenizer.Name())
			seen[tokenizer.Name()] = true
		}
		expectArg = false
	}
	return tokenizers, nil
}

// Parse parses a schema string and returns the schema representation for it.
func Parse(s string) ([]*graphp.SchemaUpdate, error) {
	var schemas []*graphp.SchemaUpdate
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
