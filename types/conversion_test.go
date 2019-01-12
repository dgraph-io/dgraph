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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSameConversionString(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{StringID, []byte("a")}, out: Val{StringID, "a"}},
		{in: Val{StringID, []byte("")}, out: Val{StringID, ""}},
		{in: Val{DefaultID, []byte("abc")}, out: Val{StringID, "abc"}},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, StringID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertToDefault(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{Val{StringID, []byte("a")}, Val{DefaultID, "a"}},
		{Val{StringID, []byte("")}, Val{DefaultID, ""}},
		{Val{DefaultID, []byte("abc")}, Val{DefaultID, "abc"}},
		{Val{BinaryID, []byte("2016")}, Val{DefaultID, "2016"}},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, DefaultID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertFromDefault(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{DefaultID, []byte("1")}, out: Val{IntID, int64(1)}},
		{in: Val{DefaultID, []byte("1.3")}, out: Val{FloatID, 1.3}},
		{in: Val{DefaultID, []byte("true")}, out: Val{BoolID, true}},
		{in: Val{DefaultID, []byte("2016")}, out: Val{BinaryID, []byte("2016")}},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, tc.out.Tid)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertStringToDateTime(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{StringID, []byte("2006-01-02T15:04:05")},
			out: Val{DateTimeID, time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC)}},
		{in: Val{StringID, []byte("2006-01-02")},
			out: Val{DateTimeID, time.Date(2006, 01, 02, 0, 0, 0, 0, time.UTC)}},
		{in: Val{StringID, []byte("2006-01")},
			out: Val{DateTimeID, time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC)}},
		{in: Val{StringID, []byte("2006")},
			out: Val{DateTimeID, time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC)}},
		// zero time value (0001-01-01 00:00:00 +0000 UTC)
		{in: Val{StringID, []byte("")},
			out: Val{DateTimeID, time.Time{}}},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertDateTimeToString(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		// time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC)
		{in: Val{DateTimeID, []byte{1, 0, 0, 0, 14, 187, 75, 55, 229, 0, 0, 0, 0, 255, 255}},
			out: Val{StringID, "2006-01-02T15:04:05Z"}},
		// time.Date(2006, 01, 02, 0, 0, 0, 0, time.UTC)
		{in: Val{DateTimeID, []byte{1, 0, 0, 0, 14, 187, 74, 100, 0, 0, 0, 0, 0, 255, 255}},
			out: Val{StringID, "2006-01-02T00:00:00Z"}},
		// time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC)
		{in: Val{DateTimeID, []byte{1, 0, 0, 0, 14, 187, 73, 18, 128, 0, 0, 0, 0, 255, 255}},
			out: Val{StringID, "2006-01-01T00:00:00Z"}},
		// zero time value (0001-01-01 00:00:00 +0000 UTC)
		{in: Val{DateTimeID, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255}},
			out: Val{StringID, ""}},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, StringID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertFromPassword(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{PasswordID, []byte("")}, out: Val{StringID, ""}},
		{in: Val{PasswordID, []byte("testing")}, out: Val{StringID, "testing"}},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, StringID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertToPassword(t *testing.T) {
	tests := []struct {
		in      Val
		out     Val
		failure string
	}{
		{in: Val{IntID, []byte{0, 0, 0, 0, 0, 0, 0, 1}},
			failure: "Cannot convert int to type password"},
		{in: Val{FloatID, []byte{0, 0, 0, 0, 0, 0, 0, 1}},
			failure: "Cannot convert float to type password"},
		{in: Val{BoolID, []byte{1}},
			failure: "Cannot convert bool to type password"},
		{in: Val{StringID, []byte("")},
			failure: "Password too short, i.e. should has at least 6 chars"},
		{in: Val{StringID, []byte("testing")}, out: Val{PasswordID, "$2a$10$"}},
		{in: Val{PasswordID, []byte("testing")}, out: Val{PasswordID, "testing"}},
		{in: Val{DefaultID, []byte("testing")}, out: Val{PasswordID, "$2a$10$"}},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, PasswordID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(out.Value.(string), tc.out.Value.(string)))
	}
}

func TestSameConversionFloat(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{Tid: FloatID, Value: []byte{7, 95, 152, 76, 21, 140, 11, 64}}, out: Val{Tid: FloatID, Value: 3.4434}},
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 8, 192}}, out: Val{Tid: FloatID, Value: -3.0}},
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 73, 64}}, out: Val{Tid: FloatID, Value: 0.5e2}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestSameConversionInt(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{Tid: IntID, Value: []byte{3, 0, 0, 0, 0, 0, 0, 0}}, out: Val{Tid: IntID, Value: int64(3)}},
		{in: Val{Tid: IntID, Value: []byte{253, 255, 255, 255, 255, 255, 255, 255}}, out: Val{Tid: IntID, Value: int64(-3)}},
		{in: Val{Tid: IntID, Value: []byte{15, 39, 0, 0, 0, 0, 0, 0}}, out: Val{Tid: IntID, Value: int64(9999)}},
		{in: Val{Tid: IntID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}}, out: Val{Tid: IntID, Value: int64(0)}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, IntID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
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
		{in: Val{Tid: StringID, Value: []byte("")}, fail: true},
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

func TestConvertFloatToInt(t *testing.T) {
	tests := []struct {
		in      Val
		out     Val
		failure string
	}{
		{in: Val{Tid: FloatID, Value: []byte{7, 95, 152, 76, 21, 140, 11, 64}}, out: Val{Tid: IntID, Value: int64(3)}},
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 12, 192}}, out: Val{Tid: IntID, Value: int64(-3)}},
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}}, out: Val{Tid: IntID, Value: int64(0)}},
		// 522638295213.3243
		{in: Val{Tid: FloatID, Value: []byte{193, 84, 43, 224, 234, 107, 94, 66}}, out: Val{Tid: IntID, Value: int64(522638295213)}},
		// -522638295213.3243
		{in: Val{Tid: FloatID, Value: []byte{193, 84, 43, 224, 234, 107, 94, 194}}, out: Val{Tid: IntID, Value: int64(-522638295213)}},
		// math.NaN
		{in: Val{Tid: FloatID, Value: []byte{1, 0, 0, 0, 0, 0, 248, 127}},
			failure: "Float out of int64 range"},
		// math.InF+
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 240, 127}},
			failure: "Float out of int64 range"},
		// math.InF-
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 240, 255}},
			failure: "Float out of int64 range"},
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

func TestConvertStringToInt(t *testing.T) {
	tests := []struct {
		in      Val
		out     Val
		failure string
	}{
		{in: Val{StringID, []byte("1")}, out: Val{IntID, int64(1)}},
		{in: Val{StringID, []byte("13816")}, out: Val{IntID, int64(13816)}},
		{in: Val{StringID, []byte("-1221")}, out: Val{IntID, int64(-1221)}},
		{in: Val{StringID, []byte("0")}, out: Val{IntID, int64(0)}},
		{in: Val{StringID, []byte("203716381366627")}, out: Val{IntID, int64(203716381366627)}},
		{in: Val{StringID, []byte("srfrog")},
			failure: `strconv.ParseInt: parsing "srfrog": invalid syntax`},
		{in: Val{StringID, []byte("")},
			failure: `strconv.ParseInt: parsing "": invalid syntax`},
		{in: Val{StringID, []byte("3.0")},
			failure: `strconv.ParseInt: parsing "3.0": invalid syntax`},
		{in: Val{StringID, []byte("-3a.5")},
			failure: `strconv.ParseInt: parsing "-3a.5": invalid syntax`},
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

func TestConvertDateTimeToInt(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		// time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 194, 139, 231, 112, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: IntID, Value: int64(1257894000)}},
		// time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 119, 78, 172, 112, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: IntID, Value: int64(-4410000)}},
		// time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 250, 249, 42, 240, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: IntID, Value: int64(2204578800)}},
		// time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 13, 247, 102, 148, 240, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: IntID, Value: int64(-2150326800)}},
		// zero time value (0001-01-01 00:00:00 +0000 UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: IntID, Value: int64(0)}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, IntID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertBoolToFloat(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{Tid: BoolID, Value: []byte{1}}, out: Val{Tid: FloatID, Value: float64(1.0)}},
		{in: Val{Tid: BoolID, Value: []byte{0}}, out: Val{Tid: FloatID, Value: float64(0.0)}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertIntToFloat(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{Tid: IntID, Value: []byte{3, 0, 0, 0, 0, 0, 0, 0}},
			out: Val{Tid: FloatID, Value: float64(3.0)}},
		{in: Val{Tid: IntID, Value: []byte{253, 255, 255, 255, 255, 255, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(-3.0)}},
		{in: Val{Tid: IntID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}},
			out: Val{Tid: FloatID, Value: float64(0.0)}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertStringToFloat(t *testing.T) {
	tests := []struct {
		in      Val
		out     Val
		failure string
	}{
		{in: Val{Tid: StringID, Value: []byte("1")}, out: Val{Tid: FloatID, Value: float64(1)}},
		{in: Val{Tid: StringID, Value: []byte("13816.251")}, out: Val{Tid: FloatID, Value: float64(13816.251)}},
		{in: Val{Tid: StringID, Value: []byte("-1221.12")}, out: Val{Tid: FloatID, Value: float64(-1221.12)}},
		{in: Val{Tid: StringID, Value: []byte("-0.0")}, out: Val{Tid: FloatID, Value: float64(-0.0)}},
		{in: Val{Tid: StringID, Value: []byte("1e10")}, out: Val{Tid: FloatID, Value: float64(1e10)}},
		{in: Val{Tid: StringID, Value: []byte("1e-2")}, out: Val{Tid: FloatID, Value: float64(0.01)}},
		{in: Val{Tid: StringID, Value: []byte("srfrog")},
			failure: `strconv.ParseFloat: parsing "srfrog": invalid syntax`},
		{in: Val{Tid: StringID, Value: []byte("")},
			failure: `strconv.ParseFloat: parsing "": invalid syntax`},
		{in: Val{Tid: StringID, Value: []byte("-3a.5")},
			failure: `strconv.ParseFloat: parsing "-3a.5": invalid syntax`},
		{in: Val{Tid: StringID, Value: []byte("1e400")},
			failure: `strconv.ParseFloat: parsing "1e400": value out of range`},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, FloatID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertDateTimeToFloat(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		// time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 194, 139, 231, 112, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(1257894000)}},
		// time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 119, 78, 172, 112, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(-4410000)}},
		// time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 250, 249, 42, 240, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(2204578800)}},
		// time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 13, 247, 102, 148, 240, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(-2150326800)}},
		// time.Date(2009, time.November, 10, 23, 0, 0, 1000000, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 194, 139, 231, 112, 0, 15, 66, 64, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(1257894000.001)}},
		// time.Date(1969, time.November, 10, 23, 0, 0, 1000000, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 119, 78, 172, 112, 0, 15, 66, 64, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(-4409999.999)}},
		// zero time value (0001-01-01 00:00:00 +0000 UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: FloatID, Value: float64(0)}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertIntToDateTime(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		// 1257894000
		{in: Val{Tid: IntID, Value: []byte{112, 240, 249, 74, 0, 0, 0, 0}},
			out: Val{Tid: DateTimeID, Value: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)}},
		// -4410000
		{in: Val{Tid: IntID, Value: []byte{112, 181, 188, 255, 255, 255, 255, 255}},
			out: Val{Tid: DateTimeID, Value: time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)}},
		// 0
		{in: Val{Tid: IntID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}},
			out: Val{Tid: DateTimeID, Value: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertFloatToDateTime(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		// 1257894000
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 28, 124, 190, 210, 65}},
			out: Val{Tid: DateTimeID, Value: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)}},
		// -4410000
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 164, 210, 80, 193}},
			out: Val{Tid: DateTimeID, Value: time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)}},
		// 0
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 0, 0, 0, 0, 0}},
			out: Val{Tid: DateTimeID, Value: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)}},
		// 2204578800
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 126, 230, 108, 224, 65}},
			out: Val{Tid: DateTimeID, Value: time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC)}},
		// -2150326800
		{in: Val{Tid: FloatID, Value: []byte{0, 0, 0, 66, 108, 5, 224, 193}},
			out: Val{Tid: DateTimeID, Value: time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC)}},
		// 1257894000.001
		{in: Val{Tid: FloatID, Value: []byte{98, 16, 0, 28, 124, 190, 210, 65}},
			out: Val{Tid: DateTimeID, Value: time.Date(2009, time.November, 10, 23, 0, 0, 999927, time.UTC)}},
		// -4409999.999
		{in: Val{Tid: FloatID, Value: []byte{178, 157, 239, 255, 163, 210, 80, 193}},
			out: Val{Tid: DateTimeID, Value: time.Date(1969, time.November, 10, 23, 0, 0, 1000001, time.UTC)}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertToString(t *testing.T) {
	tests := []struct {
		in  Val
		out Val
	}{
		{in: Val{Tid: FloatID, Value: []byte{166, 155, 196, 32, 32, 252, 202, 64}},
			out: Val{Tid: StringID, Value: "13816.251"}},
		{in: Val{Tid: IntID, Value: []byte{59, 251, 255, 255, 255, 255, 255, 255}},
			out: Val{Tid: StringID, Value: "-1221"}},
		{in: Val{Tid: BoolID, Value: []byte{1}},
			out: Val{Tid: StringID, Value: "true"}},
		{in: Val{Tid: StringID, Value: []byte("srfrog")},
			out: Val{Tid: StringID, Value: "srfrog"}},
		{in: Val{Tid: PasswordID, Value: []byte("password")},
			out: Val{Tid: StringID, Value: "password"}},
		// time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 14, 187, 75, 55, 229, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: StringID, Value: "2006-01-02T15:04:05Z"}},
		// zero time value (0001-01-01 00:00:00 +0000 UTC)
		{in: Val{Tid: DateTimeID, Value: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255}},
			out: Val{Tid: StringID, Value: ""}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, StringID)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}
