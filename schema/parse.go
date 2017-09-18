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
	"strings"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// ParseBytes parses the byte array which holds the schema. We will reset
// all the globals.
// Overwrites schema blindly - called only during initilization in testing
func ParseBytes(s []byte, gid uint32) (rerr error) {
	if pstate == nil {
		reset()
	}
	pstate.predicate = make(map[string]*protos.SchemaUpdate)
	updates, err := Parse(string(s))
	if err != nil {
		return err
	}

	for _, update := range updates {
		State().Set(update.Predicate, *update)
	}
	State().Set("_predicate_", protos.SchemaUpdate{
		ValueType: uint32(types.StringID),
		List:      true,
	})
	return nil
}

func parseDirective(it *lex.ItemIterator, schema *protos.SchemaUpdate, t types.TypeID) error {
	it.Next()
	next := it.Item()
	if next.Typ != itemText {
		return x.Errorf("Missing directive name")
	}
	switch next.Val {
	case "reverse":
		if t != types.UidID {
			return x.Errorf("Cannot reverse for non-UID type")
		}
		schema.Directive = protos.SchemaUpdate_REVERSE
	case "index":
		if tokenizer, err := parseIndexDirective(it, schema.Predicate, t); err != nil {
			return err
		} else {
			schema.Directive = protos.SchemaUpdate_INDEX
			schema.Tokenizer = tokenizer
		}
	case "count":
		schema.Count = true
	default:
		return x.Errorf("Invalid index specification")
	}
	it.Next()

	return nil
}

func parseScalarPair(it *lex.ItemIterator, predicate string) (*protos.SchemaUpdate,
	error) {
	it.Next()
	if next := it.Item(); next.Typ != itemColon {
		return nil, x.Errorf("Missing colon")
	}

	if !it.Next() {
		return nil, x.Errorf("Invalid ending while trying to parse schema.")
	}
	next := it.Item()
	schema := &protos.SchemaUpdate{Predicate: predicate, Explicit: true}
	// Could be list type.
	if next.Typ == itemLeftSquare {
		schema.List = true
		if !it.Next() {
			return nil, x.Errorf("Invalid ending while trying to parse schema.")
		}
		next = it.Item()
	}

	if next.Typ != itemText {
		return nil, x.Errorf("Missing Type")
	}
	typ := strings.ToLower(next.Val)
	// We ignore the case for types.
	t, ok := types.TypeForName(typ)
	if !ok {
		return nil, x.Errorf("Undefined Type")
	}
	if schema.List {
		if !t.IsScalar() {
			return nil, x.Errorf("Expected scalar type inside []. Got: [%s] for attr: [%s].",
				t.Name(), predicate)
		}
		// TODO - Add a test case for multiple Geo and ensure that it works.
		if uint32(t) == uint32(types.PasswordID) {
			return nil, x.Errorf("Password list type is not supported.")
		}
	}
	schema.ValueType = uint32(t)

	// Check for index / reverse.
	it.Next()
	next = it.Item()
	if schema.List {
		if next.Typ != itemRightSquare {
			return nil, x.Errorf("Unclosed [ while parsing schema for: %s", predicate)
		}
		if !it.Next() {
			return nil, x.Errorf("Invalid ending")
		}
		next = it.Item()
	}
	if next.Typ == itemAt {
		if err := parseDirective(it, schema, t); err != nil {
			return nil, err
		}
		next = it.Item()
	}
	// Check for another directive, we could have @count too.
	if next.Typ == itemAt {
		if err := parseDirective(it, schema, t); err != nil {
			return nil, err
		}
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
	var seenSortableTok bool

	if typ == types.UidID || typ == types.DefaultID || typ == types.PasswordID {
		return tokenizers, x.Errorf("Indexing not allowed on predicate %s of type %s",
			predicate, typ.Name())
	}
	if !it.Next() {
		// Nothing to read.
		return []string{}, x.Errorf("Invalid ending.")
	}
	next := it.Item()
	if next.Typ != itemLeftRound {
		it.Prev() // Backup.
		return []string{}, x.Errorf("Require type of tokenizer for pred: %s for indexing.",
			predicate)
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
		tokenizer, has := tok.GetTokenizer(strings.ToLower(next.Val))
		if !has {
			return tokenizers, x.Errorf("Invalid tokenizer %s", next.Val)
		}
		if tokenizer.Type() != typ {
			return tokenizers,
				x.Errorf("Tokenizer: %s isn't valid for predicate: %s of type: %s",
					tokenizer.Name(), predicate, typ.Name())
		}
		if _, found := seen[tokenizer.Name()]; found {
			return tokenizers, x.Errorf("Duplicate tokenizers defined for pred %v",
				predicate)
		}
		if tokenizer.IsSortable() {
			if seenSortableTok {
				return nil, x.Errorf("More than one sortable index encountered for: %v",
					predicate)
			}
			seenSortableTok = true
		}
		tokenizers = append(tokenizers, tokenizer.Name())
		seen[tokenizer.Name()] = true
		expectArg = false
	}
	return tokenizers, nil
}

// resolveTokenizers resolves default tokenizers and verifies tokenizers definitions.
func resolveTokenizers(updates []*protos.SchemaUpdate) error {
	for _, schema := range updates {
		typ := types.TypeID(schema.ValueType)

		if (typ == types.UidID || typ == types.DefaultID || typ == types.PasswordID) &&
			schema.Directive == protos.SchemaUpdate_INDEX {
			return x.Errorf("Indexing not allowed on predicate %s of type %s",
				schema.Predicate, typ.Name())
		}

		if typ == types.UidID {
			continue
		}

		if len(schema.Tokenizer) == 0 && schema.Directive == protos.SchemaUpdate_INDEX {
			return x.Errorf("Require type of tokenizer for pred: %s of type: %s for indexing.",
				schema.Predicate, typ.Name())
		} else if len(schema.Tokenizer) > 0 && schema.Directive != protos.SchemaUpdate_INDEX {
			return x.Errorf("Tokenizers present without indexing on attr %s", schema.Predicate)
		}
		// check for valid tokeniser types and duplicates
		var seen = make(map[string]bool)
		var seenSortableTok bool
		for _, t := range schema.Tokenizer {
			tokenizer, has := tok.GetTokenizer(t)
			if !has {
				return x.Errorf("Invalid tokenizer %s", t)
			}
			if tokenizer.Type() != typ {
				return x.Errorf("Tokenizer: %s isn't valid for predicate: %s of type: %s",
					tokenizer.Name(), schema.Predicate, typ.Name())
			}
			if _, ok := seen[tokenizer.Name()]; !ok {
				seen[tokenizer.Name()] = true
			} else {
				return x.Errorf("Duplicate tokenizers present for attr %s", schema.Predicate)
			}
			if tokenizer.IsSortable() {
				if seenSortableTok {
					return x.Errorf("More than one sortable index encountered for: %v",
						schema.Predicate)
				}
				seenSortableTok = true
			}
		}
	}
	return nil
}

// Parse parses a schema string and returns the schema representation for it.
func Parse(s string) ([]*protos.SchemaUpdate, error) {
	var schemas []*protos.SchemaUpdate
	l := lex.Lexer{Input: s}
	l.Run(lexText)
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemEOF:
			if err := resolveTokenizers(schemas); err != nil {
				return nil, x.Wrapf(err, "failed to enrich schema")
			}
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
