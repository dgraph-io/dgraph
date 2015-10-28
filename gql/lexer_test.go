package gql

import (
	"fmt"
	"testing"
)

func TestNewLexer(t *testing.T) {
	input := `
	mutation {
		me( id: 10, xid: rick ) {
			name0 # my name
			_city, # 0what would fail lex.
			profilePic(width: 100, height: 100)
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
