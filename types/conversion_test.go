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
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func bs(v interface{}) []byte {
	switch x := v.(type) {
	case bool:
		if x {
			return []byte{1}
		}
		return []byte{0}
	case int64:
		var bs [8]byte
		binary.LittleEndian.PutUint64(bs[:], uint64(x))
		return bs[:]
	case float64:
		var bs [8]byte
		binary.LittleEndian.PutUint64(bs[:], math.Float64bits(x))
		return bs[:]
	case time.Time:
		bs, err := x.MarshalBinary()
		if err == nil {
			return bs
		}
	}
	return nil
}

func TestSameConversionString(t *testing.T) {
	tests := []struct {
		in string
	}{
		{in: "a"},
		{in: ""},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BinaryID, Value: []byte(tc.in)}, StringID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: StringID, Value: tc.in}, out)
	}
}

func TestConvertToDefault(t *testing.T) {
	tests := []struct {
		in  Val
		out string
	}{
		{
			in:  Val{StringID, []byte("a")},
			out: "a",
		},
		{
			in:  Val{StringID, []byte("")},
			out: "",
		},
		{
			in:  Val{DefaultID, []byte("abc")},
			out: "abc",
		},
		{
			in:  Val{BinaryID, []byte("2016")},
			out: "2016",
		},
		{
			in:  Val{IntID, bs(int64(3))},
			out: "3",
		},
		{
			in:  Val{FloatID, bs(float64(-3.5))},
			out: "-3.5",
		},
		{
			in:  Val{DateTimeID, bs(time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC))},
			out: "2006-01-02T15:04:05Z",
		},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, DefaultID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: DefaultID, Value: tc.out}, out)
	}
}

func TestConvertFromDefault(t *testing.T) {
	tests := []struct {
		in  string
		out Val
	}{
		{
			in:  "1",
			out: Val{IntID, int64(1)},
		},
		{
			in:  "1.3",
			out: Val{FloatID, float64(1.3)},
		},
		{
			in:  "true",
			out: Val{BoolID, true},
		},
		{
			in:  "2016",
			out: Val{BinaryID, []byte("2016")},
		},
		{
			in:  "",
			out: Val{BinaryID, []byte("")},
		},
		{
			in:  "hello",
			out: Val{StringID, "hello"},
		},
		{
			in:  "",
			out: Val{StringID, ""},
		},
		{
			in:  "2016",
			out: Val{DateTimeID, time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{DefaultID, []byte(tc.in)}, tc.out.Tid)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertStringToDateTime(t *testing.T) {
	tests := []struct {
		in  string
		out time.Time
	}{
		{
			in:  "2006-01-02T15:04:05",
			out: time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC),
		},
		{
			in:  "2006-01-02",
			out: time.Date(2006, 01, 02, 0, 0, 0, 0, time.UTC),
		},
		{
			in:  "2006-01",
			out: time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC),
		},
		{
			in:  "2006",
			out: time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: StringID, Value: []byte(tc.in)}, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: DateTimeID, Value: tc.out}, out)
	}
}

func TestConvertFromPassword(t *testing.T) {
	tests := []struct {
		in      string
		out     Val
		failure string
	}{
		{
			in:  "",
			out: Val{StringID, ""},
		},
		{
			in:  "password",
			out: Val{PasswordID, "password"},
		},
		{
			in:  "password",
			out: Val{StringID, "password"},
		},
		{
			in:  "password",
			out: Val{BinaryID, []byte("password")},
		},
		{
			in:      "password",
			failure: `Cannot convert password to type default`,
		},
		{
			in: "password", out: Val{IntID, bs(int64(1))},
			failure: `Cannot convert password to type int`,
		},
		{
			in: "password", out: Val{FloatID, bs(float64(1.0))},
			failure: `Cannot convert password to type float`,
		},
		{
			in: "password", out: Val{BoolID, bs(true)},
			failure: `Cannot convert password to type bool`,
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: PasswordID, Value: []byte(tc.in)}, tc.out.Tid)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertToPassword(t *testing.T) {
	tests := []struct {
		in      Val
		out     string
		failure string
	}{
		{
			in:  Val{StringID, []byte("testing")},
			out: "$2a$10$",
		},
		{
			in:  Val{PasswordID, []byte("testing")},
			out: "testing",
		},
		{
			in:  Val{DefaultID, []byte("testing")},
			out: "$2a$10$",
		},
		{
			in:      Val{StringID, []byte("")},
			failure: `Password too short, i.e. should has at least 6 chars`,
		},
		{
			in:      Val{IntID, bs(int64(1))},
			failure: `Cannot convert int to type password`,
		},
		{
			in:      Val{FloatID, bs(float64(1.0))},
			failure: `Cannot convert float to type password`,
		},
		{
			in:      Val{BoolID, bs(true)},
			failure: `Cannot convert bool to type password`,
		},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, PasswordID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		if tc.in.Tid == PasswordID {
			require.EqualValues(t, Val{Tid: PasswordID, Value: tc.out}, out)
			continue
		}
		require.True(t, out.Tid == PasswordID)
		require.NoError(t, VerifyPassword(string(tc.in.Value.([]byte)), out.Value.(string)))
	}
}

func TestConvertIntToBool(t *testing.T) {
	tests := []struct {
		in  int64
		out bool
	}{
		{
			in:  int64(3),
			out: true,
		},
		{
			in:  int64(-3),
			out: true,
		},
		{
			in:  int64(0),
			out: false,
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: IntID, Value: bs(tc.in)}, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: BoolID, Value: tc.out}, out)
	}
}

func TestConvertFloatToBool(t *testing.T) {
	tests := []struct {
		in  float64
		out bool
	}{
		{
			in:  float64(3.0),
			out: true,
		},
		{
			in:  float64(-3.5),
			out: true,
		},
		{
			in:  float64(0),
			out: false,
		},
		{
			in:  math.NaN(),
			out: true,
		},
		{
			in:  math.Inf(0),
			out: true,
		},
		{
			in:  math.Inf(-1),
			out: true,
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: FloatID, Value: bs(tc.in)}, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: BoolID, Value: tc.out}, out)
	}
}

func TestConvertStringToBool(t *testing.T) {
	tests := []struct {
		in      string
		out     bool
		failure string
	}{
		{
			in:  "1",
			out: true,
		},
		{
			in:  "true",
			out: true,
		},
		{
			in:  "True",
			out: true,
		},
		{
			in:  "T",
			out: true,
		},
		{
			in:  "F",
			out: false,
		},
		{
			in:  "0",
			out: false,
		},
		{
			in:  "false",
			out: false,
		},
		{
			in:  "False",
			out: false,
		},
		{
			in:  "",
			out: false,
		},
		{
			in:      "srfrog",
			failure: `strconv.ParseBool: parsing "srfrog": invalid syntax`,
		},
		{
			in:      "3",
			failure: `strconv.ParseBool: parsing "3": invalid syntax`,
		},
		{
			in:      "-3",
			failure: `strconv.ParseBool: parsing "-3": invalid syntax`,
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: StringID, Value: []byte(tc.in)}, BoolID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: BoolID, Value: tc.out}, out)
	}
}

func TestConvertDateTimeToBool(t *testing.T) {
	_, err := Convert(Val{Tid: DateTimeID, Value: bs(time.Now())}, BoolID)
	require.Error(t, err)
	require.EqualError(t, err, "Cannot convert datetime to type bool")
}

func TestConvertBoolToDateTime(t *testing.T) {
	_, err := Convert(Val{Tid: BoolID, Value: bs(true)}, DateTimeID)
	require.Error(t, err)
	require.EqualError(t, err, "Cannot convert bool to type datetime")
}

func TestConvertBoolToInt(t *testing.T) {
	tests := []struct {
		in  bool
		out int64
	}{
		{
			in:  true,
			out: int64(1),
		},
		{
			in:  false,
			out: int64(0),
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BoolID, Value: bs(tc.in)}, IntID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: IntID, Value: tc.out}, out)
	}
}

func TestTruthy(t *testing.T) {
	tests := []struct {
		in Val
	}{
		{
			in: Val{Tid: StringID, Value: []byte("true")},
		},
		{
			in: Val{Tid: DefaultID, Value: []byte("true")},
		},
		{
			in: Val{Tid: IntID, Value: bs(int64(1))},
		},
		{
			in: Val{Tid: IntID, Value: bs(int64(-1))},
		},
		{
			in: Val{Tid: FloatID, Value: bs(float64(1.0))},
		},
		{
			in: Val{Tid: FloatID, Value: bs(float64(-1.0))},
		},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, true, out.Value)
	}
}

func TestFalsy(t *testing.T) {
	tests := []struct {
		in Val
	}{
		{
			in: Val{Tid: StringID, Value: []byte("false")},
		},
		{
			in: Val{Tid: StringID, Value: []byte("")},
		},
		{
			in: Val{Tid: DefaultID, Value: []byte("false")},
		},
		{
			in: Val{Tid: DefaultID, Value: []byte("")},
		},
		{
			in: Val{Tid: IntID, Value: bs(int64(0))},
		},
		{
			in: Val{Tid: FloatID, Value: bs(float64(0.0))},
		},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, false, out.Value)
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
