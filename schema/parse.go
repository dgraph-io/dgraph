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

// ParseBytes parses the byte array which holds the schema.
func ParseBytes(schema []byte) (rerr error) {
	s := string(schema)

	l := lex.NewLexer(s).Run(lexText)

	it := l.NewIterator()
	for it.Next() {
		item := it.Item()
		if item.Typ != itemText {
			return x.Errorf("Expected text here but got: %v", item)
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

func processScalarBlock(it *lex.ItemIterator) error {
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemRightRound:
			return nil
		case itemText:
			{
				var name, typ string
				name = item.Val

				it.Next()
				if next := it.Item(); next.Typ != itemColon {
					return x.Errorf("Missing colon")
				}

				it.Next()
				next := it.Item()
				if next.Typ != itemText {
					return x.Errorf("Missing Type")
				}
				typ = next.Val

				t, ok := types.TypeForName(typ)
				if !ok {
					return x.Errorf("Invalid type")
				}
				str[name] = t

				// Check for index / reverse.
				for it.Next() {
					next = it.Item()
					if next.Typ == lex.ItemError {
						return x.Errorf(next.Val)
					}
					if next.Typ == itemDummy {
						break
					}
					if next.Typ == itemAt {
						it.Next()
						next = it.Item()
						if next.Typ != itemText {
							return x.Errorf("Missing directive name")
						}
						switch next.Val {
						case "index":
							if err := processIndexDirective(it, name, t); err != nil {
								return err
							}
						case "reverse":
							if t != types.UidID {
								return x.Errorf("Cannot reverse for non-UID type")
							}
							reversedFields[name] = true
						default:
							return x.Errorf("Invalid index specification")
						}
					}
				}
			}
		case lex.ItemError:
			return x.Errorf(item.Val)
		}
	}

	return nil
}

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

func processScalar(it *lex.ItemIterator) error {

	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemLeftRound:
			{
				return processScalarBlock(it)
			}
		case itemText:
			{
				var name, typ string
				name = item.Val

				it.Next()
				next := it.Item()
				if next.Typ != itemColon {
					return x.Errorf("Missing colon")
				}

				it.Next()
				next = it.Item()
				if next.Typ != itemText {
					return x.Errorf("Missing Type")
				}
				typ = next.Val

				t, ok := types.TypeForName(typ)
				if ok {
					str[name] = t
				} else {
					return x.Errorf("Invalid type")
				}

				// Check for index.
				for it.Next() {
					next = it.Item()
					if next.Typ == lex.ItemError {
						return x.Errorf(next.Val)
					}
					if next.Typ == itemDummy {
						break
					}
					if next.Typ == itemAt {
						it.Next()
						next = it.Item()
						if next.Typ != itemText {
							return x.Errorf("Missing directive name")
						}
						switch next.Val {
						case "index":
							if err := processIndexDirective(it, name, t); err != nil {
								return err
							}
						case "reverse":
							if t != types.UidID {
								return x.Errorf("Cannot reverse for non-UID type")
							}
							reversedFields[name] = true
						default:
							return x.Errorf("Invalid directive")
						}
					}
				}
				return nil
			}
		case lex.ItemError:
			return x.Errorf(item.Val)
		}
	}
	return nil
}

func processObject(it *lex.ItemIterator) error {
	var objName string
	it.Next()
	next := it.Item()
	if next.Typ != itemText {
		return x.Errorf("Missing object name")
	}
	objName = next.Val
	str[objName] = types.UidID

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
			{
				var name, typ string
				name = item.Val

				it.Next()
				next := it.Item()
				if next.Typ != itemColon {
					return x.Errorf("Missing colon")
				}

				it.Next()
				next = it.Item()
				if next.Typ != itemText {
					return x.Errorf("Missing Type")
				}
				typ = next.Val
				t, ok := types.TypeForName(typ)
				if ok {
					if t1, ok := str[name]; ok {
						if t1 != t {
							return x.Errorf("Same field cant have multiple types")
						}
					} else {
						str[name] = t
					}
				}
				// Check for reverse.
				it.Next()
				next = it.Item()
				if next.Typ == itemAt {
					it.Next()
					index := it.Item()
					if index.Typ != itemText {
						return x.Errorf("Missing directive name")
					}
					if index.Val == "reverse" {
						// TODO(jchiu): Add test for this check.
						if t.IsScalar() /* && t != types.UidID */ {
							return x.Errorf("Cannot reverse non-UID scalar")
						}
						reversedFields[name] = true
					} else {
						return x.Errorf("Invalid reverse specification")
					}
				}
				fieldCount++
			}
		case lex.ItemError:
			return x.Errorf(item.Val)
		}
	}
	if fieldCount == 0 {
		return x.Errorf("Object type %v with no fields", objName)
	}
	return nil
}
