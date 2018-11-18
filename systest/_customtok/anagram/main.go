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

import "sort"

func Tokenizer() interface{} { return AnagramTokenizer{} }

type AnagramTokenizer struct{}

func (AnagramTokenizer) Name() string     { return "anagram" }
func (AnagramTokenizer) Type() string     { return "string" }
func (AnagramTokenizer) Identifier() byte { return 0xfc }

func (t AnagramTokenizer) Tokens(value interface{}) ([]string, error) {
	b := []byte(value.(string))
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return []string{string(b)}, nil
}
