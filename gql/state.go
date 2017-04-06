/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

// Package gql is responsible for lexing and parsing a GraphQL query/mutation.
package gql

import "github.com/dgraph-io/dgraph/lex"

const (
	leftCurl     = '{'
	rightCurl    = '}'
	leftRound    = '('
	rightRound   = ')'
	leftSquare   = '['
	rightSquare  = ']'
	period       = '.'
	comma        = ','
	bang         = '!'
	dollar       = '$'
	queryMode    = 1
	mutationMode = 2
	fragmentMode = 3
	schemaMode   = 4
	equal        = '='
	quote        = '"'
	at           = '@'
	colon        = ':'
	lsThan       = '<'
	grThan       = '>'
)

// Constants representing type of different graphql lexed items.
const (
	itemText            lex.ItemType = 5 + iota // plain text
	itemLeftCurl                                // left curly bracket
	itemRightCurl                               // right curly bracket
	itemEqual                                   // equals to symbol
	itemName                                    // [9] names
	itemOpType                                  // operation type
	itemString                                  // quoted string
	itemLeftRound                               // left round bracket
	itemRightRound                              // right round bracket
	itemColon                                   // Colon
	itemAt                                      // @
	itemDollar                                  // $
	itemMutationOp                              // mutation operation
	itemMutationContent                         // mutation content
	itemThreeDots                               // three dots (...)
	itemLeftSquare
	itemRightSquare
	itemComma
	itemMathOp
)

func lexInsideMutation(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				l.Mode = 0 // Set it to default before levaving the mode.
				return lexText
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
			if l.Depth >= 2 {
				return lexTextMutation
			}
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexNameMutation
		case r == '#':
			return lexComment
		case r == lex.EOF:
			return l.Errorf("Unclosed mutation action")
		default:
			return l.Errorf("Unrecognized character inside mutation: %#U", r)
		}
	}
}

func lexInsideSchema(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == rightRound:
			l.Emit(itemRightRound)
		case r == leftRound:
			l.Emit(itemLeftRound)
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				l.Mode = 0
				return lexText
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
		case r == leftSquare:
			l.Emit(itemLeftSquare)
		case r == rightSquare:
			l.Emit(itemRightSquare)
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexArgName
		case r == '#':
			return lexComment
		case r == colon:
			l.Emit(itemColon)
		case r == comma:
			l.Emit(itemComma)
		case r == lex.EOF:
			return l.Errorf("Unclosed schema action")
		default:
			return l.Errorf("Unrecognized character inside schema: %#U", r)
		}
	}
}

func lexFuncOrArg(l *lex.Lexer) lex.StateFn {
	var empty bool
	for {
		switch r := l.Next(); {
		case r == at:
			l.Emit(itemAt)
			return lexDirective
		case isNameBegin(r) || isNumber(r):
			empty = false
			return lexArgName
		case isMathOp(r):
			l.Emit(itemMathOp)
		case isInequalityOp(r):
			if r == equal {
				if !isInequalityOp(l.Peek()) {
					l.Emit(itemEqual)
					continue
				}
			} else if r == lsThan {
				if !isSpace(l.Peek()) && l.Peek() != '=' {
					// as long as its not '=' or ' '
					return lexIRIRef
				}
			}
			if isInequalityOp(l.Peek()) {
				l.Next()
			}
			l.Emit(itemMathOp)
		case r == leftRound:
			l.Emit(itemLeftRound)
			l.ArgDepth++
		case r == rightRound:
			if l.ArgDepth == 0 {
				return l.Errorf("Unexpected right round bracket")
			}
			l.ArgDepth--
			l.Emit(itemRightRound)
			if empty {
				return l.Errorf("Empty Argument")
			}
			if l.ArgDepth == 0 {
				l.InsideDirective = false
				return lexText // Filter directive is done.
			}
		case r == lex.EOF:
			return l.Errorf("Unclosed Brackets")
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case r == comma:
			if empty {
				return l.Errorf("Consecutive commas not allowed.")
			}
			empty = true
			l.Emit(itemComma)
		case isDollar(r):
			l.Emit(itemDollar)
		case r == colon:
			l.Emit(itemColon)
		case isEndLiteral(r):
			{
				empty = false
				l.AcceptUntil(isEndLiteral) // This call will backup the ending ".
				l.Next()                    // Consume the " .
				l.Emit(itemName)
			}
		case r == '[':
			{
				depth := 1
				for {
					r := l.Next()
					if r == lex.EOF || r == ')' {
						return l.Errorf("Invalid bracket sequence")
					} else if r == '[' {
						depth++
					} else if r == ']' {
						depth--
					}
					if depth > 2 || depth < 0 {
						return l.Errorf("Invalid bracket sequence")
					} else if depth == 0 {
						break
					}
				}
				l.Emit(itemName)
				empty = false
				l.AcceptRun(isSpace)
				l.Ignore()
				if !isEndArg(l.Peek()) {
					return l.Errorf("Invalid bracket sequence")
				}
			}
		case r == '#':
			return lexComment
		default:
			return l.Errorf("Unrecognized character in inside a func: %#U", r)
		}
	}
}

func lexTopLevel(l *lex.Lexer) lex.StateFn {
Loop:
	for {
		switch r := l.Next(); {
		case r == leftCurl:
			l.Depth++ // one level down.
			l.Emit(itemLeftCurl)
			return lexText
		case r == rightCurl:
			return l.Errorf("Too many right curl")
		case r == lex.EOF:
			break Loop
		case r == '#':
			return lexComment
		case r == leftRound:
			l.Backup()
			l.Emit(itemText)
			l.Next()
			l.Emit(itemLeftRound)
			l.ArgDepth++
			return lexText
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

// lexText lexes the input string and calls other lex functions.
func lexText(l *lex.Lexer) lex.StateFn {
	for {
		if l.Mode == mutationMode {
			return lexInsideMutation
		} else if l.Mode == schemaMode {
			return lexInsideSchema
		} else if l.ArgDepth > 0 || l.InsideDirective {
			return lexFuncOrArg
		} else if l.Depth == 0 && l.Mode != fragmentMode {
			return lexTopLevel
		}

		switch r := l.Next(); {
		case r == period:
			if l.Next() == period && l.Next() == period {
				l.Emit(itemThreeDots)
				return lexName
			}
			// We do not expect a period at all. If you do, you may want to
			// backup the two extra periods we try to read.
			return l.Errorf("Unrecognized character in lexText: %#U", r)
		case r == rightCurl:
			l.Depth--
			l.Emit(itemRightCurl)
			if l.Depth == 0 && l.Mode == fragmentMode {
				l.Mode = 0
			}
		case r == leftCurl:
			l.Depth++
			l.Emit(itemLeftCurl)
		case r == lex.EOF:
			return l.Errorf("Unclosed action")
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case r == comma:
			l.Emit(itemComma)
		case isNameBegin(r):
			return lexName
		case r == '#':
			return lexComment
		case r == leftRound:
			l.Emit(itemLeftRound)
			l.AcceptRun(isSpace)
			l.Ignore()
			l.ArgDepth++
			return lexText
		case r == colon:
			l.Emit(itemColon)
		case r == at:
			l.Emit(itemAt)
			return lexDirective
		case r == lsThan:
			return lexIRIRef
		default:
			return l.Errorf("Unrecognized character in lexText: %#U", r)
		}
	}
}

func lexIRIRef(l *lex.Lexer) lex.StateFn {
	if err := lex.LexIRIRef(l, itemName); err != nil {
		return l.Errorf(err.Error())
	}
	return lexText
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
		l.Emit(itemName)
		break
	}
	return lexText
}

// lexDirective is called right after we see a @.
func lexDirective(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if !isNameBegin(r) {
		return l.Errorf("Unrecognized character in lexDirective: %#U", r)
	}

	l.Backup()
	return lexText
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
	return lexText
}

// lexComment lexes a comment text.
func lexComment(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isEndOfLine(r) {
			l.Ignore()
			return lexText
		}
		if r == lex.EOF {
			break
		}
	}
	l.Ignore()
	l.Emit(lex.ItemEOF)
	return nil // Stop the run loop.
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
	return lexText
}

// lexTextMutation lexes and absorbs the text inside a mutation operation block.
func lexTextMutation(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if r == lex.EOF {
			return l.Errorf("Unclosed mutation text")
		}
		if r == quote {
			return lexMutationValue
		}
		if r == leftCurl {
			return l.Errorf("Invalid character '{' inside mutation text")
		}
		if r != rightCurl {
			// Absorb everything until we find '}'.
			continue
		}
		l.Backup()
		l.Emit(itemMutationContent)
		break
	}
	return lexText
}

// This function is used to absorb the object value.
func lexMutationValue(l *lex.Lexer) lex.StateFn {
LOOP:
	for {
		r := l.Next()
		switch r {
		case lex.EOF:
			return l.Errorf("Unclosed mutation value")
		case quote:
			break LOOP
		case '\\':
			l.Next() // skip one.
		}
	}
	return lexTextMutation
}

// lexOperationType lexes a query or mutation or schema operation type.
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
		} else if word == "schema" {
			l.Emit(itemOpType)
			l.Mode = schemaMode
		} else {
			if l.Mode == 0 {
				l.Errorf("Invalid operation type")
			}
		}
		break
	}
	return lexText
}

// lexArgName lexes and emits the name part of an argument.
func lexArgName(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemName)
		break
	}
	return lexText
}

// isDollar returns true if the rune is a Dollar($).
func isDollar(r rune) bool {
	return r == '$' || r == '\u0024'
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

// isEndArg returns true if rune is a comma or right round bracket.
func isEndArg(r rune) bool {
	return r == comma || r == ')'
}

// isNameBegin returns true if the rune is an alphabet or an '_' or '~'.
func isNameBegin(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r == '_':
		return true
	case r == '~':
		return true
	default:
		return false
	}
}

func isMathOp(r rune) bool {
	switch r {
	case '+', '-', '*', '/':
		return true
	default:
		return false
	}
}

func isInequalityOp(r rune) bool {
	switch {
	case r == '<' || r == '>' || r == '=':
		return true
	default:
		return false
	}
}

func isNumber(r rune) bool {
	switch {
	case (r >= '0' && r <= '9'):
		return true
	default:
		return false
	}
}

func isNameSuffix(r rune) bool {
	if isMathOp(r) {
		return false
	}
	if isNameBegin(r) || isNumber(r) {
		return true
	}
	if r == '.' /*|| r == '-'*/ || r == '!' { // Use by freebase.
		return true
	}
	return false
}
