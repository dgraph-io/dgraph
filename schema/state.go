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
	"errors"
	"fmt"

	"github.com/dgraph-io/dgraph/lex"
)

const (
	leftCurl   = '{'
	rightCurl  = '}'
	leftRound  = '('
	rightRound = ')'
	collon     = ':'
	lsThan     = '<'
	grThan     = '>'
)

// Constants representing type of different graphql lexed items.
const (
	itemText       lex.ItemType = 5 + iota // plain text
	itemScalar                             // scalar
	itemType                               // type
	itemLeftCurl                           // left curly bracket
	itemRightCurl                          // right curly bracket
	itemComment                            // comment
	itemLeftRound                          // left round bracket
	itemRightRound                         // right round bracket
	itemScalarName
	itemScalarType
	itemObject
	itemObjectName
	itemObjectType
	itemCollon
	itemAt
	itemIndex
	itemReverse
	itemDummy // Used if index specification is missing
)

// lexText lexes the input string and calls other lex functions.
func lexText(l *lex.Lexer) lex.StateFn {
Loop:
	for {
		switch r := l.Next(); {
		case r == lex.EOF:
			break Loop
		case isNameBegin(r):
			l.Backup()
			return lexStart
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
	if l.Pos > l.Start {
		l.Emit(itemText)
	}
	l.Emit(lex.ItemEOF)
	return nil
}

func lexStart(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		// l.Pos would be index of the end of operation type + 1.
		word := l.Input[l.Start:l.Pos]
		if word == "scalar" {
			l.Emit(itemScalar)
			return lexScalar
		} else if word == "type" {
			l.Emit(itemType)
			return lexObject
		} else {
			return l.Errorf("Invalid schema")
		}
	}

}

func lexScalar(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == lsThan || isNameBegin(r):
			l.Backup()
			return lexScalarPair
		case r == leftRound:
			l.Emit(itemLeftRound)
			l.Next()
			return lexScalarBlock
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
}

func lexScalarBlock(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == ')':
			l.Emit(itemRightRound)
			return lexText
		case r == lsThan || isNameBegin(r):
			l.Backup()
			return lexScalarPair1
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
}

func lexObject(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == rightCurl:
			return lexText
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case r == lsThan || isNameBegin(r):
			l.Backup()
			if err := lexName(l, itemObject); err != nil {
				return l.Errorf("Invalid schema. Error: ", err.Error())
			}
			return lexObjectBlock
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
}

func lexObjectBlock(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == leftCurl:
			l.Emit(itemLeftCurl)
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case r == rightCurl:
			l.Emit(itemRightCurl)
			return lexText
		case r == '<' || isNameBegin(r):
			l.Backup()
			return lexObjectPair
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
}

func lexScalarPair(l *lex.Lexer) lex.StateFn {
	if err := lexName(l, itemScalarName); err != nil {
		return l.Errorf("Invalid schema. Error : %s", err.Error())
	}

L:
	for {
		switch r := l.Next(); {
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case r == ':':
			l.Emit(itemCollon)
			break
		case isNameBegin(r):
			l.Backup()
			break L
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}

	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		l.Emit(itemScalarType)
		break
	}

	// Keep reading for terms that start with @. Keep outputting. If we see
	// something that doesn't start with @, we output ItemDummy. It is like
	// null-termination of C strings.

L1:
	for {
		switch r := l.Next(); {
		case isSpace(r):
			l.Ignore()
		case r == lex.EOF || isEndOfLine(r):
			break L1
		case r == '@':
			l.Emit(itemAt)
			if errState := processDirective(l); errState != nil {
				return errState
			}
			break L1
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
	l.Emit(itemDummy)
	return lexText
}

func lexName(l *lex.Lexer, styp lex.ItemType) error {
	r := l.Next()
	if r == lsThan {
		return lexIRIRef(l, styp)
	}

	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		// l.Pos would be index of the end of operation type + 1.
		l.Emit(styp)
		break
	}
	return nil
}

// assumes '<' is already lexed.
func lexIRIRef(l *lex.Lexer, styp lex.ItemType) error {
	l.Ignore() // ignore '<'
	l.AcceptRunRec(isIRIChar)
	l.Emit(styp) // will emit without '<' and '>'
	r := l.Next()
	if r != grThan {
		return errors.New(fmt.Sprintf("Unexpected character %v. Expected '>'.", r))
	}
	l.Ignore() // ignore '>'
	return nil
}

func lexScalarPair1(l *lex.Lexer) lex.StateFn {
	if err := lexName(l, itemScalarName); err != nil {
		return l.Errorf("Invalid schema. Error : %s", err.Error())
	}

L:
	for {
		switch r := l.Next(); {
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case r == ':':
			l.Emit(itemCollon)
			break
		case isNameBegin(r):
			l.Backup()
			break L
		default:
			return l.Errorf("Invalid schema. Unexpected %s and %v",
				l.Input[l.Start:l.Pos], r)
		}
	}

	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		l.Emit(itemScalarType)
		break
	}

L1:
	for {
		switch r := l.Next(); {
		case isSpace(r):
			l.Ignore()
		case isEndOfLine(r):
			break L1
		case r == '@':
			l.Emit(itemAt)
			if errState := processDirective(l); errState != nil {
				return errState
			}
			break L1
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
	l.Emit(itemDummy)
	return lexScalarBlock
}

func lexObjectPair(l *lex.Lexer) lex.StateFn {
	if err := lexName(l, itemObjectName); err != nil {
		return l.Errorf("Invalid schema. Error: ", err.Error())
	}

L:
	for {
		switch r := l.Next(); {
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case r == ':':
			l.Emit(itemCollon)
			break
		case isNameBegin(r):
			l.Backup()
			break L
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}

	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		l.Emit(itemObjectType)
		break
	}

	// Check for mention of @reverse.
	var isReversed bool
L1:
	for {
		switch r := l.Next(); {
		case isSpace(r):
			l.Ignore()
		case isEndOfLine(r):
			break L1
		case r == '@':
			l.Emit(itemAt)
			isReversed = true
			for {
				r := l.Next()
				if isNameSuffix(r) {
					continue // absorb
				}
				l.Backup()
				// l.Pos would be index of the end of operation type + 1.
				word := l.Input[l.Start:l.Pos]
				if word == "reverse" {
					l.Emit(itemReverse)
					break L1
				} else {
					return l.Errorf("Invalid mention of reverse")
				}
			}
			break L1
		default:
			return l.Errorf("Invalid schema. Unexpected %s", l.Input[l.Start:l.Pos])
		}
	}
	if !isReversed {
		l.Emit(itemDummy)
	}
	return lexObjectBlock
}

// processDirective returns nil if we are ok. Otherwise, it returns error state.
func processDirective(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		// l.Pos would be index of the end of operation type + 1.
		word := l.Input[l.Start:l.Pos]
		if word == "index" {
			l.Emit(itemIndex)
		} else if word == "reverse" {
			l.Emit(itemReverse)
		} else {
			return l.Errorf("Unexpected directive %s", word)
		}

		for {
			r = l.Next()
			switch {
			case r == leftRound:
				l.Emit(itemLeftRound)
				// Read until we see a right round.
				for {
					r = l.Next()
					if isSpace(r) || isEndOfLine(r) {
						l.Ignore()
						continue
					}
					if r == rightRound {
						// We are done with parsing this directive.
						l.Emit(itemRightRound)
						return nil
					}
					if isNameBegin(r) {
						// Start of a directive argument.
						for {
							r = l.Next()
							if isNameSuffix(r) {
								continue
							}
							l.Backup()
							l.Emit(itemText)
							break
						}
					}
				}
			case isSpace(r) || isEndOfLine(r):
				l.Ignore()
			default:
				l.Backup()
				return nil
			}
		}
	}
}

// isNameBegin returns true if the rune is an alphabet.
func isNameBegin(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
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
	if r == '_' || r == '.' || r == '-' { // Use by freebase.
		return true
	}
	return false
}

// isSpace returns true if the rune is a tab or space.
func isSpace(r rune) bool {
	return r == '\u0009' || r == '\u0020'
}

// isEndOfLine returns true if the rune is a Linefeed or a Carriage return.
func isEndOfLine(r rune) bool {
	return r == '\u000A' || r == '\u000D'
}

// IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
func isIRIChar(r rune, l *lex.Lexer) bool {
	if r <= 32 { // no chars b/w 0x00 to 0x20 inclusive
		return false
	}
	switch r {
	case '<':
	case '>':
	case '"':
	case '{':
	case '}':
	case '|':
	case '^':
	case '`':
	case '\\':
		r2 := l.Next()
		if r2 != 'u' && r2 != 'U' {
			l.Backup()
			return false
		}
		return hasUChars(r2, l)
	default:
		return true
	}
	return false
}

// UCHAR ::= '\u' HEX HEX HEX HEX | '\U' HEX HEX HEX HEX HEX HEX HEX HEX
func hasUChars(r rune, l *lex.Lexer) bool {
	if r != 'u' && r != 'U' {
		return false
	}
	times := 4
	if r == 'U' {
		times = 8
	}
	return times == l.AcceptRunTimes(isHex, times)
}

// HEX ::= [0-9] | [A-F] | [a-f]
func isHex(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
	case r >= 'a' && r <= 'f':
	case r >= 'A' && r <= 'F':
	default:
		return false
	}
	return true
}
