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

import "encoding/binary"

func Tokenizer() interface{} { return RuneTokenizer{} }

type RuneTokenizer struct{}

func (RuneTokenizer) Name() string     { return "rune" }
func (RuneTokenizer) Type() string     { return "string" }
func (RuneTokenizer) Identifier() byte { return 0xfd }

func (t RuneTokenizer) Tokens(value interface{}) ([]string, error) {
	var toks []string
	for _, r := range value.(string) {
		var buf [binary.MaxVarintLen32]byte
		n := binary.PutVarint(buf[:], int64(r))
		tok := string(buf[:n])
		toks = append(toks, tok)
	}
	return toks, nil
}
