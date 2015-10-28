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

const (
	leftCurl  = '{'
	rightCurl = '}'
)

// stateFn represents the state of the scanner as a function that
// returns the next state.
type stateFn func(*lexer) stateFn

func lexText(l *lexer) stateFn {
Loop:
	for {
		switch r := l.next(); {
		case r == leftCurl:
			l.backup()
			l.emit(itemText) // emit whatever we have so far.
			l.next()         // advance one to get back to where we saw leftCurl.
			l.depth += 1     // one level down.
			l.emit(itemLeftCurl)
			return lexInside // we're in.

		case r == rightCurl:
			return l.errorf("Too many right characters")
		case r == EOF:
			break Loop
		case isNameBegin(r):
			l.backup()
			l.emit(itemText)
			return lexOperationType
		}
	}
	if l.pos > l.start {
		l.emit(itemText)
	}
	l.emit(itemEOF)
	return nil
}

func lexInside(l *lexer) stateFn {
	for {
		switch r := l.next(); {
		case r == rightCurl:
			l.depth -= 1
			l.emit(itemRightCurl)
			if l.depth == 0 {
				return lexText
			}
		case r == leftCurl:
			l.depth += 1
			l.emit(itemLeftCurl)
		case r == EOF:
			return l.errorf("unclosed action")
		case isSpace(r) || isEndOfLine(r) || r == ',':
			l.ignore()
		case isNameBegin(r):
			return lexName
		case r == '#':
			l.backup()
			return lexComment
		case r == '(':
			l.emit(itemLeftRound)
			return lexArgInside
		default:
			return l.errorf("Unrecognized character in lexInside: %#U", r)
		}
	}
}

func lexName(l *lexer) stateFn {
	for {
		// The caller already checked isNameBegin, and absorbed one rune.
		r := l.next()
		if isNameSuffix(r) {
			continue
		}
		l.backup()
		l.emit(itemName)
		break
	}
	return lexInside
}

func lexComment(l *lexer) stateFn {
	for {
		r := l.next()
		if isEndOfLine(r) {
			l.emit(itemComment)
			return lexInside
		}
		if r == EOF {
			break
		}
	}
	if l.pos > l.start {
		l.emit(itemComment)
	}
	l.emit(itemEOF)
	return nil // Stop the run loop.
}

func lexOperationType(l *lexer) stateFn {
	for {
		r := l.next()
		if isNameSuffix(r) {
			continue // absorb
		}
		l.backup()
		word := l.input[l.start:l.pos]
		if word == "query" || word == "mutation" {
			l.emit(itemOpType)
		}
		break
	}
	return lexText
}

func lexArgInside(l *lexer) stateFn {
	for {
		switch r := l.next(); {
		case r == EOF:
			return l.errorf("unclosed argument")
		case isSpace(r) || isEndOfLine(r):
			l.ignore()
		case isNameBegin(r):
			return lexArgName
		case r == ':':
			l.ignore()
			return lexArgVal
		case r == ')':
			l.emit(itemRightRound)
			return lexInside
		case r == ',':
			l.ignore()
		}
	}
}

func lexArgName(l *lexer) stateFn {
	for {
		r := l.next()
		if isNameSuffix(r) {
			continue
		}
		l.backup()
		l.emit(itemArgName)
		break
	}
	return lexArgInside
}

func lexArgVal(l *lexer) stateFn {
	l.acceptRun(isSpace)
	l.ignore() // Any spaces encountered.
	for {
		r := l.next()
		if isSpace(r) || isEndOfLine(r) || r == ')' || r == ',' {
			l.backup()
			l.emit(itemArgVal)
			return lexArgInside
		}
		if r == EOF {
			return l.errorf("Reached EOF while reading var value: %v",
				l.input[l.start:l.pos])
		}
	}
	glog.Fatal("This shouldn't be reached.")
	return nil
}

func lexArgumentVal(l *lexer) stateFn {
	for {
		switch r := l.next(); {
		case isSpace(r):
			l.ignore()
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
	return false
}
