/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package schema

import (
	"log"
	"strings"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/pb"
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
	pstate.DeleteAll()
	result, err := Parse(string(s))
	if err != nil {
		return err
	}

	for _, update := range result.Schemas {
		State().Set(update.Predicate, *update)
	}
	State().Set("_predicate_", pb.SchemaUpdate{
		ValueType: pb.Posting_STRING,
		List:      true,
	})
	return nil
}

func parseDirective(it *lex.ItemIterator, schema *pb.SchemaUpdate, t types.TypeID) error {
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
		schema.Directive = pb.SchemaUpdate_REVERSE
	case "index":
		tokenizer, err := parseIndexDirective(it, schema.Predicate, t)
		if err != nil {
			return err
		}
		schema.Directive = pb.SchemaUpdate_INDEX
		schema.Tokenizer = tokenizer
	case "count":
		schema.Count = true
	case "upsert":
		schema.Upsert = true
	case "lang":
		if t != types.StringID || schema.List {
			return x.Errorf("@lang directive can only be specified for string type."+
				" Got: [%v] for attr: [%v]", t.Name(), schema.Predicate)
		}
		schema.Lang = true
	default:
		return x.Errorf("Invalid index specification")
	}
	it.Next()

	return nil
}

func parseScalarPair(it *lex.ItemIterator, predicate string) (*pb.SchemaUpdate, error) {
	it.Next()
	next := it.Item()
	switch {
	// This check might seem redundant but it's necessary. We have two possibilities,
	//   1) that the schema is of form: name@en: string .
	//
	//   2) or this alternate form: <name@en>: string .
	//
	// The itemAt test invalidates 1) and string.Contains() tests for 2). We don't allow
	// '@' in predicate names, so both forms are disallowed. Handling them here avoids
	// messing with the lexer and IRI values.
	case next.Typ == itemAt || strings.Contains(predicate, "@"):
		return nil, x.Errorf("Invalid '@' in name")
	case next.Typ != itemColon:
		return nil, x.Errorf("Missing colon")
	case !it.Next():
		return nil, x.Errorf("Invalid ending while trying to parse schema.")
	}
	next = it.Item()
	schema := &pb.SchemaUpdate{Predicate: predicate}
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
		if uint32(t) == uint32(types.PasswordID) || uint32(t) == uint32(types.BoolID) {
			return nil, x.Errorf("Unsupported type for list: [%s].", types.TypeID(t).Name())
		}
	}
	schema.ValueType = t.Enum()

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

	for {
		if next.Typ != itemAt {
			break
		}
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
		tokenizerType, ok := types.TypeForName(tokenizer.Type())
		x.AssertTrue(ok) // Type is validated during tokenizer loading.
		if tokenizerType != typ {
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
func resolveTokenizers(updates []*pb.SchemaUpdate) error {
	for _, schema := range updates {
		typ := types.TypeID(schema.ValueType)

		if (typ == types.UidID || typ == types.DefaultID || typ == types.PasswordID) &&
			schema.Directive == pb.SchemaUpdate_INDEX {
			return x.Errorf("Indexing not allowed on predicate %s of type %s",
				schema.Predicate, typ.Name())
		}

		if typ == types.UidID {
			continue
		}

		if len(schema.Tokenizer) == 0 && schema.Directive == pb.SchemaUpdate_INDEX {
			return x.Errorf("Require type of tokenizer for pred: %s of type: %s for indexing.",
				schema.Predicate, typ.Name())
		} else if len(schema.Tokenizer) > 0 && schema.Directive != pb.SchemaUpdate_INDEX {
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
			tokenizerType, ok := types.TypeForName(tokenizer.Type())
			x.AssertTrue(ok) // Type is validated during tokenizer loading.
			if tokenizerType != typ {
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

func parseTypeDeclaration(it *lex.ItemIterator) (*pb.TypeUpdate, error) {
	// Iterator is currently on the token corresponding to the keyword type.
	// Call Next to land on the type name.
	it.Next()
	typeUpdate := &pb.TypeUpdate{Name: it.Item().Val}
	log.Printf("Name: %v\n", it.Item().Val)

	// Call next again to skip the { character.
	it.Next()

	var fields []*pb.TypeField
	for {
		item := it.Item()
		switch item.Typ {
		case itemRightCurl:
			it.Next()
			if it.Item().Typ != itemNewLine {
				return nil, x.Errorf("Expected new line after type declaration. Got %v",
					it.Item().Val)
			}

			typeUpdate.Fields = fields
			return typeUpdate, nil
		case itemText:
			field, err := parseTypeField(it)
			if err != nil {
				return nil, err
			}
			fields = append(fields, field)
		default:
			it.Next()
		}
	}
}

func parseTypeField(it *lex.ItemIterator) (*pb.TypeField, error) {
	field := &pb.TypeField{Name: it.Item().Val}
	list := false

	it.Next()
	if it.Item().Typ != itemColon {
		return nil, x.Errorf("Missing colon in type declaration. Got %v", it.Item().Val)
	}

	it.Next()
	if it.Item().Typ == itemLeftSquare {
		list = true
		it.Next()
	}

	if it.Item().Typ != itemText {
		return nil, x.Errorf("Missing field type in type declaration. Got %v", it.Item().Val)
	}
	typ := getType(it.Item().Val)
	field.ValueType = typ
	if typ == pb.Posting_OBJECT {
		field.ObjectTypeName = it.Item().Val
	}

	it.Next()
	if it.Item().Typ == itemExclamationMark {
		field.NonNullable = true
		it.Next()
	}

	if list {
		if it.Item().Typ != itemRightSquare {
			return nil, x.Errorf("Expected matching square bracket. Got %v", it.Item().Val)
		}
		field.List = true
		it.Next()

		if it.Item().Typ == itemExclamationMark {
			field.NonNullableList = true
			it.Next()
		}
	}

	if it.Item().Typ != itemNewLine {
		return nil, x.Errorf("Expected new line after field declaration. Got %v", it.Item().Val)
	}

	it.Next()
	return field, nil
}

func getType(typeName string) pb.Posting_ValType {
	typ, ok := types.TypeForName(strings.ToLower(typeName))
	if ok {
		return pb.Posting_ValType(typ)
	}

	return pb.Posting_OBJECT
}

// SchemasAndTypes represents the parsed schema and type updates.
type SchemasAndTypes struct {
	Schemas []*pb.SchemaUpdate
	Types   []*pb.TypeUpdate
}

func isTypeDeclaration(item lex.Item, it *lex.ItemIterator) bool {
	if item.Val != "type" {
		return false
	}

	nextItems, err := it.Peek(2)
	if err != nil || len(nextItems) != 2 {
		return false
	}

	if nextItems[0].Typ != itemText {
		return false
	}

	if nextItems[1].Typ != itemLeftCurl {
		return false
	}

	return true
}

// Parse parses a schema string and returns the schema representation for it.
func Parse(s string) (SchemasAndTypes, error) {
	var result SchemasAndTypes
	var schemas []*pb.SchemaUpdate
	var types []*pb.TypeUpdate

	l := lex.NewLexer(s)
	l.Run(lexText)
	if err := l.ValidateResult(); err != nil {
		return result, err
	}
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemEOF:
			if err := resolveTokenizers(schemas); err != nil {
				return result, x.Wrapf(err, "failed to enrich schema")
			}
			result.Schemas = schemas
			result.Types = types
			return result, nil

		case itemText:
			if isTypeDeclaration(item, it) {
				typeUpdate, err := parseTypeDeclaration(it)
				if err != nil {
					return result, err
				}
				types = append(types, typeUpdate)
				continue
			}

			schema, err := parseScalarPair(it, item.Val)
			if err != nil {
				return result, err
			}
			schemas = append(schemas, schema)
		case itemNewLine:
			// pass empty line

		default:
			return result, x.Errorf("Unexpected token: %v while parsing schema", item)
		}
	}
	return result, x.Errorf("Shouldn't reach here")
}
