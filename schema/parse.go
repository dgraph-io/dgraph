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
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

func run(l *lex.Lexer) {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.Items) // No more tokens.
}

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

	l := &lex.Lexer{}
	l.Init(s)
	go run(l)

	for item := range l.Items {
		switch item.Typ {
		case itemScalar:
			{
				if rerr = processScalar(l); rerr != nil {
					return rerr
				}
			}
		case itemType:
			{
				if rerr = processObject(l); rerr != nil {
					return rerr
				}
			}
		case lex.ItemError:
			return x.Errorf(item.Val)
		}
	}

	return nil
}

func processScalarBlock(l *lex.Lexer) error {
	for item := range l.Items {
		switch item.Typ {
		case itemRightRound:
			return nil
		case itemScalarName:
			{
				var name, typ string
				name = item.Val

				if next := <-l.Items; next.Typ != itemCollon {
					return x.Errorf("Missing collon")
				}

				next := <-l.Items
				if next.Typ != itemScalarType {
					return x.Errorf("Missing Type")
				}
				typ = next.Val

				t, ok := types.TypeForName(typ)
				if !ok {
					return x.Errorf("Invalid type")
				}
				str[name] = t

				// Check for index / reverse.
				for {
					next = <-l.Items
					if next.Typ == lex.ItemError {
						return x.Errorf(next.Val)
					}
					if next.Typ == itemDummy {
						break
					}
					if next.Typ == itemAt {
						next = <-l.Items
						if next.Typ == itemIndex {
							indexedFields[name] = true
						} else if next.Typ == itemReverse {
							if t != types.UidID {
								return x.Errorf("Cannot reverse for non-UID type")
							}
							reversedFields[name] = true
						} else {
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

func processScalar(l *lex.Lexer) error {
	for item := range l.Items {
		switch item.Typ {
		case itemLeftRound:
			{
				return processScalarBlock(l)
			}
		case itemScalarName:
			{
				var name, typ string
				name = item.Val

				next := <-l.Items
				if next.Typ != itemCollon {
					return x.Errorf("Missing collon")
				}

				next = <-l.Items
				if next.Typ != itemScalarType {
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
				for {
					next = <-l.Items
					if next.Typ == lex.ItemError {
						return x.Errorf(next.Val)
					}
					if next.Typ == itemDummy {
						break
					}
					if next.Typ == itemAt {
						next = <-l.Items
						if next.Typ == itemIndex {
							indexedFields[name] = true
						} else if next.Typ == itemReverse {
							if t != types.UidID {
								return x.Errorf("Cannot reverse for non-UID type")
							}
							reversedFields[name] = true
						} else {
							return x.Errorf("Invalid index specification")
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

func processObject(l *lex.Lexer) error {
	var objName string
	next := <-l.Items
	if next.Typ != itemObject {
		return x.Errorf("Missing object name")
	}
	objName = next.Val
	str[objName] = types.UidID

	next = <-l.Items
	if next.Typ != itemLeftCurl {
		return x.Errorf("Missing left curly brace")
	}

	fieldCount := 0
L:
	for item := range l.Items {
		switch item.Typ {
		case itemRightCurl:
			break L
		case itemObjectName:
			{
				var name, typ string
				name = item.Val

				next := <-l.Items
				if next.Typ != itemCollon {
					return x.Errorf("Missing collon")
				}

				next = <-l.Items
				if next.Typ != itemObjectType {
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
				next = <-l.Items
				if next.Typ == itemAt {
					index := <-l.Items
					if index.Typ == itemReverse {
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
