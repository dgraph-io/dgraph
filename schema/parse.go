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

package schema

import (
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

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
	pstate.m = make(map[uint32]*stateGroup)
	updates, err := Parse(string(s))
	if err != nil {
		return err
	}

	for _, update := range updates {
		State().Set(update.Predicate, From(update))
	}
	return nil
}

func parseScalarPair(it *lex.ItemIterator, predicate string) (*graphp.SchemaUpdate,
	error) {
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
	schema := &graphp.SchemaUpdate{Predicate: predicate, ValueType: uint32(t)}
	it.Next()
	next = it.Item()
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
			schema.Directive = graphp.SchemaUpdate_REVERSE
		case "index":
			if tokenizer, err := parseIndexDirective(it, predicate, t); err != nil {
				return nil, err
			} else {
				schema.Directive = graphp.SchemaUpdate_INDEX
				schema.Tokenizer = tokenizer
			}
		default:
			return nil, x.Errorf("Invalid index specification")
		}
		it.Next()
		next = it.Item()
	}
	if next.Typ != itemDot {
		return nil, x.Errorf("Invalid ending")
	}
	it.Next()
	next = it.Item()
	if next.Typ == lex.ItemEOF {
		it.Prev()
		return schema, nil
	}
	if next.Typ != itemNewLine {
		return nil, x.Errorf("Invalid ending")
	}
	return schema, nil
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
		if tokenizer.Type() != typ {
			return tokenizers, x.Errorf("Tokenizer: %s isn't valid for predicate: %s of type: %s",
				tokenizer.Name(), predicate, typ.Name())
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
			if schema, err := parseScalarPair(it, item.Val); err != nil {
				return nil, err
			} else {
				schemas = append(schemas, schema)
			}
		case lex.ItemError:
			return nil, x.Errorf(item.Val)
		case itemNewLine:
			// pass empty line
		default:
			return nil, x.Errorf("Unexpected token: %v", item)
		}
	}
	return nil, x.Errorf("Shouldn't reach here")
}
