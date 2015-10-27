package gqlex

import (
	"fmt"
	"testing"
)

func TestNewLexer(t *testing.T) {
	input := `
	{
		me {
			name
			city
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
