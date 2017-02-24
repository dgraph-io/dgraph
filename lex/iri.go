/*
 * Copyright 2017 Dgraph Labs, Inc.
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

package lex

import (
	"errors"
	"fmt"
)

func LexIRIRef(l *Lexer, styp ItemType) error {
	l.Ignore() // ignore '<'
	l.AcceptRunRec(isIRIChar)
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
func isIRIChar(r rune, l *Lexer) bool {
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
