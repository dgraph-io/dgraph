package gqlex

import (
	"fmt"
	"testing"
)

func TestNewLexer(t *testing.T) {
	input := `
	mutation {
		me {
			name0 # my name
			_city, # 0what would fail lex.
			friends {
				name
			}
		}
	}`
	l := newLexer(input)
	for item := range l.items {
		fmt.Println(item.String())
	}
}
