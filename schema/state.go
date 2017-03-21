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
	"github.com/dgraph-io/dgraph/lex"
)

// Constants representing type of different graphql lexed items.
const (
	itemText       lex.ItemType = 5 + iota // plain text
	itemLeftCurl                           // left curly bracket
	itemRightCurl                          // right curly bracket
	itemColon                              // colon
	itemLeftRound                          // left round bracket
	itemRightRound                         // right round bracket
	itemAt
	itemDot
)

func lexText(l *lex.Lexer) lex.StateFn {
Loop:
	for {
		switch r := l.Next(); {
		case r == lex.EOF:
			break Loop
		case isNameBegin(r):
			l.Backup()
			return lexWord
		case isSpace(r) || isEndOfLine(r) || r == ',':
			l.Ignore()
		case r == '<':
			if err := lex.LexIRIRef(l, itemText); err != nil {
				return l.Errorf("Invalid schema: %v", err)
			}
		case r == '{':
			l.Emit(itemLeftCurl)
		case r == '}':
			l.Emit(itemRightCurl)
		case r == '(':
			l.Emit(itemLeftRound)
		case r == ')':
			l.Emit(itemRightRound)
		case r == ':':
			l.Emit(itemColon)
		case r == '@':
			l.Emit(itemAt)
		case r == '.':
			l.Emit(itemDot)
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

func lexWord(l *lex.Lexer) lex.StateFn {
	for {
		// The caller already checked isNameBegin, and absorbed one rune.
		r := l.Next()
		if isNameSuffix(r) {
			continue
		}
		l.Backup()
		l.Emit(itemText)
		break
	}
	return lexText
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
