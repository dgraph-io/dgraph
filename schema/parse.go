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
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// Parse parses the schema file.
func Parse(file string) (rerr error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return x.Errorf("Error reading file: %v", err)
	}
	return ParseBytes(b)
}

// ParseBytes parses the byte array which holds the schema. We will reset
// all the globals.
func ParseBytes(schema []byte) (rerr error) {
	reset()
	s := string(schema)

	l := lex.NewLexer(s).Run(lexText)

	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		if item.Typ == lex.ItemEOF {
			break
		}
		if item.Typ != itemText {
			return x.Errorf("Expected text here but got: [%v] %v %v", item.Val, item.Typ, lex.EOF)
		}
		switch item.Val {
		case "scalar":
			if rerr = processScalar(it); rerr != nil {
				return rerr
			}
		case "type":
			if rerr = processObject(it); rerr != nil {
				return rerr
			}
		default:
			return x.Errorf("Expected either 'scalar' or 'type' but got: %v", item)
		}
	}
	return nil
}

// processScalarBlock starts work on the inside of a scalar block.
func processScalarBlock(it *lex.ItemIterator) error {
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemRightRound:
			return nil
		case itemText:
			if err := processScalarPair(it, item.Val, true); err != nil {
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
// consumed and is provided as input.
func processScalarPair(it *lex.ItemIterator, name string, allowIndex bool) error {
	it.Next()
	if next := it.Item(); next.Typ != itemColon {
		return x.Errorf("Missing colon")
	}

	it.Next()
	next := it.Item()
	if next.Typ != itemText {
		return x.Errorf("Missing Type")
	}
	typ := next.Val
	t, ok := types.TypeForName(typ)
	if ok {
		if t1, ok := str.getSchema(name); ok {
			if t1 != t {
				return x.Errorf("Same field cannot have multiple types")
			}
		} else {
			str.updateSchema(name, t)
		}
	}

	// Check for index / reverse.
	for it.Next() {
		next = it.Item()
		if next.Typ == lex.ItemError {
			return x.Errorf(next.Val)
		}
		if next.Typ == itemAt {
			it.Next()
			next = it.Item()
			if next.Typ != itemText {
				return x.Errorf("Missing directive name")
			}
			switch next.Val {
			case "reverse":
				if t != types.UidID {
					return x.Errorf("Cannot reverse for non-UID type")
				}
				reversedFields[name] = true
				return nil
			case "index":
				if !allowIndex {
					return x.Errorf("@index not allowed")
				}
				return processIndexDirective(it, name, t)
			default:
				return x.Errorf("Invalid index specification")
			}
		}
		it.Prev()
		break
	}
	return nil
}

// processIndexDirective works on "@index" or "@index(customtokenizer)".
func processIndexDirective(it *lex.ItemIterator, name string, typ types.TypeID) error {
	indexedFields[name] = tok.Default(typ)
	if !it.Next() {
		// Nothing to read.
		return nil
	}
	next := it.Item()
	if next.Typ != itemLeftRound {
		it.Prev() // Backup.
		return nil
	}

	// Look for tokenizer.
	var hasArg bool
	for {
		it.Next()
		next = it.Item()
		if next.Typ == itemRightRound {
			break
		}
		if next.Typ != itemText {
			return x.Errorf("Expected directive arg but got: %v", next)
		}
		// We have the argument.
		if hasArg {
			return x.Errorf("Found more than one arguments for index directive")
		}
		// Look for custom tokenizer.
		indexedFields[name] = tok.GetTokenizer(next.Val)
	}
	return nil
}

// processScalar works on either a single scalar pair or a scalar block.
// A scalar block looks like "scalar ( .... )".
func processScalar(it *lex.ItemIterator) error {
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemLeftRound:
			return processScalarBlock(it)
		case itemText:
			return processScalarPair(it, item.Val, true)
		case lex.ItemError:
			return x.Errorf(item.Val)
		default:
			return x.Errorf("Unexpected item: %v", item)
		}
	}
	return nil
}

// processObject works on "type { ... }".
func processObject(it *lex.ItemIterator) error {
	var objName string
	it.Next()
	next := it.Item()
	if next.Typ != itemText {
		return x.Errorf("Missing object name")
	}
	objName = next.Val
	str.updateSchema(objName, types.UidID)

	it.Next()
	next = it.Item()
	if next.Typ != itemLeftCurl {
		return x.Errorf("Missing left curly brace")
	}

	fieldCount := 0

L:
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemRightCurl:
			break L
		case itemText:
			if err := processScalarPair(it, item.Val, false); err != nil {
				return err
			}
			fieldCount++
		case lex.ItemError:
			return x.Errorf(item.Val)
		}
	}
	if fieldCount == 0 {
		return x.Errorf("Object type %v with no fields", objName)
	}
	return nil
}
