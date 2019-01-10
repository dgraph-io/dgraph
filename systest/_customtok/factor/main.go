/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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
		return nil, fmt.Errorf("Cannot factor int <= 1: %d", x)
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
