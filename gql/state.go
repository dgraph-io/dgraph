/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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

package gql

import "github.com/dgraph-io/dgraph/lex"

const (
	leftCurl  = '{'
	rightCurl = '}'
)

const (
	itemText       lex.ItemType = 5 + iota // plain text
	itemLeftCurl                           // left curly bracket
	itemRightCurl                          // right curly bracket
	itemComment                            // comment
	itemName                               // [9] names
	itemOpType                             // operation type
	itemString                             // quoted string
	itemLeftRound                          // left round bracket
	itemRightRound                         // right round bracket
	itemArgName                            // argument name
	itemArgVal                             // argument val
)

func lexText(l *lex.Lexer) lex.StateFn {
Loop:
	for {
		switch r := l.Next(); {
		case r == leftCurl:
			l.Backup()
			l.Emit(itemText) // emit whatever we have so far.
			l.Next()         // advance one to get back to where we saw leftCurl.
			l.Depth += 1     // one level down.
			l.Emit(itemLeftCurl)
			return lexInside // we're in.

		case r == rightCurl:
			return l.Errorf("Too many right characters")
		case r == lex.EOF:
			break Loop
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

func lexInside(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == rightCurl:
			l.Depth -= 1
			l.Emit(itemRightCurl)
			if l.Depth == 0 {
				return lexText
			}
		case r == leftCurl:
			l.Depth += 1
			l.Emit(itemLeftCurl)
		case r == lex.EOF:
			return l.Errorf("unclosed action")
		case isSpace(r) || isEndOfLine(r) || r == ',':
			l.Ignore()
		case isNameBegin(r):
			return lexName
		case r == '#':
			l.Backup()
			return lexComment
		case r == '(':
			l.Emit(itemLeftRound)
			return lexArgInside
		default:
			return l.Errorf("Unrecognized character in lexInside: %#U", r)
		}
	}
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

func lexOperationType(l *lex.Lexer) lex.StateFn {
	for {
		r := l.Next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.Backup()
		word := l.Input[l.Start:l.Pos]
		if word == "query" || word == "mutation" {
			l.Emit(itemOpType)
		}
		break
	}
	return lexText
}

func lexArgInside(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case r == lex.EOF:
			return l.Errorf("unclosed argument")
		case isSpace(r) || isEndOfLine(r):
			l.Ignore()
		case isNameBegin(r):
			return lexArgName
		case r == ':':
			l.Ignore()
			return lexArgVal
		case r == ')':
			l.Emit(itemRightRound)
			return lexInside
		case r == ',':
			l.Ignore()
		}
	}
}

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

func lexArgVal(l *lex.Lexer) lex.StateFn {
	l.AcceptRun(isSpace)
	l.Ignore() // Any spaces encountered.
	for {
		r := l.Next()
		if isSpace(r) || isEndOfLine(r) || r == ')' || r == ',' {
			l.Backup()
			l.Emit(itemArgVal)
			return lexArgInside
		}
		if r == lex.EOF {
			return l.Errorf("Reached lex.EOF while reading var value: %v",
				l.Input[l.Start:l.Pos])
		}
	}
	glog.Fatal("This shouldn't be reached.")
	return nil
}

func lexArgumentVal(l *lex.Lexer) lex.StateFn {
	for {
		switch r := l.Next(); {
		case isSpace(r):
			l.Ignore()
		}
	}
}

func isSpace(r rune) bool {
	return r == '\u0009' || r == '\u0020'
}

func isEndOfLine(r rune) bool {
	return r == '\u000A' || r == '\u000D'
}

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
