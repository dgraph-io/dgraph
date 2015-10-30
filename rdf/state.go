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

import "github.com/dgraph-io/dgraph/lex"

const (
	itemText      lex.ItemType = 5 + iota // plain text
	itemSubject                           // subject
	itemPredicate                         // predicate
	itemObject                            // object
	itemLabel                             // label
)

const (
	AT_SUBJECT int = iota
	AT_PREDICATE
	AT_OBJECT
	AT_LABEL
)

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
		case r == '.' || r == lex.EOF:
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
		l.Depth += 1
		return sfn
	}
	return l.Errorf("Invalid character %v found for itemType: %v", r, styp)
}

func lexSubject(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r == '<' {
		return lexUntilClosing(l, itemSubject, lexText)
	}

	if r == '_' {
		r = l.Next()
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
			l.Emit(itemSubject)
			l.Depth += 1
			return lexText
		}
	}

	return l.Errorf("Invalid character during lexSubject: %v", r)
}

func lexPredicate(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r != '<' {
		return l.Errorf("Invalid character in lexPredicate: %v", r)
	}
	return lexUntilClosing(l, itemPredicate, lexText)
}

func lexObject(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	if r == '<' {
		return lexUntilClosing(l, itemObject, lexText)
	}
	return l.Errorf("Invalid char: %v at lexObject", r)
}

func isClosingBracket(r rune) bool {
	return r == '>'
}

func isSpace(r rune) bool {
	return r == '\u0009' || r == '\u0020'
}
