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

	for _, v := range str {
		if obj, ok := v.(types.Object); ok {
			for p, q := range obj.Fields {
				typ := TypeOf(q)
				if typ == nil {
					return x.Errorf("Type not defined %v", q)
				}
				if typ != nil && !typ.IsScalar() {
					str[p] = typ
				}
			}
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

				t, ok := getScalar(typ)
				if !ok {
					return x.Errorf("Invalid type")
				}
				str[name] = t

				// Check for index.
				next = <-l.Items
				if next.Typ == itemAt {
					index := <-l.Items
					if index.Typ == itemIndex {
						indexedFields[name] = true
					} else {
						return x.Errorf("Invalid index specification")
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

				if t, ok := getScalar(typ); ok {
					str[name] = t
				} else {
					return x.Errorf("Invalid type")
				}

				// Check for index.
				next = <-l.Items
				if next.Typ == itemAt {
					index := <-l.Items
					if index.Typ == itemIndex {
						indexedFields[name] = true
					} else {
						return x.Errorf("Invalid index specification")
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

	obj := types.Object{
		Name:   objName,
		Fields: make(map[string]string),
	}

	next = <-l.Items
	if next.Typ != itemLeftCurl {
		return x.Errorf("Missing left curly brace")
	}

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
				if t, ok := getScalar(typ); ok {
					if t1, ok := str[name]; ok {
						if t1.(types.TypeID) != t.(types.TypeID) {
							return x.Errorf("Same field cant have multiple types")
						}
					} else {
						str[name] = t
					}
				}
				if _, ok := obj.Fields[name]; ok {
					return x.Errorf("Repeated field %v in object %v", name, objName)
				}
				obj.Fields[name] = typ

				// Check for reverse.
				next = <-l.Items
				if next.Typ == itemAt {
					index := <-l.Items
					if index.Typ == itemReverse {
						reversedFields[name] = true
					} else {
						return x.Errorf("Invalid reverse specification")
					}
				}
			}
		case lex.ItemError:
			return x.Errorf(item.Val)
		}
	}
	if len(obj.Fields) == 0 {
		return x.Errorf("Object type %v with no fields", objName)
	}
	str[objName] = obj
	return nil
}
