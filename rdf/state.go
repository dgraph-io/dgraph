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

// rdf package parses N-Quad statements based on
// http://www.w3.org/TR/n-quads/
package rdf

import (
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/x"
)

const (
	itemText       lex.ItemType = 5 + iota // plain text
	itemSubject                            // subject, 6
	itemPredicate                          // predicate, 7
	itemObject                             // object, 8
	itemLabel                              // label, 9
	itemLiteral                            // literal, 10
	itemLanguage                           // language, 11
	itemObjectType                         // object type, 12
	itemValidEnd                           // end with dot, 13
)

const (
	AT_SUBJECT int = iota
	AT_PREDICATE
	AT_OBJECT
	AT_LABEL
)

var glog = x.Log("rdf")

func run(l *lex.Lexer) {
	for state := lexText; state != nil; {
		state = state(l)
	}
	close(l.Items) // No more tokens.
}

func lexText(l *lex.Lexer) lex.StateFn {
Loop:
	for {
		switch r := l.Next(); {
		case r == '<' || r == '_':
			if l.Depth == AT_SUBJECT {
				l.Backup()
				l.Emit(itemText) // emit whatever we have so far.
				return lexSubject

			} else if l.Depth == AT_PREDICATE {
				l.Backup()
				l.Emit(itemText)
				return lexPredicate

			} else if l.Depth == AT_OBJECT {
				l.Backup()
				l.Emit(itemText)
				return lexObject

			} else {
				return l.Errorf("Invalid input: %v at lexText", r)
			}

		case r == '"':
			if l.Depth != AT_OBJECT {
				return l.Errorf("Invalid quote for non-object.")
			}
			l.Backup()
			l.Emit(itemText)
			return lexObject

		case r == lex.EOF:
			break Loop

		case r == '.':
			if l.Depth > AT_OBJECT {
				l.Emit(itemValidEnd)
			}
			break Loop
		}
	}
	if l.Pos > l.Start {
		l.Emit(itemText)
	}
	l.Emit(lex.ItemEOF)
	return nil
}

func lexUntilClosing(l *lex.Lexer, styp lex.ItemType,
	sfn lex.StateFn) lex.StateFn {

	l.AcceptUntil(isClosingBracket)
	r := l.Next()
	if r == lex.EOF {
		return l.Errorf("Unexpected end of subject")
	}
	if r == '>' {
		l.Emit(styp)
		return sfn
	}
	return l.Errorf("Invalid character %v found for itemType: %v", r, styp)
}

// Assumes that the current rune is '_'.
func lexBlankNode(l *lex.Lexer, styp lex.ItemType,
	sfn lex.StateFn) lex.StateFn {

	r := l.Next()
	if r != ':' {
		return l.Errorf("Invalid input RDF Blank Node found at pos: %v", r)
	}
	// RDF Blank Node.
	// TODO: At some point do checkings based on the guidelines. For now,
	// just accept everything until space.
	l.AcceptUntil(isSpace)
	r = l.Peek()
	if r == lex.EOF {
		return l.Errorf("Unexpected end of subject")
	}
	if isSpace(r) {
		l.Emit(styp)
		l.Depth += 1
		return sfn
	}
	return l.Errorf("Invalid character %v found for itemType: %v", r, styp)
}

func lexSubject(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r == '<' {
		l.Depth += 1
		return lexUntilClosing(l, itemSubject, lexText)
	}

	if r == '_' {
		return lexBlankNode(l, itemSubject, lexText)
	}

	return l.Errorf("Invalid character during lexSubject: %v", r)
}

func lexPredicate(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r != '<' {
		return l.Errorf("Invalid character in lexPredicate: %v", r)
	}
	l.Depth += 1
	return lexUntilClosing(l, itemPredicate, lexText)
}

func lexLanguage(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r != '@' {
		return l.Errorf("Expected @ prefix for lexLanguage")
	}
	l.Ignore()
	r = l.Next()
	if !isLangTagPrefix(r) {
		return l.Errorf("Invalid language tag prefix: %v", r)
	}
	l.AcceptRun(isLangTag)
	l.Emit(itemLanguage)
	return lexText
}

// Assumes '"' has already been encountered.
func lexLiteral(l *lex.Lexer) lex.StateFn {
	l.AcceptUntil(isEndLiteral)
	l.Emit(itemLiteral)
	l.Next()   // Move to end literal.
	l.Ignore() // Ignore end literal.
	l.Depth += 1

	r := l.Peek()
	if r == '@' {
		return lexLanguage(l)

	} else if r == '^' {
		return lexObjectType(l)

	} else {
		return lexText
	}
}

func lexObjectType(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r != '^' {
		return l.Errorf("Expected ^ for lexObjectType")
	}
	r = l.Next()
	if r != '^' {
		return l.Errorf("Expected ^^ for lexObjectType")
	}
	l.Ignore()
	r = l.Next()
	if r != '<' {
		return l.Errorf("Expected < for lexObjectType")
	}
	return lexUntilClosing(l, itemObjectType, lexText)
}

func lexObject(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r == '<' {
		l.Depth += 1
		return lexUntilClosing(l, itemObject, lexText)
	}
	if r == '_' {
		return lexBlankNode(l, itemObject, lexText)
	}
	if r == '"' {
		l.Ignore()
		return lexLiteral(l)
	}

	return l.Errorf("Invalid char: %v at lexObject", r)
}

func isClosingBracket(r rune) bool {
	return r == '>'
}

func isSpace(r rune) bool {
	return r == '\u0009' || r == '\u0020'
}

func isEndLiteral(r rune) bool {
	return r == '"' || r == '\u000d' || r == '\u000a'
}

func isLangTagPrefix(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	default:
		return false
	}
}

func isLangTag(r rune) bool {
	if isLangTagPrefix(r) {
		return true
	}

	switch {
	case r == '-':
		return true
	case r >= '0' && r <= '9':
		return true
	default:
		return false
	}
}
