/*
 * Copyright 2017 DGraph Labs, Inc.
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
	quote        = '"'
	attherate    = '@'
)

// Constants representing type of different graphql lexed items.
const (
	itemText            lex.ItemType = 5 + iota // plain text
	itemLeftCurl                                // left curly bracket
	itemRightCurl                               // right curly bracket
	itemEqual                                   // equals to symbol
	itemComment                                 // comment
	itemName                                    // [9] names
	itemOpType                                  // operation type
	itemString                                  // quoted string
	itemLeftRound                               // left round bracket
	itemRightRound                              // right round bracket
	itemCollon                                  // Collon
	itemAt                                      // @
	itemDollar                                  // $
	itemMutationOp                              // mutation operation
	itemMutationContent                         // mutation content
	itemThreeDots                               // three dots (...)
	itemAnd                                     // And inside a filter.
	itemOr                                      // Or inside a filter.

)

// lexText lexes the input string and calls other lex functions.
func lexText(l *lex.Lexer) lex.StateFn {
	var empty bool
Loop:
	for {
		if l.Mode == mutationMode {
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
			case r == lex.EOF:
				return l.Errorf("Unclosed mutation action")
			case isSpace(r) || isEndOfLine(r):
				l.Ignore()
			case isNameBegin(r):
				return lexNameMutation
			default:
				return l.Errorf("Unrecognized character inside mutation: %#U", r)
			}
		} else if l.FilterDepth > 0 || l.InsideDirective {
			switch r := l.Next(); {
			case r == leftRound:
				l.Emit(itemLeftRound)
				l.FilterDepth++
			case r == rightRound:
				if l.FilterDepth == 0 {
					return l.Errorf("Unexpected right round bracket")
				}
				l.FilterDepth--
				l.Emit(itemRightRound)
				if empty {
					return l.Errorf("Empty Argument")
				}
				if l.FilterDepth == 0 {
					l.InsideDirective = false
					return lexText // Filter directive is done.
				}
			case r == lex.EOF:
				return l.Errorf("Unclosed Brackets")
			case isSpace(r) || isEndOfLine(r):
				l.Ignore()
			case r == comma:
				empty = true
				l.Ignore()
			case r == '&':
				r2 := l.Next()
				if r2 == '&' {
					l.Emit(itemAnd)
					return lexText
				}
				return l.Errorf("Expected & but got %v", r2)
			case r == '|':
				r2 := l.Next()
				if r2 == '|' {
					l.Emit(itemOr)
					return lexText
				}
				return l.Errorf("Expected | but got %v", r2)
			case isDollar(r):
				l.Emit(itemDollar)
			case isNameBegin(r) || isNumber(r):
				empty = false
				return lexArgName
			case r == ':':
				l.Emit(itemCollon)
			case r == equal:
				l.Emit(itemEqual)
			case isEndLiteral(r):
				{
					empty = false
					l.Ignore()
					l.AcceptUntil(isEndLiteral) // This call will backup the ending ".
					l.Emit(itemName)
					l.Next() // Consume the " and ignore it.
					l.Ignore()
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
			default:
				return l.Errorf("Unrecognized character in lexText: %#U", r)
			}
		} else if l.Depth == 0 {
			switch r := l.Next(); {
			case r == leftCurl:
				l.Backup()
				l.Emit(itemText) // emit whatever we have so far.
				l.Next()         // advance one to get back to where we saw leftCurl.
				l.Depth++        // one level down.
				l.Emit(itemLeftCurl)
				if l.Mode == mutationMode {
					return lexText
				}
				// Both queryMode and fragmentMode are handled by lexInside.
				return lexText
			case r == rightCurl:
				return l.Errorf("Too many right characters")
			case r == lex.EOF:
				break Loop
			case r == leftRound:
				l.Backup()
				l.Emit(itemText)
				l.Next()
				l.Emit(itemLeftRound)
				l.FilterDepth++
				return lexText
			case isNameBegin(r):
				l.Backup()
				l.Emit(itemText)
				return lexOperationType
			}
		} else {
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
				l.AcceptRun(isSpace)
				l.Ignore()
				l.FilterDepth++
				return lexText
			case r == ':':
				l.Emit(itemCollon)
			case r == attherate:
				l.Emit(itemAt)
				return lexDirective
			default:
				return l.Errorf("Unrecognized character in lexText: %#U", r)
			}

		}
	}
	if l.Pos > l.Start {
		l.Emit(itemText)
	}
	l.Emit(lex.ItemEOF)
	return nil
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
		l.Emit(itemName)

		directive := buf.Bytes()[:buf.Len()-1]
		// The lexer may behave differently for different directives. Hence, we need
		// to check the directive here and go into the right state.
		if string(directive) == "filter" {
			l.InsideDirective = true
			return lexText
		}
		return l.Errorf("Unhandled directive %s", directive)
	}
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
			l.Emit(itemComment)
			return lexText
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
			l.Depth++
		}
		if r == rightCurl {
			if l.Depth > 2 {
				l.Depth--
				continue
			}
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
	for {
		r := l.Next()
		if r == '\\' {
			// So that we don't count \" as end of value.
			if l.Next() == quote {
				continue
			}

		}
		// This is an end of value so lets return.
		if r == quote {
			break
		}
		// We absorb everything else.
		continue
	}
	return lexTextMutation
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
func isNumber(r rune) bool {
	switch {
	case (r >= '0' && r <= '9') || r == '-' || r == '+':
		return true
	default:
		return false
	}
}
func isNameSuffix(r rune) bool {
	if isNameBegin(r) {
		return true
	}
	if isNumber(r) {
		return true
	}
	if r == '.' || r == '-' || r == '!' { // Use by freebase.
		return true
	}
	return false
}
