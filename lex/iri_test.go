/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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
