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

package lex

import (
	"errors"
	"fmt"
)

func LexIRIRef(l *Lexer, styp ItemType) error {
	l.Ignore() // ignore '<'
	l.AcceptRunRec(IsIRIChar)
	l.Emit(styp) // will emit without '<' and '>'
	r := l.Next()
	if r == EOF {
		return errors.New("Unexpected end of IRI.")
	}
	if r != '>' {
		return fmt.Errorf(
			"Unexpected character %q while parsing IRI", r)
	}
	l.Ignore() // ignore '>'
	return nil
}

// IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
func IsIRIChar(r rune, l *Lexer) bool {
	if r <= 32 { // no chars b/w 0x00 to 0x20 inclusive
		return false
	}
	switch r {
	case '<', '>', '"', '{', '}', '|', '^', '`':
		return false
	case '\\':
		r2 := l.Next()
		if r2 != 'u' && r2 != 'U' {
			l.Backup()
			return false
		}
		return HasUChars(r2, l)
	}
	return true
}

// UCHAR ::= '\u' HEX HEX HEX HEX | '\U' HEX HEX HEX HEX HEX HEX HEX HEX
func HasUChars(r rune, l *Lexer) bool {
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
