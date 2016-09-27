package schema

import (
	"fmt"
	"io/ioutil"

	"github.com/dgraph-io/dgraph/lex"
)

func run(l *lex.Lexer) {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.Items) // No more tokens.
}

// Parse parses the schema file
func Parse(file string) (rerr error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("Error reading file: %v", err)
	}
	str := string(b)

	l := &lex.Lexer{}

	l.Init(str)
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
		}
	}

	for _, v := range store {
		if obj, ok := v.(Object); ok {
			for p, q := range obj.Fields {
				typ := TypeOf(q)
				if typ == nil {
					return fmt.Errorf("Type not defined %v", q)
				}
				if typ != nil && !typ.IsScalar() {
					store[p] = typ
				}
			}
		}
	}

	fmt.Println(store)
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
					return fmt.Errorf("Missing collon")
				}

				if next := <-l.Items; next.Typ != itemScalarType {
					return fmt.Errorf("Missing Type")
				} else {
					typ = next.Val
				}

				if t, ok := getScalar(typ); ok {
					store[name] = t
				} else {
					return fmt.Errorf("Invalid type")
				}
			}
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

				if next := <-l.Items; next.Typ != itemCollon {
					return fmt.Errorf("Missing collon")
				}

				if next := <-l.Items; next.Typ != itemScalarType {
					return fmt.Errorf("Missing Type")
				} else {
					typ = next.Val
				}

				if t, ok := getScalar(typ); ok {
					store[name] = t
				} else {
					return fmt.Errorf("Invalid type")
				}
				return nil
			}
		}
	}
	return nil
}

func processObject(l *lex.Lexer) error {
	var objName string
	if next := <-l.Items; next.Typ != itemObject {
		return fmt.Errorf("Missing object name")
	} else {
		objName = next.Val
	}

	obj := Object{
		Name:   objName,
		Fields: make(map[string]string),
	}

	if next := <-l.Items; next.Typ != itemLeftCurl {
		return fmt.Errorf("Missing left curly brace")
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

				if next := <-l.Items; next.Typ != itemCollon {
					return fmt.Errorf("Missing collon")
				}

				if next := <-l.Items; next.Typ != itemObjectType {
					return fmt.Errorf("Missing Type")
				} else {
					typ = next.Val
				}
				if t, ok := getScalar(typ); ok {
					if t1, ok := store[name]; ok {
						if t1.(Scalar).Name != t.(Scalar).Name {
							return fmt.Errorf("Same field cant have multiple types")
						}
					} else {
						store[name] = t
					}
				}
				obj.Fields[name] = typ
			}
		}
	}
	store[objName] = obj
	return nil
}
