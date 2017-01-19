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

// Package rdf package parses N-Quad statements based on
// http://www.w3.org/TR/n-quads/
package rdf

import (
	"strconv"

	"github.com/dgraph-io/dgraph/lex"
)

// The constants represent different types of lexed Items possible for an rdf N-Quad.
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

// These constants keep a track of the depth while parsing an rdf N-Quad.
const (
	atSubject int = iota
	atPredicate
	atObject
	atLabel
)

// This function inspects the next rune and calls the appropriate stateFn.
func lexText(l *lex.Lexer) lex.StateFn {
Loop:
	for {
		switch r := l.Next(); {
		case r == '<' || r == '_':
			if l.Depth == atSubject {
				l.Backup()
				l.Emit(itemText) // emit whatever we have so far.
				return lexSubject
			}

			if l.Depth == atPredicate {
				l.Backup()
				l.Emit(itemText)
				return lexPredicate
			}

			if l.Depth == atObject {
				l.Backup()
				l.Emit(itemText)
				return lexObject
			}

			if l.Depth == atLabel {
				l.Backup()
				l.Emit(itemText)
				return lexLabel
			}

			return l.Errorf("Invalid input: %c at lexText", r)

		case r == '"':
			if l.Depth != atObject {
				return l.Errorf("Invalid quote for non-object.")
			}
			l.Backup()
			l.Emit(itemText)
			return lexObject

		case r == lex.EOF:
			break Loop

		case r == '.':
			if l.Depth > atObject {
				l.Emit(itemValidEnd)
			}
			break Loop

		case isSpace(r):
			continue
		default:
			l.Errorf("Invalid input: %c at lexText", r)
		}
	}
	if l.Pos > l.Start {
		l.Emit(itemText)
	}
	l.Emit(lex.ItemEOF)
	return nil
}

// Assumes that caller has consumed initial '<'
func lexIRIRef(l *lex.Lexer, styp lex.ItemType,
	sfn lex.StateFn) lex.StateFn {
	l.AcceptRunRec(isIRIChar)
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

func lexUidNode(l *lex.Lexer, styp lex.ItemType, sfn lex.StateFn) lex.StateFn {
	l.AcceptUntil(isSpace)
	r := l.Peek()
	if r == lex.EOF {
		return l.Errorf("Unexpected end of uid subject")
	}

	in := l.Input[l.Start:l.Pos]
	if _, err := strconv.ParseUint(in[:], 0, 64); err != nil {
		return l.Errorf("Unable to convert '%v' to UID", in[:])
	}

	if isSpace(r) {
		l.Emit(styp)
		return sfn
	}

	return l.Errorf("Invalid character %v found for UID node itemType: %v", r,
		styp)
}

// Assumes that caller has consumed '_'.
func lexBlankNode(l *lex.Lexer, styp lex.ItemType,
	sfn lex.StateFn) lex.StateFn {
	r := l.Next()
	if r != ':' {
		return l.Errorf("Invalid character after _. Expected :, found %v", r)
	}
	l.AcceptUntil(isSpace)
	r = l.Peek()
	if r == lex.EOF {
		return l.Errorf("Unexpected end of subject")
	}

	if isSpace(r) {
		l.Emit(styp)
		return sfn
	}

	return l.Errorf("Invalid character %v found for itemType: %v", r, styp)
}

func lexSubject(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	// The subject is an IRI, so we lex till we encounter '>'.
	if r == '<' {
		l.Depth++
		return lexIRIRef(l, itemSubject, lexText)
	}

	// The subject represents a blank node.
	if r == '_' {
		l.Depth++
		return lexBlankNode(l, itemSubject, lexText)
	}
	// See if its an uid
	return lexUidNode(l, itemSubject, lexText)
}

func lexPredicate(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	// The predicate can only be an IRI according to the spec.
	if r != '<' {
		return l.Errorf("Invalid character in lexPredicate: %v", r)
	}

	l.Depth++
	return lexIRIRef(l, itemPredicate, lexText)
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
	for {
		r := l.Next()
		if r == '\u005c' { // backslash
			r = l.Next()
			continue // This would skip over the escaped rune.
		}

		if r == lex.EOF || isEndLiteral(r) {
			break
		}
	}
	l.Backup()

	l.Emit(itemLiteral)
	l.Next()   // Move to end literal.
	l.Ignore() // Ignore end literal.
	l.Depth++

	r := l.Peek()
	if r == '@' {
		return lexLanguage(l)
	}

	if r == '^' {
		return lexObjectType(l)
	}

	return lexText
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

	return lexIRIRef(l, itemObjectType, lexText)
}

func lexObject(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	// The object can be an IRI, blank node or a literal.
	if r == '<' {
		l.Depth++
		return lexIRIRef(l, itemObject, lexText)
	}

	if r == '_' {
		l.Depth++
		return lexBlankNode(l, itemObject, lexText)
	}

	if r == '"' {
		l.Ignore()
		return lexLiteral(l)
	}

	return l.Errorf("Invalid char: %v at lexObject", r)
}

func lexLabel(l *lex.Lexer) lex.StateFn {
	r := l.Next()
	// Graph label can either be an IRI or a blank node according to spec.
	if r == '<' {
		l.Depth++
		return lexIRIRef(l, itemLabel, lexText)
	}

	if r == '_' {
		l.Depth++
		return lexBlankNode(l, itemLabel, lexText)
	}
	return l.Errorf("Invalid char: %v at lexLabel", r)
}

func isClosingBracket(r rune) bool {
	return r == '>'
}

// isSpace returns true if the rune is a tab or space.
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

// isLangTag returns true if the rune is allowed by the RDF spec.
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
		times := 4
		if r2 != 'u' && r2 != 'U' {
			l.Backup()
			return false
		} else {
			if r2 == 'U' {
				times = 8
			}
			rs := l.AcceptRunTimes(isHex, times)
			return rs == times
		}
	default:
		return true
	}
	return false
}

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
