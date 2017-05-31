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

package types

import (
	"unicode/utf8"

	"github.com/dgraph-io/dgraph/x"
)

// Should be used only in filtering arg1 by comparing with arg2.
// arg2 is reference Val to which arg1 is compared.
func CompareVals(op string, arg1, arg2 Val) bool {
	negateRes := func(b bool, e error) (bool, error) { // reverses result
		return !b, e
	}
	noError := func(b bool, e error) bool {
		return b && e == nil
	}
	switch op {
	case "ge":
		return noError(negateRes(Less(arg1, arg2)))
	case "gt":
		return noError(Less(arg2, arg1))
	case "le":
		return noError(negateRes(Less(arg2, arg1)))
	case "lt":
		return noError(Less(arg1, arg2))
	case "eq":
		return noError(Equal(arg1, arg2))
	default:
		// should have been checked at query level.
		x.Fatalf("Unknown ineqType %v", op)
	}
	return false
}

func Args(val string) ([]string, error) {
	// parses and array of string tokens and returns them as a slice of strings.
	var tokens []string
	// Empty val is checked in parser so safe to access first index here.
	if val[0] != '[' {
		return []string{val}, nil
	}

	if val[len(val)-1] != ']' {
		return tokens, x.Errorf("Expected ]. Got: %q", val[len(val)-1])
	}

	var expectArg bool
	// lets truncate [
	val = val[1:]
	argStart := 0
	// L:
	for i, w := 0, 0; i < len(val); i += w {
		r, width := utf8.DecodeRuneInString(val[i:])
		if expectArg {
			// Lets collect everything till unescaped ".
			for i < len(val) {
				r, width := utf8.DecodeRuneInString(val[i:])
				if r == '"' && i > 0 {
					if val[i-1] == '\\' {
						i = i + width
						continue
					} else {
						tokens = append(tokens, val[argStart:i])
						expectArg = false
						i = i + width
						break
					}
				} else {
					// Accept other things.
					i = i + w
					continue
				}
			}
		}
		w = width
		// Lets ignore spaces that are not part of the token.
		if r == '"' {
			if !expectArg {
				argStart = i + 1
			}
			expectArg = !expectArg
			continue
		}
		if r == ',' || r == ' ' {
			continue
		}
		if r == ']' {
			break
		}
		if r == '[' || r == ')' {
			return tokens, x.Errorf("Invalid syntax for tokens. Got: %+v", val)
		}
	}
	return tokens, nil
}
