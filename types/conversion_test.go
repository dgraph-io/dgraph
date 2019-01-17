/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package types

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSameConversionString(t *testing.T) {
	data := []struct {
		in  Val
		out Val
	}{
		{Val{StringID, []byte("a")}, Val{StringID, "a"}},
		{Val{StringID, []byte("")}, Val{StringID, ""}},
		{Val{DefaultID, []byte("abc")}, Val{StringID, "abc"}},
	}

	for _, tc := range data {
		if v, err := Convert(tc.in, StringID); err != nil {
			t.Errorf("Unexpected error converting int to bool: %v", err)
		} else if v != tc.out {
			t.Errorf("Converting string to string: Expected %v, got %v", tc.out, v)
		}
	}
}

func TestConvertToDefault(t *testing.T) {
	data := []struct {
		in  Val
		out Val
	}{
		{Val{StringID, []byte("a")}, Val{DefaultID, "a"}},
		{Val{StringID, []byte("")}, Val{DefaultID, ""}},
		{Val{DefaultID, []byte("abc")}, Val{DefaultID, "abc"}},
		{Val{BinaryID, []byte("2016")}, Val{DefaultID, "2016"}},
	}

	for _, tc := range data {
		if v, err := Convert(tc.in, DefaultID); err != nil {
			t.Errorf("Unexpected error converting int to bool: %v", err)
		} else if !reflect.DeepEqual(v, tc.out) {
			t.Errorf("Converting string to string: Expected %v, got %v", tc.out, v)
		}
	}
}

func TestConvertFromDefault(t *testing.T) {
	data := []struct {
		in  Val
		out Val
		typ TypeID
	}{
		{Val{DefaultID, []byte("1")}, Val{IntID, int64(1)}, IntID},
		{Val{DefaultID, []byte("1.3")}, Val{FloatID, 1.3}, FloatID},
		{Val{DefaultID, []byte("true")}, Val{BoolID, true}, BoolID},
		{Val{DefaultID, []byte("2016")}, Val{BinaryID, []byte("2016")}, BinaryID},
	}

	for _, tc := range data {
		if v, err := Convert(tc.in, tc.typ); err != nil {
			t.Errorf("Unexpected error converting int to bool: %v", err)
		} else if !reflect.DeepEqual(v, tc.out) {
			t.Errorf("Converting string to string: Expected %+v, got %+v", tc.out, v)
		}
	}
}

func TestConversionToDateTime(t *testing.T) {
	data := []struct {
		in  Val
		out time.Time
	}{
		{
			Val{StringID, []byte("2006-01-02T15:04:05")},
			time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC),
		},
		{
			Val{StringID, []byte("2006-01-02")},
			time.Date(2006, 01, 02, 0, 0, 0, 0, time.UTC),
		},
		{
			Val{StringID, []byte("2006-01")},
			time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC),
		},
		{
			Val{StringID, []byte("2006")},
			time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range data {
		if val, err := Convert(tc.in, DateTimeID); err != nil {
			t.Errorf("Unexpected error converting string to datetime: %v", err)
		} else if !tc.out.Equal(val.Value.(time.Time)) {
			t.Errorf("Converting string to datetime: Expected %+v, got %+v", tc.out, val.Value)
		}
	}
}

func TestConvertFromPassword(t *testing.T) {
	data := []struct {
		in  Val
		out Val
		typ TypeID
	}{
		{Val{PasswordID, []byte("")}, Val{StringID, ""}, StringID},
		{Val{PasswordID, []byte("testing")}, Val{StringID, "testing"}, StringID},
	}

	for _, tc := range data {
		if val, err := Convert(tc.in, tc.typ); err != nil {
			t.Errorf("Unexpected error converting to string: %v", err)
		} else if !reflect.DeepEqual(val, tc.out) {
			t.Errorf("Converting password to string: Expected %+v, got %+v", tc.out, val)
		}
	}
}

func TestConvertToPassword(t *testing.T) {
	data := []struct {
		in       Val
		out      Val
		hasError bool
	}{
		{Val{IntID, []byte{0, 0, 0, 0, 0, 0, 0, 1}}, Val{PasswordID, ""}, true},
		{Val{FloatID, []byte{0, 0, 0, 0, 0, 0, 0, 1}}, Val{PasswordID, ""}, true},
		{Val{BoolID, []byte{1}}, Val{PasswordID, ""}, true},
		{Val{StringID, []byte("")}, Val{PasswordID, ""}, true},
		{Val{StringID, []byte("testing")}, Val{PasswordID, "$2a$10$"}, false},
		{Val{PasswordID, []byte("testing")}, Val{PasswordID, "testing"}, false},
		{Val{DefaultID, []byte("testing")}, Val{PasswordID, "$2a$10$"}, false},
	}

	for i, tc := range data {
		val, err := Convert(tc.in, PasswordID)
		if err != nil && !tc.hasError {
			t.Errorf("test#%d Unexpected error converting to string: %v", i+1, err)
		} else if err == nil && tc.hasError {
			t.Errorf("test#%d Expected an error but got instead: %+v", i+1, val)
		} else if !strings.HasPrefix(val.Value.(string), tc.out.Value.(string)) {
			t.Errorf("test#%d Converting string to password: Expected %+v, got %+v", i+1, tc.out, val)
		}
	}
}

func TestConvertIntToBool(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{Tid: IntID, Value: []byte{3, 0, 0, 0, 0, 0, 0, 0}}, out: Val{Tid: BoolID, Value: true}},
		{in: Val{Tid: IntID, Value: []byte{253, 255, 255, 255, 255, 255, 255, 255}}, out: Val{Tid: BoolID, Value: true}},
		{in: Val{Tid: IntID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}}, out: Val{Tid: BoolID, Value: false}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertFloatToBool(t *testing.T) {
	tests := []struct {
		n   float64
		in  Val
		out Val
	}{
		{n: 3.0, in: Val{Tid: FloatID, Value: []byte{7, 95, 152, 76, 21, 140, 11, 64}}, out: Val{Tid: BoolID, Value: true}},
		{n: -3.5, in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 12, 192}}, out: Val{Tid: BoolID, Value: true}},
		{n: 0, in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}}, out: Val{Tid: BoolID, Value: false}},
		{n: math.NaN(), in: Val{Tid: FloatID, Value: []byte{1, 0, 0, 0, 0, 0, 248, 127}}, out: Val{Tid: BoolID, Value: true}},
		{n: math.Inf(1), in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 240, 127}}, out: Val{Tid: BoolID, Value: true}},
		{n: math.Inf(-1), in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 240, 255}}, out: Val{Tid: BoolID, Value: true}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out, "%f should be %t", tc.n, tc.out.Value)
	}
}

func TestConvertStringToBool(t *testing.T) {
	tests := []struct {
		in   Val
		out  Val
		fail bool
	}{
		{in: Val{Tid: StringID, Value: []byte("1")}, out: Val{Tid: BoolID, Value: true}},
		{in: Val{Tid: StringID, Value: []byte("true")}, out: Val{Tid: BoolID, Value: true}},
		{in: Val{Tid: StringID, Value: []byte("True")}, out: Val{Tid: BoolID, Value: true}},
		{in: Val{Tid: StringID, Value: []byte("T")}, out: Val{Tid: BoolID, Value: true}},
		{in: Val{Tid: StringID, Value: []byte("F")}, out: Val{Tid: BoolID, Value: false}},
		{in: Val{Tid: StringID, Value: []byte("0")}, out: Val{Tid: BoolID, Value: false}},
		{in: Val{Tid: StringID, Value: []byte("false")}, out: Val{Tid: BoolID, Value: false}},
		{in: Val{Tid: StringID, Value: []byte("False")}, out: Val{Tid: BoolID, Value: false}},
		{in: Val{Tid: StringID, Value: []byte("srfrog")}, fail: true},
		{in: Val{Tid: StringID, Value: []byte("")}, out: Val{Tid: BoolID, Value: false}},
		{in: Val{Tid: StringID, Value: []byte("3")}, fail: true},
		{in: Val{Tid: StringID, Value: []byte("-3")}, fail: true},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, BoolID)
		if tc.fail {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertDateTimeToBool(t *testing.T) {
	val, err := time.Now().MarshalBinary()
	require.NoError(t, err)
	in := Val{Tid: DateTimeID, Value: val}
	_, err = Convert(in, BoolID)
	require.Error(t, err)
	require.EqualError(t, err, "Cannot convert datetime to type bool")
}

func TestConvertBoolToDateTime(t *testing.T) {
	in := Val{Tid: BoolID, Value: []byte{1}}
	_, err := Convert(in, DateTimeID)
	require.Error(t, err)
	require.EqualError(t, err, "Cannot convert bool to type datetime")
}

func TestConvertBoolToInt(t *testing.T) {
	tests := []struct {
		in      Val
		out     Val
		failure string
	}{
		{in: Val{Tid: BoolID, Value: []byte{1}}, out: Val{Tid: IntID, Value: int64(1)}},
		{in: Val{Tid: BoolID, Value: []byte{0}}, out: Val{Tid: IntID, Value: int64(0)}},
		{in: Val{Tid: BoolID, Value: []byte{3}},
			failure: "Invalid value for bool 3"},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, IntID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestTruthy(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{
			in:  Val{Tid: StringID, Value: []byte("true")},
			out: Val{Tid: BoolID, Value: true},
		},
		{
			in:  Val{Tid: DefaultID, Value: []byte("true")},
			out: Val{Tid: BoolID, Value: true},
		},
		{
			in:  Val{Tid: IntID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 1}},
			out: Val{Tid: BoolID, Value: true},
		},
		{
			in:  Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 1}},
			out: Val{Tid: BoolID, Value: true},
		},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, tc.out.Tid)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestFalsy(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{
			in:  Val{Tid: StringID, Value: []byte("false")},
			out: Val{Tid: BoolID, Value: false},
		},
		{
			in:  Val{Tid: DefaultID, Value: []byte("false")},
			out: Val{Tid: BoolID, Value: false},
		},
		{
			in:  Val{Tid: IntID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}},
			out: Val{Tid: BoolID, Value: false},
		},
		{
			in:  Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}},
			out: Val{Tid: BoolID, Value: false},
		},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, tc.out.Tid)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

/*
func TestSameConversionFloat(t *testing.T) {
	data := []struct {
		in  Float
		out Float
	}{
		{3.4434, 3.4434},
		{-3, -3},
		{0.5e2, 0.5e2},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, FloatID); err != nil {
			t.Errorf("Unexpected error converting int to bool: %v", err)
		} else if *(out.(*Float)) != tc.out {
			t.Errorf("Converting float to float: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestSameConversionInt(t *testing.T) {
	data := []struct {
		in  Int32
		out Int32
	}{
		{3, 3},
		{-3, -3},
		{0, 0},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, IntID); err != nil {
			t.Errorf("Unexpected error converting int to bool: %v", err)
		} else if *(out.(*Int32)) != tc.out {
			t.Errorf("Converting int to int: Expected %v, got %v", tc.out, out)
		}
	}
}


func TestConvertInt32ToBool(t *testing.T) {
	data := []struct {
		in  Int32
		out Bool
	}{
		{3, true},
		{-3, true},
		{0, false},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, BoolID); err != nil {
			t.Errorf("Unexpected error converting int to bool: %v", err)
		} else if *(out.(*Bool)) != tc.out {
			t.Errorf("Converting int to bool: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertFloatToBool(t *testing.T) {
	data := []struct {
		in  Float
		out Bool
	}{
		{3.0, true},
		{-3.5, true},
		{0, false},
		{-0.0, false},
		{Float(math.NaN()), true},
		{Float(math.Inf(1)), true},
		{Float(math.Inf(-1)), true},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, BoolID); err != nil {
			t.Errorf("Unexpected error converting float to bool: %v", err)
		} else if *(out.(*Bool)) != tc.out {
			t.Errorf("Converting float to bool: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertStringToBool(t *testing.T) {
	data := []struct {
		in  String
		out Bool
	}{
		{"1", true},
		{"true", true},
		{"True", true},
		{"T", true},
		{"F", false},
		{"0", false},
		{"false", false},
		{"False", false},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, BoolID); err != nil {
			t.Errorf("Unexpected error converting string to bool: %v", err)
		} else if *(out.(*Bool)) != tc.out {
			t.Errorf("Converting string to bool: Expected %v, got %v", tc.out, out)
		}
	}

	errData := []String{
		"hello",
		"",
		"3",
		"-3",
	}

	for _, tc := range errData {
		if out, err := Convert(&tc, BoolID); err == nil {
			t.Errorf("Expected error converting string %s to bool %v", tc, out)
		}
	}
}

func TestConvertDateTimeToBool(t *testing.T) {
	tm := Time{time.Now()}
	if _, err := Convert(&tm, BoolID); err == nil {
		t.Errorf("Expected error converting time to bool")
	}
}

func TestConvertBoolToInt32(t *testing.T) {
	data := []struct {
		in  Bool
		out Int32
	}{
		{true, 1},
		{false, 0},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, IntID); err != nil {
			t.Errorf("Unexpected error converting bool to int: %v", err)
		} else if *(out.(*Int32)) != tc.out {
			t.Errorf("Converting bool to in: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertFloatToInt32(t *testing.T) {
	data := []struct {
		in  Float
		out Int32
	}{
		{3.0, 3},
		{-3.5, -3},
		{0, 0},
		{-0.0, 0},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, IntID); err != nil {
			t.Errorf("Unexpected error converting float to int: %v", err)
		} else if *(out.(*Int32)) != tc.out {
			t.Errorf("Converting float to int: Expected %v, got %v", tc.out, out)
		}
	}
	errData := []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		522638295213.3243,
		-522638295213.3243,
	}
	for _, tc := range errData {
		if out, err := Convert((*Float)(&tc), IntID); err == nil {
			t.Errorf("Expected error converting float %f to int %v", tc, out)
		}
	}
}

func TestConvertStringToInt32(t *testing.T) {
	data := []struct {
		in  String
		out Int32
	}{
		{"1", 1},
		{"13816", 13816},
		{"-1221", -1221},
		{"0", 0},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, IntID); err != nil {
			t.Errorf("Unexpected error converting string to int: %v", err)
		} else if *(out.(*Int32)) != tc.out {
			t.Errorf("Converting string to int: Expected %v, got %v", tc.out, out)
		}
	}

	errData := []String{
		"hello",
		"",
		"3.0",
		"-3a.5",
		"203716381366627",
	}

	for _, tc := range errData {
		if out, err := Convert(&tc, IntID); err == nil {
			t.Errorf("Expected error converting string %s to int %v", tc, out)
		}
	}
}

func TestConvertDateTimeToInt32(t *testing.T) {
	data := []struct {
		in  time.Time
		out Int32
	}{
		{time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC), 1257894000},
		{time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC), -4410000},
	}
	for _, tc := range data {
		if out, err := Convert(&Time{tc.in}, IntID); err != nil {
			t.Errorf("Unexpected error converting time to int: %v", err)
		} else if *(out.(*Int32)) != tc.out {
			t.Errorf("Converting time to int: Expected %v, got %v", tc.out, out)
		}
	}

	errData := []time.Time{
		time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC),
		time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC),
	}

	for _, tc := range errData {
		if out, err := Convert(&Time{tc}, IntID); err == nil {
			t.Errorf("Expected error converting time %s to int %v", tc, out)
		}
	}
}

func TestConvertBoolToFloat(t *testing.T) {
	data := []struct {
		in  Bool
		out Float
	}{
		{true, 1.0},
		{false, 0.0},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, FloatID); err != nil {
			t.Errorf("Unexpected error converting bool to float: %v", err)
		} else if *(out.(*Float)) != tc.out {
			t.Errorf("Converting bool to float: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertInt32ToFloat(t *testing.T) {
	data := []struct {
		in  Int32
		out Float
	}{
		{3, 3.0},
		{-3, -3.0},
		{0, 0.0},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, FloatID); err != nil {
			t.Errorf("Unexpected error converting int to float: %v", err)
		} else if *(out.(*Float)) != tc.out {
			t.Errorf("Converting int to float: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertStringToFloat(t *testing.T) {
	data := []struct {
		in  String
		out Float
	}{
		{"1", 1},
		{"13816.251", 13816.251},
		{"-1221.12", -1221.12},
		{"-0.0", -0.0},
		{"1e10", 1e10},
		{"1e-2", 0.01},
	}
	for _, tc := range data {
		if out, err := Convert(&tc.in, FloatID); err != nil {
			t.Errorf("Unexpected error converting string to float: %v", err)
		} else if *(out.(*Float)) != tc.out {
			t.Errorf("Converting string to float: Expected %v, got %v", tc.out, out)
		}
	}

	errData := []String{
		"hello",
		"",
		"-3a.5",
		"1e400",
	}

	for _, tc := range errData {
		if out, err := Convert(&tc, FloatID); err == nil {
			t.Errorf("Expected error converting string %s to float %v", tc, out)
		}
	}
}

func TestConvertDateTimeToFloat(t *testing.T) {
	data := []struct {
		in  time.Time
		out Float
	}{
		{time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC), 1257894000},
		{time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC), -4410000},
		{time.Date(2009, time.November, 10, 23, 0, 0, 1000000, time.UTC), 1257894000.001},
		{time.Date(1969, time.November, 10, 23, 0, 0, 1000000, time.UTC), -4409999.999},
		{time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC), 2204578800},
		{time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC), -2150326800},
	}
	for _, tc := range data {
		if out, err := Convert(&Time{tc.in}, FloatID); err != nil {
			t.Errorf("Unexpected error converting time to int: %v", err)
		} else if *(out.(*Float)) != tc.out {
			t.Errorf("Converting time to int: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertBoolToTime(t *testing.T) {
	b := Bool(false)
	if _, err := Convert(&b, DateTimeID); err == nil {
		t.Errorf("Expected error converting bool to time")
	}
}

func TestConvertInt32ToTime(t *testing.T) {
	data := []struct {
		in  Int32
		out time.Time
	}{
		{1257894000, time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{-4410000, time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{0, time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range data {
		tout := Time{tc.out}
		if out, err := Convert(&tc.in, DateTimeID); err != nil {
			t.Errorf("Unexpected error converting time to int: %v", err)
		} else if *(out.(*Time)) != tout {
			t.Errorf("Converting time to int: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertFloatToTime(t *testing.T) {
	data := []struct {
		in  Float
		out time.Time
	}{
		{1257894000, time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{-4410000, time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{0, time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)},
		{2204578800, time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{-2150326800, time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC)},
		// For these two the conversion is not exact due to float64 rounding
		{1257894000.001, time.Date(2009, time.November, 10, 23, 0, 0, 999927, time.UTC)},
		{-4409999.999, time.Date(1969, time.November, 10, 23, 0, 0, 1000001, time.UTC)},
	}
	for _, tc := range data {
		tout := Time{tc.out}
		if out, err := Convert(&tc.in, DateTimeID); err != nil {
			t.Errorf("Unexpected error converting float to int: %v", err)
		} else if *(out.(*Time)) != tout {
			t.Errorf("Converting float to int: Expected %v, got %v", tc.out, out)
		}
	}
}


func TestConvertToString(t *testing.T) {
	f := Float(13816.251)
	i := Int32(-1221)
	b := Bool(true)
	data := []struct {
		in  Value
		out String
	}{
		{&Time{time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC)}, "2006-01-02T15:04:05Z"},
		{&f, "1.3816251E+04"},
		{&i, "-1221"},
		{&b, "true"},
	}
	for _, tc := range data {
		if out, err := Convert(tc.in, StringID); err != nil {
			t.Errorf("Unexpected error converting to string: %v", err)
		} else if *(out.(*String)) != tc.out {
			t.Errorf("Converting to string: Expected %v, got %v", tc.out, out)
		}
	}
}
*/
