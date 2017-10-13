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

package main

import (
	"encoding/binary"
	"fmt"
)

func Tokenizer() interface{} { return FactorTokenizer{} }

type FactorTokenizer struct{}

func (FactorTokenizer) Name() string     { return "factor" }
func (FactorTokenizer) Type() string     { return "int" }
func (FactorTokenizer) Identifier() byte { return 0xfe }

func (FactorTokenizer) Tokens(value interface{}) ([]string, error) {
	x := value.(int64)
	if x <= 1 {
		return nil, fmt.Errorf("cannot factor int <= 1: %d", x)
	}
	var toks []string
	for p := int64(2); x > 1; p++ {
		if x%p == 0 {
			toks = append(toks, encodeInt(p))
			for x%p == 0 {
				x /= p
			}
		}
	}
	return toks, nil

}

func encodeInt(x int64) string {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutVarint(buf[:], x)
	return string(buf[:n])
}
