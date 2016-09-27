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
	s := string(b)

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
		}
	}

	for _, v := range str {
		if obj, ok := v.(Object); ok {
			for p, q := range obj.Fields {
				typ := TypeOf(q)
				if typ == nil {
					return fmt.Errorf("Type not defined %v", q)
				}
				if typ != nil && !typ.IsScalar() {
					str[p] = typ
				}
			}
		}
	}

	fmt.Println(str)
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

				next := <-l.Items
				if next.Typ != itemScalarType {
					return fmt.Errorf("Missing Type")
				}
				typ = next.Val

				t, ok := getScalar(typ)
				if !ok {
					return fmt.Errorf("Invalid type")
				}
				str[name] = t
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

				next := <-l.Items
				if next.Typ != itemCollon {
					return fmt.Errorf("Missing collon")
				}

				next = <-l.Items
				if next.Typ != itemScalarType {
					return fmt.Errorf("Missing Type")
				}
				typ = next.Val

				if t, ok := getScalar(typ); ok {
					str[name] = t
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
	next := <-l.Items
	if next.Typ != itemObject {
		return fmt.Errorf("Missing object name")
	}
	objName = next.Val

	obj := Object{
		Name:   objName,
		Fields: make(map[string]string),
	}

	next = <-l.Items
	if next.Typ != itemLeftCurl {
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

				next := <-l.Items
				if next.Typ != itemCollon {
					return fmt.Errorf("Missing collon")
				}

				next = <-l.Items
				if next.Typ != itemObjectType {
					return fmt.Errorf("Missing Type")
				}
				typ = next.Val
				if t, ok := getScalar(typ); ok {
					if t1, ok := str[name]; ok {
						if t1.(Scalar).Name != t.(Scalar).Name {
							return fmt.Errorf("Same field cant have multiple types")
						}
					} else {
						str[name] = t
					}
				}
				obj.Fields[name] = typ
			}
		}
	}
	str[objName] = obj
	return nil
}
