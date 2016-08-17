package parser

import (
	"fmt"
	"strings"
)

// Have to do this as an interface to retain expected err != nil behaviour,
// because syntaxError(nil) is a valid error.
type SyntaxError interface {
	error
	AddContext(SyntaxErrorContext)
}

// Hides inside the SyntaxError for reasons give on that interface.
type syntaxError struct {
	Err   error
	Stack []SyntaxErrorContext
}

// Add a calling frame context.
func (se *syntaxError) AddContext(c SyntaxErrorContext) {
	se.Stack = append(se.Stack, c)
}

// Start a new syntax error. It's very unlikely that you wouldn't specify an
// Err in the first context.
func NewSyntaxError(c SyntaxErrorContext) SyntaxError {
	se := new(syntaxError)
	se.Stack = append(se.Stack, c)
	return se
}

// Represents frame in the recursive descent.
type SyntaxErrorContext struct {
	Stream Stream
	Parser Parser
	Err    error
}

// Can't think of a better way to do this formatting.
func (se *syntaxError) Error() string {
	var ss []string
	for _, c := range se.Stack {
		if c.Err != nil {
			ss = append(ss, c.Err.Error())
		}
		if c.Parser != nil {
			ss = append(ss, fmt.Sprintf("while parsing %s", ParserName(c.Parser)))
		}
		if c.Stream != nil {
			ss = append(ss, fmt.Sprintf("starting with %q at %s", c.Stream.Token(), c.Stream.Position()))
		}
	}
	s := strings.Join(ss, " ")
	ret := "syntax error"
	if s != "" {
		ret += ": " + s
	}
	return ret
}
