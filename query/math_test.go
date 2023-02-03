/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/types"
)

func TestProcessBinary(t *testing.T) {
	tests := []struct {
		in  *mathTree
		out types.Val
	}{
		{in: &mathTree{
			Fn: "+",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(4)},
		},
		{in: &mathTree{
			Fn: "+",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(2)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(4)},
		},
		{in: &mathTree{
			Fn: "+",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(2)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(2)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(4)},
		},
		{in: &mathTree{
			Fn: "+",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(48038396025285290)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(48038396025285292)},
		},
		{in: &mathTree{
			Fn: "-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(99)},
		},
		{in: &mathTree{
			Fn: "-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(100)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(99)},
		},
		{in: &mathTree{
			Fn: "-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(100)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(1)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(99)},
		},
		{in: &mathTree{
			Fn: "*",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(3)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(3)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(9)},
		},
		{in: &mathTree{
			Fn: "*",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(3)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(3)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(9)},
		},
		{in: &mathTree{
			Fn: "*",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(3)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(3)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(9)},
		},
		{in: &mathTree{
			Fn: "/",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(12)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(4)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(3)},
		},
		{in: &mathTree{
			Fn: "/",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(12)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(4)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(3)},
		},
		{in: &mathTree{
			Fn: "/",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(12)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(4)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(3)},
		},
		{in: &mathTree{
			Fn: "%",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(10)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(0)},
		},
		{in: &mathTree{
			Fn: "%",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(10)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(0)},
		},
		{in: &mathTree{
			Fn: "%",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(10)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(2)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(0)},
		},
		{in: &mathTree{
			Fn: "max",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(100.0)},
		},
		{in: &mathTree{
			Fn: "max",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(1)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(100.0)},
		},
		{in: &mathTree{
			Fn: "max",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(1)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(100)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(100.0)},
		},
		{in: &mathTree{
			Fn: "min",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(1.0)},
		},
		{in: &mathTree{
			Fn: "min",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(1)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(1.0)},
		},
		{in: &mathTree{
			Fn: "min",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(1)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(100)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: float64(1.0)},
		},
		{in: &mathTree{
			Fn: "logbase",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(16)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 4.0},
		},
		{in: &mathTree{
			Fn: "pow",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(3)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 8.0},
		},
	}
	for _, tc := range tests {
		t.Logf("Test %s", tc.in.Fn)
		err := processBinary(tc.in)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, tc.in.Const)
	}

	errorTests := []struct {
		name string
		in   *mathTree
		err  error
	}{
		{in: &mathTree{
			Fn: "+",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(9223372036854775800)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(10)}},
			}},
			err:  ErrorIntOverflow,
			name: "Addition integer overflow",
		},
		{in: &mathTree{
			Fn: "+",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(-10)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(-9223372036854775800)}},
			}},
			err:  ErrorIntOverflow,
			name: "Addition integer underflow",
		},
		{in: &mathTree{
			Fn: "-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(9223372036854775800)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(-10)}},
			}},
			err:  ErrorIntOverflow,
			name: "Subtraction integer overflow",
		},
		{in: &mathTree{
			Fn: "-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(-10)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(9223372036854775800)}},
			}},
			err:  ErrorIntOverflow,
			name: "Subtraction integer underflow",
		},
		{in: &mathTree{
			Fn: "*",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(9223372036854775)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(10000)}},
			}},
			err:  ErrorIntOverflow,
			name: "Multiplication integer overflow",
		},
		{in: &mathTree{
			Fn: "*",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(-10000)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(9223372036854775)}},
			}},
			err:  ErrorIntOverflow,
			name: "Multiplication integer underflow",
		},
		{in: &mathTree{
			Fn: "/",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(23)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(0)}},
			}},
			err:  ErrorDivisionByZero,
			name: "Division int zero",
		},
		{in: &mathTree{
			Fn: "/",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(23)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(0)}},
			}},
			err:  ErrorDivisionByZero,
			name: "Division float zero",
		},
		{in: &mathTree{
			Fn: "%",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(23)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(0)}},
			}},
			err:  ErrorDivisionByZero,
			name: "Modulo int zero",
		},
		{in: &mathTree{
			Fn: "%",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(23)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(0)}},
			}},
			err:  ErrorDivisionByZero,
			name: "Modulo float zero",
		},
		{in: &mathTree{
			Fn: "pow",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(-2)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(1.7)}},
			}},
			err:  ErrorFractionalPower,
			name: "Fractional negative power",
		},
		{in: &mathTree{
			Fn: "logbase",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(-2)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			err:  ErrorNegativeLog,
			name: "Log negative integer numerator",
		},
		{in: &mathTree{
			Fn: "logbase",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(-2)}},
			}},
			err:  ErrorNegativeLog,
			name: "Log negative integer denominator",
		},
		{in: &mathTree{
			Fn: "logbase",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(-2)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(2)}},
			}},
			err:  ErrorNegativeLog,
			name: "Log negative float numerator",
		},
		{in: &mathTree{
			Fn: "logbase",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(2)}},
				{Const: types.Val{Tid: types.FloatID, Value: float64(-2)}},
			}},
			err:  ErrorNegativeLog,
			name: "Log negative float denominator",
		},
	}

	for _, tc := range errorTests {
		t.Logf("Test %s", tc.name)
		err := processBinary(tc.in)
		require.EqualError(t, err, tc.err.Error())
	}
}

func TestProcessUnary(t *testing.T) {
	tests := []struct {
		in  *mathTree
		out types.Val
	}{
		{in: &mathTree{
			Fn: "u-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(-2.0)},
		},
		{in: &mathTree{
			Fn: "ln",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(15)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 2.70805020110221},
		},
		{in: &mathTree{
			Fn: "exp",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 2.718281828459045},
		},
		{in: &mathTree{
			Fn: "sqrt",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: 9.0}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 3.0},
		},
		{in: &mathTree{
			Fn: "floor",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: 2.5}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 2.0},
		},
		{in: &mathTree{
			Fn: "ceil",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: 2.5}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 3.0},
		},
	}
	for _, tc := range tests {
		t.Logf("Test %s", tc.in.Fn)
		err := processUnary(tc.in)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, tc.in.Const)
	}

	errorTests := []struct {
		name string
		in   *mathTree
		err  error
	}{
		{in: &mathTree{
			Fn: "ln",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(-2)}},
			}},
			err:  ErrorNegativeLog,
			name: "Negative int ln",
		},
		{in: &mathTree{
			Fn: "ln",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(-2)}},
			}},
			err:  ErrorNegativeLog,
			name: "Negative float ln",
		},
		{in: &mathTree{
			Fn: "u-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(math.MinInt64)}},
			}},
			err:  ErrorIntOverflow,
			name: "Negation int overflow",
		},
		{in: &mathTree{
			Fn: "sqrt",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(-2)}},
			}},
			err:  ErrorNegativeRoot,
			name: "Negative int sqrt",
		},
		{in: &mathTree{
			Fn: "sqrt",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: float64(-2)}},
			}},
			err:  ErrorNegativeRoot,
			name: "Negative float sqrt",
		},
	}

	for _, tc := range errorTests {
		t.Logf("Test %s", tc.name)
		err := processUnary(tc.in)
		require.EqualError(t, err, tc.err.Error())
	}
}

func TestProcessBinaryBoolean(t *testing.T) {
	tests := []struct {
		in  *mathTree
		out types.Val
	}{
		{in: &mathTree{
			Fn: "<",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.IntID, Value: int64(1)}}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: true},
		},
		{in: &mathTree{
			Fn: ">",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.IntID, Value: int64(1)}}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: false},
		},
		{in: &mathTree{
			Fn: "<=",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.IntID, Value: int64(1)}}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: true},
		},
		{in: &mathTree{
			Fn: ">=",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.IntID, Value: int64(1)}}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: false},
		},
		{in: &mathTree{
			Fn: "==",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.IntID, Value: int64(1)}}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: false},
		},
		{in: &mathTree{
			Fn: "!=",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.IntID, Value: int64(1)}}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: true},
		},
	}
	for _, tc := range tests {
		t.Logf("Test %s", tc.in.Fn)
		err := processBinaryBoolean(tc.in)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, tc.in.Val[0])
	}
}

func TestProcessTernary(t *testing.T) {
	tests := []struct {
		in  *mathTree
		out types.Val
	}{
		{in: &mathTree{
			Fn: "cond",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{0: {Tid: types.BoolID, Value: true}}},
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.IntID, Value: int64(1)},
		},
		{in: &mathTree{
			Fn: "cond",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{0: {Tid: types.BoolID, Value: false}}},
				{Const: types.Val{Tid: types.FloatID, Value: 1.0}},
				{Const: types.Val{Tid: types.FloatID, Value: 2.0}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 2.0},
		},
	}
	for _, tc := range tests {
		t.Logf("Test %s", tc.in.Fn)
		err := processTernary(tc.in)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, tc.in.Val[0])
	}
}

func TestEvalMathTree(t *testing.T) {}
