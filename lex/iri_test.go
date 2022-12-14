/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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

func TestHasUChars(t *testing.T) {
	l := &Lexer{}
	testInputs := []string{"u1def", "UADEFABCD", "uYDQW", "Uydvxypqt", "abc", "uabcdg"}
	expectedResult := []bool{true, true, false, false, false, true}
	for i, line := range testInputs {
		l.Reset(line)
		r := l.Next()
		result := HasUChars(r, l)
		require.Equal(t, expectedResult[i], result)

	}

}

func TestHasXChars(t *testing.T) {
	l := &Lexer{}
	testInputs := []string{"xad", "xAD", "xYD", "xyd", "abc"}
	expectedResult := []bool{true, true, false, false, false}
	for i, line := range testInputs {
		l.Reset(line)
		r := l.Next()
		result := HasXChars(r, l)
		require.Equal(t, expectedResult[i], result)

	}

}

func TestIsHex(t *testing.T) {
	testinputs := []int32{'x', 'X', 'L', 'x', 'b', 'B'}
	expectedResult := []bool{false, false, false, false, true, true}
	for i, char := range testinputs {
		result := isHex(char)
		require.Equal(t, expectedResult[i], result)
	}

}

func TestIsIRIRefChar(t *testing.T) {
	testInputs := []string{"\\u1def", "\\UADEFABCD", "A", "a", "<", ">", "{", "}", "`", "	", "\\abc"}
	expectedResult := []bool{true, true, true, true, false, false, false, false, false, false, false}
	l := &Lexer{}
	for i, line := range testInputs {
		l.Reset(line)
		r := l.Next()
		result := isIRIRefChar(r, l)
		require.Equal(t, expectedResult[i], result)

	}

}
