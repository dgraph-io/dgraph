/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package gql is responsible for lexing and parsing a GraphQL query/mutation.
package gql

import (
	"bytes"

	"github.com/dgraph-io/dgraph/lex"
)

const (
	leftCurl     = '{'
	rightCurl    = '}'
	leftRound    = '('
	rightRound   = ')'
	period       = '.'
	comma        = ','
	bang         = '!'
	dollar       = '$'
	queryMode    = 1
	mutationMode = 2
	fragmentMode = 3
	equal        = '='
)

// Constants representing type of different graphql lexed items.
const (
	itemText            lex.ItemType = 5 + iota // plain text
	itemLeftCurl                                // left curly bracket
	itemRightCurl                               // right curly bracket
	itemComma                                   // a comma
	itemEqual                                   // equals to symbol
	itemComment                                 // comment
	itemName                                    // [9] names
	itemOpType                                  // operation type
	itemString                                  // quoted string
	itemLeftRound                               // left round bracket
	itemRightRound                              // right round bracket
	itemArgName                                 // argument name
	itemArgVal                                  // argument val
	itemMutationOp                              // mutation operation
	itemMutationContent                         // mutation content
	itemFragmentSpread                          // three dots and name
	itemVarName                                 // dollar followed by a name
	itemVarType                                 // type a variable
	itemVarDefault                              // default value of a variable
	itemAlias                                   // Alias for a field
	itemDirectiveName                           // Name starting with @
	itemFilterAnd                               // And inside a filter.
	itemFilterOr                                // Or inside a filter.
	itemFilterFunc                              // Function inside a filter.
	itemFilterFuncArg                           // Function args inside a filter.
)

// lexText lexes the input string and calls other lex functions.
func lexText(l *lex.Lexer) lex.StateFn {
Loop:
	for {
		switch r := l.Next(); {
		case r == leftCurl:
			l.Backup()
			l.Emit(itemText) // emit whatever we have so far.
			l.Next()         // advance one to get back to where we saw leftCurl.
			l.Depth++        // one level down.
			l.Emit(itemLeftCurl)
			if l.Mode == mutationMode {
				return lexInsideMutation
			}
			// Both queryMode and fragmentMode are handled by lexInside.
			return lexInside
		case r == rightCurl:
			return l.Errorf("Too many right characters")
		case r == lex.EOF:
			break Loop
		case r == leftRound:
			l.Backup()
			l.Emit(itemText)
			l.Next()
			l.Emit(itemLeftRound)
			return lexVarInside
		case isNameBegin(r):
			l.Backup()
			l.Emit(itemText)
			return lexOperationType
		}
	}
	if l.Pos > l.Start {
		l.Emit(itemText)
	}
	l.Emit(lex.ItemEOF)
	return nil
}

// lexInside lexes the content inside a query block.
func lexInside(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == period:
			if l.Next() == period && l.Next() == period {
				return lexFragmentSpread
			}
			// We do not expect a period at all. If you do, you may want to
			// backup the two extra periods we try to read.
			return l.Errorf("Unrecognized character in lexInside: %#U", r)
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				return lexText
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
		case r == lex.EOF:
			return l.Errorf("Unclosed action")
		case isSpace(r) || isEndOfLine(r) || r == comma:
			l.Ignore()
		case isNameBegin(r):
			return lexName
		case r == '#':
			l.Backup()
			return lexComment
		case r == leftRound:
			l.Emit(itemLeftRound)
			return lexArgInside
		case r == ':':
			return lexAlias
		case r == '@':
			return lexDirective
		default:
			return l.Errorf("Unrecognized character in lexInside: %#U", r)
		}
	}
}

// lexFilterFuncInside expects input to look like ("...", "...").
func lexFilterFuncInside(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isSpace)
	l.Ignore() // Any spaces encountered.

	for {
		r := l.Next()
		if isSpace(r) || r == ',' {
			l.Ignore()
		} else if r == leftRound {
			l.Emit(itemLeftRound)
		} else if r == rightRound {
			l.Emit(itemRightRound)
			return lexFilterInside
		} else if isEndLiteral(r) {
			l.Ignore()
			l.AcceptUntil(isEndLiteral) // This call will backup the ending ".
			l.Emit(itemFilterFuncArg)
			l.Next() // Consume " and ignore it.
			l.Ignore()
		} else {
			return l.Errorf("Expected quotation mark in lexFilterFuncArgs")
		}
	}
}

// lexFilterFuncName expects input to look like equal("...", "...").
func lexFilterFuncName(l *lex.Lexer) lex.StateFn {
	for {
		// The caller already checked isNameBegin, and absorbed one rune.
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemFilterFunc)
		break
	}
	return lexFilterFuncInside
}

// lexFilterInside expects input like (  (equal(...) && f(...)) || g(...)  ).
func lexFilterInside(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		switch {
		case r == leftRound:
			l.Emit(itemLeftRound)
			l.FilterDepth++
		case r == rightRound:
			if l.FilterDepth == 0 {
				return l.Errorf("Unexpected right round bracket")
			}
			l.FilterDepth--
			l.Emit(itemRightRound)
			if l.FilterDepth == 0 {
				return lexInside // Filter directive is done.
			}
		case r == lex.EOF:
			return l.Errorf("Unclosed directive")
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexFilterFuncName
		case r == '&':
			r2 := l.Next()
			if r2 == '&' {
				l.Emit(itemFilterAnd)
				return lexFilterInside
			}
			return l.Errorf("Expected & but got %v", r2)
		case r == '|':
			r2 := l.Next()
			if r2 == '|' {
				l.Emit(itemFilterOr)
				return lexFilterInside
			}
			return l.Errorf("Expected | but got %v", r2)
		default:
			return l.Errorf("Unrecognized character in lexInside: %#U", r)
		}
	}
}

// lexDirective is called right after we see a @.
func lexDirective(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if !isNameBegin(r) {
		return l.Errorf("Unrecognized character in lexDirective: %#U", r)
	}

	l.Backup()
	// This gives our buffer an initial capacity. Its length is zero though. The
	// buffer can grow beyond this initial capacity.
	buf := bytes.NewBuffer(make([]byte, 0, 15))
	for {
		// The caller already checked isNameBegin, and absorbed one rune.
		r = l.Next()
		buf.WriteRune(r)
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemDirectiveName)

		directive := buf.Bytes()[:buf.Len()-1]
		// The lexer may behave differently for different directives. Hence, we need
		// to check the directive here and go into the right state.
		if string(directive) == "filter" {
			return lexFilterInside
		}
		return l.Errorf("Unhandled directive %s", directive)
	}
	return lexInside
}

func lexAlias(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isSpace)
	l.Ignore() // Any spaces encountered.
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemAlias)
		break
	}
	return lexInside
}

func lexFragmentSpread(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemFragmentSpread)
		break
	}
	return lexInside
}

func lexName(l *lex.Lexer) lex.StateFn {
	for {
		// The caller already checked isNameBegin, and absorbed one rune.
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemName)
		break
	}
	return lexInside
}

// lexComment lexes a comment text.
func lexComment(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isEndOfLine(r) {
			l.Emit(itemComment)
			return lexInside
		}
		if r == lex.EOF {
			break
		}
	}
	if l.Pos > l.Start {
		l.Emit(itemComment)
	}
	l.Emit(lex.ItemEOF)
	return nil // Stop the run loop.
}

// lexInsideMutation lexes the text inside a mutation block.
func lexInsideMutation(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				return lexText
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
			if l.Depth >= 2 {
				return lexTextMutation
			}
		case r == lex.EOF:
			return l.Errorf("Unclosed mutation action")
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexNameMutation
		default:
			return l.Errorf("Unrecognized character in lexInsideMutation: %#U", r)
		}
	}
}

// lexNameMutation lexes the itemMutationOp, which could be set or delete.
func lexNameMutation(l *lex.Lexer) lex.StateFn {
	for {
		// The caller already checked isNameBegin, and absorbed one rune.
		r := l.Next()
		if isNameBegin(r) {
			continue
		}
		l.Backup()
		l.Emit(itemMutationOp)
		break
	}
	return lexInsideMutation
}

// lexTextMutation lexes and absorbs the text inside a mutation operation block.
func lexTextMutation(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if r == lex.EOF {
			return l.Errorf("Unclosed mutation text")
		}
		if r != rightCurl {
			// Absorb everything until we find '}'.
			continue
		}
		l.Backup()
		l.Emit(itemMutationContent)
		break
	}
	return lexInsideMutation
}

// lexOperationType lexes a query or mutation operation type.
func lexOperationType(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		// l.Pos would be index of the end of operation type + 1.
		word := l.Input[l.Start:l.Pos]
		if word == "mutation" {
			l.Emit(itemOpType)
			l.Mode = mutationMode
		} else if word == "fragment" {
			l.Emit(itemOpType)
			l.Mode = fragmentMode
		} else if word == "query" {
			l.Emit(itemOpType)
			l.Mode = queryMode
		}
		break
	}
	return lexText
}

func lexVarInside(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == lex.EOF:
			return l.Errorf("unclosed argument")
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case isDollar(r):
			return lexVarName
		case r == ':':
			l.Ignore()
			return lexVarType
		case r == equal:
			l.Emit(itemEqual)
			l.Ignore()
			return lexVarDefault
		case r == rightRound:
			l.Emit(itemRightRound)
			return lexText
		case r == comma:
			l.Emit(itemComma)
			l.Ignore()
		default:
			return l.Errorf("variable list invalid")
		}
	}
}

// lexVarName lexes and emits the name of a variable.
func lexVarName(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemVarName)
		break
	}
	return lexVarInside
}

// lexVarType lexes and emits the type of a variable.
func lexVarType(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isSpace)
	l.Ignore() // Any spaces encountered.
	for {
		r := l.Next()
		if isSpace(r) || isEndOfLine(r) || r == rightRound || r == comma {
			l.Backup()
			l.Emit(itemVarType)
			return lexVarInside
		}
		if r == lex.EOF {
			return l.Errorf("Reached lex.EOF while reading var value: %v",
				l.Input[l.Start:l.Pos])
		}
	}
}

// lexVarDefault lexes and emits the Default value of a variable.
func lexVarDefault(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isSpace)
	l.Ignore() // Any spaces encountered.
	for {
		r := l.Next()
		if isSpace(r) || isEndOfLine(r) || r == rightRound || r == comma {
			l.Backup()
			l.Emit(itemVarDefault)
			return lexVarInside
		}
		if r == lex.EOF {
			return l.Errorf("Reached lex.EOF while reading default value: %v",
				l.Input[l.Start:l.Pos])
		}
	}
}

// lexArgInside is used to lex the arguments inside ().
func lexArgInside(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == lex.EOF:
			return l.Errorf("unclosed argument")
		case isSpace(r) || isEndOfLine(r) || r == comma:
			l.Ignore()
		case isNameBegin(r):
			return lexArgName
		case r == ':':
			l.Ignore()
			return lexArgVal
		case r == rightRound:
			l.Emit(itemRightRound)
			return lexInside
		default:
			return l.Errorf("argument list invalid")
		}
	}
}

// lexArgName lexes and emits the name part of an argument.
func lexArgName(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemArgName)
		break
	}
	return lexArgInside
}

// lexArgVal lexes and emits the value part of an argument.
func lexArgVal(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isSpace)
	l.Ignore() // Any spaces encountered.
	for {
		r := l.Next()
		if isSpace(r) || isEndOfLine(r) || r == rightRound || r == comma {
			l.Backup()
			l.Emit(itemArgVal)
			return lexArgInside
		}
		if r == lex.EOF {
			return l.Errorf("Reached lex.EOF while reading var value: %v",
				l.Input[l.Start:l.Pos])
		}
	}
}

// isDollar returns true if the rune is a Dollar($).
func isDollar(r rune) bool {
	return r == '\u0024'
}

// isSpace returns true if the rune is a tab or space.
func isSpace(r rune) bool {
	return r == '\u0009' || r == '\u0020'
}

// isEndOfLine returns true if the rune is a Linefeed or a Carriage return.
func isEndOfLine(r rune) bool {
	return r == '\u000A' || r == '\u000D'
}

// isEndLiteral returns true if rune is quotation mark.
func isEndLiteral(r rune) bool {
	return r == '"' || r == '\u000d' || r == '\u000a'
}

// isNameBegin returns true if the rune is an alphabet or an '_'.
func isNameBegin(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r == '_':
		return true
	default:
		return false
	}
}

func isNameSuffix(r rune) bool {
	if isNameBegin(r) {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	if r == '.' || r == '-' { // Use by freebase.
		return true
	}
	return false
}
