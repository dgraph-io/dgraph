/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"math/big"
	"testing"

	"github.com/dgraph-io/dgraph/types"
	"github.com/stretchr/testify/require"
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
			out: types.Val{Tid: types.FloatID, Value: 4.0},
		},
		{in: &mathTree{
			Fn: "-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 99.0},
		},
		{in: &mathTree{
			Fn: "*",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(3)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(3)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 9.0},
		},
		{in: &mathTree{
			Fn: "/",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(12)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(4)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 3.0},
		},
		{in: &mathTree{
			Fn: "%",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(10)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 0.0},
		},
		{in: &mathTree{
			Fn: "max",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 100.0},
		},
		{in: &mathTree{
			Fn: "min",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(1)}},
				{Const: types.Val{Tid: types.IntID, Value: int64(100)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 1.0},
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
}

func TestProcessBinaryBigFloat(t *testing.T) {
	tests := []struct {
		in  *mathTree
		out types.Val
	}{
		{in: &mathTree{
			Fn: "+",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(2.15)}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(1.15)}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(3.3)},
		},
		{in: &mathTree{
			Fn: "-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(100)}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(1)}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(99)},
		},
		{in: &mathTree{
			Fn: "*",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(3)}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(3)}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(9)},
		},
		{in: &mathTree{
			Fn: "/",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(12)}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(4)}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(3)},
		},
		{in: &mathTree{
			Fn: "max",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(1)}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(100)}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(100)},
		},
		{in: &mathTree{
			Fn: "min",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(1)}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(100)}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(1)},
		},
	}
	for _, tc := range tests {
		t.Logf("Test %s", tc.in.Fn)
		err := processBinary(tc.in)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, tc.in.Const)
	}
}

func TestProcessUnary(t *testing.T) {
	float3 := *new(big.Float).SetPrec(200)
	float3.SetFloat64(3.1)
	sqrt3 := *new(big.Float).SetPrec(200)
	sqrt3.Sqrt(&float3)

	tests := []struct {
		in  *mathTree
		out types.Val
	}{
		{in: &mathTree{
			Fn: "u-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.IntID, Value: int64(2)}},
			}},
			out: types.Val{Tid: types.FloatID, Value: -2.0},
		},
		{in: &mathTree{
			Fn: "u-",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: float3}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(-3.1).SetPrec(200)},
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
			Fn: "sqrt",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: float3}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: sqrt3},
		},
		{in: &mathTree{
			Fn: "floor",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: 2.5}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 2.0},
		},
		{in: &mathTree{
			Fn: "floor",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: sqrt3}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(1).SetPrec(200)},
		},
		{in: &mathTree{
			Fn: "ceil",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.FloatID, Value: 2.5}},
			}},
			out: types.Val{Tid: types.FloatID, Value: 3.0},
		},
		{in: &mathTree{
			Fn: "ceil",
			Child: []*mathTree{
				{Const: types.Val{Tid: types.BigFloatID, Value: sqrt3}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(2).SetPrec(200)},
		},
	}
	for _, tc := range tests {
		t.Logf("Test %s", tc.in.Fn)
		err := processUnary(tc.in)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, tc.in.Const)
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

func TestBigFloatMathsBoolean(t *testing.T) {
	tests := []struct {
		in  *mathTree
		out types.Val
	}{
		{in: &mathTree{
			Fn: "==",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.BigFloatID, Value: *big.NewFloat(2.123)}}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(2.123)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: true},
		},
		{in: &mathTree{
			Fn: "!=",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.BigFloatID, Value: *big.NewFloat(2.4623)}}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(3.623)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: true},
		},
		{in: &mathTree{
			Fn: ">=",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.BigFloatID, Value: *big.NewFloat(2.123)}}},
				{Val: map[uint64]types.Val{
					0: {Tid: types.BigFloatID, Value: *big.NewFloat(4.123)}}},
			}},
			out: types.Val{Tid: types.BoolID, Value: false},
		},
		{in: &mathTree{
			Fn: "<=",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.BigFloatID, Value: *big.NewFloat(2.123)}}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(3.992)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: true},
		},
		{in: &mathTree{
			Fn: ">",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.BigFloatID, Value: *big.NewFloat(2.45)}}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(3.43)}},
			}},
			out: types.Val{Tid: types.BoolID, Value: false},
		},
		{in: &mathTree{
			Fn: "<",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{
					0: {Tid: types.BigFloatID, Value: *big.NewFloat(2.1213)}}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(2.1232)}},
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
		{in: &mathTree{
			Fn: "cond",
			Child: []*mathTree{
				{Val: map[uint64]types.Val{0: {Tid: types.BoolID, Value: false}}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(1.456)}},
				{Const: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(2.123)}},
			}},
			out: types.Val{Tid: types.BigFloatID, Value: *big.NewFloat(2.123)},
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
