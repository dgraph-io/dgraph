/*
 * Copyright 2017-2025 Hypermode Inc. and Contributors
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

package lex

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type testCase struct {
	input     string
	expResult bool
}

func TestHasUChars(t *testing.T) {
	l := &Lexer{}
	testInputs := []testCase{{"u1def", true}, {"UADEFABCD", true},
		{"uYDQW", false}, {"Uydvxypqt", false}, {"abc", false}, {"uabcdg", true}}
	for _, test := range testInputs {
		l.Reset(test.input)
		r := l.Next()
		result := HasUChars(r, l)
		require.Equal(t, test.expResult, result)
	}
}

func TestHasXChars(t *testing.T) {
	l := &Lexer{}
	testInputs := []testCase{{"xad", true}, {"xAD", true}, {"xYD", false}, {"xyd", false}, {"abc", false}}
	for _, test := range testInputs {
		l.Reset(test.input)
		r := l.Next()
		result := HasXChars(r, l)
		require.Equal(t, test.expResult, result)
	}
}

func TestIsHex(t *testing.T) {
	type testCase struct {
		input     int32
		expResult bool
	}
	testInputs := []testCase{{'x', false}, {'X', false}, {'L', false}, {'A', true}, {'b', true}}
	for _, test := range testInputs {
		result := isHex(test.input)
		require.Equal(t, test.expResult, result)
	}
}

func TestIsIRIRefChar(t *testing.T) {
	testInputs := []testCase{{"\\u1def", true}, {"\\UADEFABCD", true}, {"A", true}, {"a", true},
		{"<", false}, {">", false}, {"{", false}, {"`", false}, {"	", false}, {"\\abc", false}}
	l := &Lexer{}
	for _, test := range testInputs {
		l.Reset(test.input)
		r := l.Next()
		result := isIRIRefChar(r, l)
		require.Equal(t, test.expResult, result)
	}
}
