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
	"strings"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/pkg/errors"
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

	for _, update := range result.Preds {
		State().Set(update.Predicate, update)
	}
	return nil
}

func parseDirective(it *lex.ItemIterator, schema *pb.SchemaUpdate, t types.TypeID) error {
	it.Next()
	next := it.Item()
	if next.Typ != itemText {
		return next.Errorf("Missing directive name")
	}
	switch next.Val {
	case "reverse":
		if t != types.UidID {
			return next.Errorf("Cannot reverse for non-UID type")
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
	case "noconflict":
		schema.NoConflict = true
	case "lang":
		if t != types.StringID || schema.List {
			return next.Errorf("@lang directive can only be specified for string type."+
				" Got: [%v] for attr: [%v]", t.Name(), schema.Predicate)
		}
		schema.Lang = true
	default:
		return next.Errorf("Invalid index specification")
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
		return nil, next.Errorf("Invalid '@' in name")
	case next.Typ != itemColon:
		return nil, next.Errorf("Missing colon")
	case !it.Next():
		return nil, next.Errorf("Invalid ending while trying to parse schema.")
	}
	next = it.Item()
	schema := &pb.SchemaUpdate{Predicate: predicate}
	// Could be list type.
	if next.Typ == itemLeftSquare {
		schema.List = true
		if !it.Next() {
			return nil, next.Errorf("Invalid ending while trying to parse schema.")
		}
		next = it.Item()
	}

	if next.Typ != itemText {
		return nil, next.Errorf("Missing Type")
	}
	typ := strings.ToLower(next.Val)
	// We ignore the case for types.
	t, ok := types.TypeForName(typ)
	if !ok {
		return nil, next.Errorf("Undefined Type")
	}
	if schema.List {
		if uint32(t) == uint32(types.PasswordID) || uint32(t) == uint32(types.BoolID) {
			return nil, next.Errorf("Unsupported type for list: [%s].", types.TypeID(t).Name())
		}
	}
	schema.ValueType = t.Enum()

	// Check for index / reverse.
	it.Next()
	next = it.Item()
	if schema.List {
		if next.Typ != itemRightSquare {
			return nil, next.Errorf("Unclosed [ while parsing schema for: %s", predicate)
		}
		if !it.Next() {
			return nil, next.Errorf("Invalid ending")
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
		return nil, next.Errorf("Invalid ending")
	}
	it.Next()
	next = it.Item()
	if next.Typ == lex.ItemEOF {
		it.Prev()
		return schema, nil
	}
	if next.Typ != itemNewLine {
		return nil, next.Errorf("Invalid ending")
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
		return tokenizers, it.Item().Errorf("Indexing not allowed on predicate %s of type %s",
			predicate, typ.Name())
	}
	if !it.Next() {
		// Nothing to read.
		return []string{}, it.Item().Errorf("Invalid ending.")
	}
	next := it.Item()
	if next.Typ != itemLeftRound {
		it.Prev() // Backup.
		return []string{}, it.Item().Errorf("Require type of tokenizer for pred: %s for indexing.",
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
				return nil, next.Errorf("Expected a tokenizer but got comma")
			}
			expectArg = true
			continue
		}
		if next.Typ != itemText {
			return tokenizers, next.Errorf("Expected directive arg but got: %v", next.Val)
		}
		if !expectArg {
			return tokenizers, next.Errorf("Expected a comma but got: %v", next)
		}
		// Look for custom tokenizer.
		tokenizer, has := tok.GetTokenizer(strings.ToLower(next.Val))
		if !has {
			return tokenizers, next.Errorf("Invalid tokenizer %s", next.Val)
		}
		tokenizerType, ok := types.TypeForName(tokenizer.Type())
		x.AssertTrue(ok) // Type is validated during tokenizer loading.
		if tokenizerType != typ {
			return tokenizers,
				next.Errorf("Tokenizer: %s isn't valid for predicate: %s of type: %s",
					tokenizer.Name(), predicate, typ.Name())
		}
		if _, found := seen[tokenizer.Name()]; found {
			return tokenizers, next.Errorf("Duplicate tokenizers defined for pred %v",
				predicate)
		}
		if tokenizer.IsSortable() {
			if seenSortableTok {
				return nil, next.Errorf("More than one sortable index encountered for: %v",
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
			return errors.Errorf("Indexing not allowed on predicate %s of type %s",
				schema.Predicate, typ.Name())
		}

		if typ == types.UidID {
			continue
		}

		if len(schema.Tokenizer) == 0 && schema.Directive == pb.SchemaUpdate_INDEX {
			return errors.Errorf("Require type of tokenizer for pred: %s of type: %s for indexing.",
				schema.Predicate, typ.Name())
		} else if len(schema.Tokenizer) > 0 && schema.Directive != pb.SchemaUpdate_INDEX {
			return errors.Errorf("Tokenizers present without indexing on attr %s", schema.Predicate)
		}
		// check for valid tokeniser types and duplicates
		var seen = make(map[string]bool)
		var seenSortableTok bool
		for _, t := range schema.Tokenizer {
			tokenizer, has := tok.GetTokenizer(t)
			if !has {
				return errors.Errorf("Invalid tokenizer %s", t)
			}
			tokenizerType, ok := types.TypeForName(tokenizer.Type())
			x.AssertTrue(ok) // Type is validated during tokenizer loading.
			if tokenizerType != typ {
				return errors.Errorf("Tokenizer: %s isn't valid for predicate: %s of type: %s",
					tokenizer.Name(), schema.Predicate, typ.Name())
			}
			if _, ok := seen[tokenizer.Name()]; !ok {
				seen[tokenizer.Name()] = true
			} else {
				return errors.Errorf("Duplicate tokenizers present for attr %s", schema.Predicate)
			}
			if tokenizer.IsSortable() {
				if seenSortableTok {
					return errors.Errorf("More than one sortable index encountered for: %v",
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
	if it.Item().Typ != itemText || it.Item().Val != "type" {
		return nil, it.Item().Errorf("Expected type keyword. Got %v", it.Item().Val)
	}

	it.Next()
	if it.Item().Typ != itemText {
		return nil, it.Item().Errorf("Expected type name. Got %v", it.Item().Val)
	}
	typeUpdate := &pb.TypeUpdate{TypeName: it.Item().Val}

	it.Next()
	if it.Item().Typ != itemLeftCurl {
		return nil, it.Item().Errorf("Expected {. Got %v", it.Item().Val)
	}

	var fields []*pb.SchemaUpdate
	for it.Next() {
		item := it.Item()

		switch item.Typ {
		case itemRightCurl:
			it.Next()
			if it.Item().Typ != itemNewLine && it.Item().Typ != lex.ItemEOF {
				return nil, it.Item().Errorf(
					"Expected new line or EOF after type declaration. Got %v", it.Item())
			}
			it.Prev()

			fieldSet := make(map[string]struct{})
			for _, field := range fields {
				if _, ok := fieldSet[field.GetPredicate()]; ok {
					return nil, it.Item().Errorf("Duplicate fields with name: %s",
						field.GetPredicate())
				}

				fieldSet[field.GetPredicate()] = struct{}{}
			}

			typeUpdate.Fields = fields
			return typeUpdate, nil
		case itemText:
			field, err := parseTypeField(it, typeUpdate.TypeName)
			if err != nil {
				return nil, err
			}
			fields = append(fields, field)
		case itemNewLine:
			// Ignore empty lines.
		default:
			return nil, it.Item().Errorf("Unexpected token. Got %v", it.Item().Val)
		}
	}
	return nil, errors.Errorf("Shouldn't reach here.")
}

func parseTypeField(it *lex.ItemIterator, typeName string) (*pb.SchemaUpdate, error) {
	field := &pb.SchemaUpdate{Predicate: it.Item().Val}
	var list bool
	it.Next()

	// Simplified type definitions only require the field name. If a new line is found,
	// proceed to the next field in the type.
	if it.Item().Typ == itemNewLine {
		return field, nil
	}

	// For the sake of backwards-compatibility, process type definitions in the old format,
	// but ignore the information after the colon.
	if it.Item().Typ != itemColon {
		return nil, it.Item().Errorf("Missing colon in type declaration. Got %v", it.Item().Val)
	}

	it.Next()
	if it.Item().Typ == itemLeftSquare {
		list = true
		it.Next()
	}

	if it.Item().Typ != itemText {
		return nil, it.Item().Errorf("Missing field type in type declaration. Got %v",
			it.Item().Val)
	}

	it.Next()
	if it.Item().Typ == itemExclamationMark {
		it.Next()
	}

	if list {
		if it.Item().Typ != itemRightSquare {
			return nil, it.Item().Errorf("Expected matching square bracket. Got %v", it.Item().Val)
		}
		it.Next()

		if it.Item().Typ == itemExclamationMark {
			it.Next()
		}
	}

	if it.Item().Typ != itemNewLine {
		return nil, it.Item().Errorf("Expected new line after field declaration. Got %v",
			it.Item().Val)
	}

	glog.Warningf("Type declaration for type %s includes deprecated information about field type "+
		"for field %s which will be ignored.", typeName, field.Predicate)
	return field, nil
}

// ParsedSchema represents the parsed schema and type updates.
type ParsedSchema struct {
	Preds []*pb.SchemaUpdate
	Types []*pb.TypeUpdate
}

func isTypeDeclaration(item lex.Item, it *lex.ItemIterator) bool {
	if item.Val != "type" {
		return false
	}

	nextItems, err := it.Peek(2)
	switch {
	case err != nil || len(nextItems) != 2:
		return false

	case nextItems[0].Typ != itemText:
		return false

	case nextItems[1].Typ != itemLeftCurl:
		return false
	}

	return true
}

// Parse parses a schema string and returns the schema representation for it.
func Parse(s string) (*ParsedSchema, error) {
	var result ParsedSchema

	var l lex.Lexer
	l.Reset(s)
	l.Run(lexText)
	if err := l.ValidateResult(); err != nil {
		return nil, err
	}
	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case lex.ItemEOF:
			if err := resolveTokenizers(result.Preds); err != nil {
				return nil, errors.Wrapf(err, "failed to enrich schema")
			}
			return &result, nil

		case itemText:
			if isTypeDeclaration(item, it) {
				typeUpdate, err := parseTypeDeclaration(it)
				if err != nil {
					return nil, err
				}
				result.Types = append(result.Types, typeUpdate)
				continue
			}

			schema, err := parseScalarPair(it, item.Val)
			if err != nil {
				return nil, err
			}
			result.Preds = append(result.Preds, schema)
		case itemNewLine:
			// pass empty line

		default:
			return nil, it.Item().Errorf("Unexpected token: %v while parsing schema", item)
		}
	}
	return nil, errors.Errorf("Shouldn't reach here")
}
