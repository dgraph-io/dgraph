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

func TestSameConversionDateTime(t *testing.T) {
	tests := []struct {
		in time.Time
	}{
		{in: time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC)},
		{in: time.Time{}},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BinaryID, Value: bs(tc.in)}, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: DateTimeID, Value: tc.in}, out)
	}
}

func TestConversionEdgeCases(t *testing.T) {
	tests := []struct {
		in, out Val
		failure string
	}{
		{in: Val{Tid: BinaryID},
			out:     Val{Tid: BinaryID},
			failure: "Invalid data to convert to binary"},

		// From BinaryID to X
		{in: Val{Tid: BinaryID, Value: []byte{}},
			out:     Val{Tid: IntID, Value: int64(0)},
			failure: "Invalid data for int64"},
		{in: Val{Tid: BinaryID, Value: []byte{}},
			out:     Val{Tid: FloatID, Value: int64(0)},
			failure: "Invalid data for float"},
		{in: Val{Tid: BinaryID, Value: []byte{}},
			out: Val{Tid: BoolID, Value: false}},
		{in: Val{Tid: BinaryID, Value: []byte{2}},
			out:     Val{Tid: BoolID, Value: false},
			failure: "Invalid value for bool"},
		{in: Val{Tid: BinaryID, Value: []byte{8}},
			out:     Val{Tid: DateTimeID, Value: time.Time{}},
			failure: "Time.UnmarshalBinary:"},
		{in: Val{Tid: BinaryID, Value: []byte{}},
			out:     Val{Tid: DateTimeID, Value: time.Time{}},
			failure: "Time.UnmarshalBinary:"},

		// From StringID|DefaultID to X
		{in: Val{Tid: StringID, Value: []byte{}},
			out:     Val{Tid: IntID, Value: int64(0)},
			failure: "strconv.ParseInt"},
		{in: Val{Tid: StringID, Value: []byte{}},
			out:     Val{Tid: FloatID, Value: float64(0)},
			failure: "strconv.ParseFloat"},
		{in: Val{Tid: StringID, Value: []byte{}},
			out:     Val{Tid: BoolID, Value: false},
			failure: "strconv.ParseBool"},
		{in: Val{Tid: StringID, Value: []byte{}},
			out:     Val{Tid: DateTimeID, Value: time.Time{}},
			failure: `parsing time "" as "2006": cannot parse "" as "2006"`},

		// From IntID to X
		{in: Val{Tid: IntID, Value: []byte{}},
			failure: "Invalid data for int64"},
		{in: Val{Tid: IntID, Value: bs(int64(0))},
			out: Val{Tid: DateTimeID, Value: time.Unix(0, 0).UTC()}},

		// From FloatID to X
		{in: Val{Tid: FloatID, Value: []byte{}},
			failure: "Invalid data for float"},
		{in: Val{Tid: FloatID, Value: bs(float64(0))},
			out: Val{Tid: DateTimeID, Value: time.Unix(0, 0).UTC()}},

		// From BoolID to X
		{in: Val{Tid: BoolID, Value: []byte{}},
			failure: "Invalid value for bool"},
		{in: Val{Tid: BoolID, Value: []byte{8}},
			failure: "Invalid value for bool"},

		// From DateTimeID to X
		{in: Val{Tid: DateTimeID, Value: []byte{}},
			out:     Val{Tid: DateTimeID, Value: time.Time{}},
			failure: "Time.UnmarshalBinary:"},
		{in: Val{Tid: DateTimeID, Value: bs(time.Time{})},
			out: Val{Tid: DateTimeID, Value: time.Time{}}},
		{in: Val{Tid: DateTimeID, Value: []byte{}},
			out:     Val{Tid: BinaryID, Value: []byte{}},
			failure: "Time.UnmarshalBinary"},
		{in: Val{Tid: DateTimeID, Value: bs(time.Time{})},
			out: Val{Tid: BinaryID, Value: bs(time.Time{})}},
		{in: Val{Tid: DateTimeID, Value: []byte{}},
			out:     Val{Tid: StringID, Value: ""},
			failure: "Time.UnmarshalBinary: no data"},
		{in: Val{Tid: DateTimeID, Value: bs(time.Time{})},
			out: Val{Tid: StringID, Value: "0001-01-01T00:00:00Z"}},
		{in: Val{Tid: DateTimeID, Value: []byte{}},
			out:     Val{Tid: DefaultID, Value: ""},
			failure: "Time.UnmarshalBinary: no data"},
		{in: Val{Tid: DateTimeID, Value: bs(time.Time{})},
			out: Val{Tid: DefaultID, Value: "0001-01-01T00:00:00Z"}},
		{in: Val{Tid: DateTimeID, Value: []byte{}},
			out:     Val{Tid: IntID, Value: int64(0)},
			failure: "Time.UnmarshalBinary: no data"},
		{in: Val{Tid: DateTimeID, Value: bs(time.Time{})},
			out: Val{Tid: IntID, Value: time.Time{}.Unix()}},
		{in: Val{Tid: DateTimeID, Value: []byte{}},
			out:     Val{Tid: FloatID, Value: float64(0)},
			failure: "Time.UnmarshalBinary: no data"},
		{in: Val{Tid: DateTimeID, Value: bs(time.Time{})},
			out: Val{Tid: FloatID,
				Value: float64(time.Time{}.UnixNano()) / float64(nanoSecondsInSec)}},
	}
	for _, tc := range tests {
		t.Logf("%s to %s != %v", tc.in.Tid.Name(), tc.out.Tid.Name(), tc.out.Value)
		out, err := Convert(tc.in, tc.out.Tid)
		if tc.failure != "" {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.failure)
		} else {
			require.NoError(t, err)
			require.EqualValues(t, tc.out, out)
		}
	}
}

func TestConvertToDefault(t *testing.T) {
	tests := []struct {
		in  Val
		out string
	}{
		{in: Val{StringID, []byte("a")}, out: "a"},
		{in: Val{StringID, []byte("")}, out: ""},
		{in: Val{DefaultID, []byte("abc")}, out: "abc"},
		{in: Val{BinaryID, []byte("2016")}, out: "2016"},
		{in: Val{IntID, bs(int64(3))}, out: "3"},
		{in: Val{FloatID, bs(float64(-3.5))}, out: "-3.5"},
		{in: Val{DateTimeID, bs(time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC))}, out: "2006-01-02T15:04:05Z"},
		{in: Val{DateTimeID, bs(time.Time{})}, out: "0001-01-01T00:00:00Z"},
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
		{in: "1", out: Val{IntID, int64(1)}},
		{in: "1.3", out: Val{FloatID, float64(1.3)}},
		{in: "true", out: Val{BoolID, true}},
		{in: "2016", out: Val{BinaryID, []byte("2016")}},
		{in: "", out: Val{BinaryID, []byte("")}},
		{in: "hello", out: Val{StringID, "hello"}},
		{in: "", out: Val{StringID, ""}},
		{in: "2016", out: Val{DateTimeID, time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)}},
	}

	for _, tc := range tests {
		out, err := Convert(Val{DefaultID, []byte(tc.in)}, tc.out.Tid)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertToBinary(t *testing.T) {
	tests := []struct {
		in  Val
		out []byte
	}{
		{in: Val{StringID, []byte("a")}, out: []byte("a")},
		{in: Val{StringID, []byte("")}, out: []byte("")},
		{in: Val{DefaultID, []byte("abc")}, out: []byte("abc")},
		{in: Val{BinaryID, []byte("2016")}, out: []byte("2016")},
		{in: Val{IntID, bs(int64(3))}, out: bs(int64(3))},
		{in: Val{FloatID, bs(float64(-3.5))}, out: bs(float64(-3.5))},
		{in: Val{DateTimeID, bs(time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC))}, out: bs(time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC))},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, BinaryID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: BinaryID, Value: tc.out}, out)
	}
}

func TestConvertFromBinary(t *testing.T) {
	tests := []struct {
		in  []byte
		out Val
	}{
		{in: bs(true), out: Val{BoolID, true}},
		{in: bs(false), out: Val{BoolID, false}},
		{in: []byte(""), out: Val{BoolID, false}},
		{in: nil, out: Val{BoolID, false}},
		{in: bs(int64(1)), out: Val{IntID, int64(1)}},
		{in: bs(float64(1.3)), out: Val{FloatID, float64(1.3)}},
		{in: []byte("2016"), out: Val{BinaryID, []byte("2016")}},
		{in: []byte(""), out: Val{BinaryID, []byte("")}},
		{in: []byte("hello"), out: Val{StringID, "hello"}},
		{in: []byte(""), out: Val{StringID, ""}},
		{in: bs(time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)), out: Val{DateTimeID, time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)}},
		{in: bs(time.Time{}), out: Val{DateTimeID, time.Time{}}},
	}

	for _, tc := range tests {
		out, err := Convert(Val{BinaryID, tc.in}, tc.out.Tid)
		require.NoError(t, err)
		require.EqualValues(t, tc.out, out)
	}
}

func TestConvertStringToDateTime(t *testing.T) {
	tests := []struct {
		in  string
		out time.Time
	}{
		{in: "2006-01-02T15:04:05", out: time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC)},
		{in: "2006-01-02", out: time.Date(2006, 01, 02, 0, 0, 0, 0, time.UTC)},
		{in: "2006-01", out: time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC)},
		{in: "2006", out: time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: StringID, Value: []byte(tc.in)}, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: DateTimeID, Value: tc.out}, out)
	}
}

func TestConvertDateTimeToString(t *testing.T) {
	tests := []struct {
		in  time.Time
		out string
	}{
		{in: time.Date(2006, 01, 02, 15, 04, 05, 0, time.UTC), out: "2006-01-02T15:04:05Z"},
		{in: time.Date(2006, 01, 02, 0, 0, 0, 0, time.UTC), out: "2006-01-02T00:00:00Z"},
		{in: time.Date(2006, 01, 01, 0, 0, 0, 0, time.UTC), out: "2006-01-01T00:00:00Z"},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: DateTimeID, Value: bs(tc.in)}, StringID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: StringID, Value: tc.out}, out)
	}
}

func TestConvertFromPassword(t *testing.T) {
	tests := []struct {
		in      string
		out     Val
		failure string
	}{
		{in: "", out: Val{StringID, ""}},
		{in: "password", out: Val{PasswordID, "password"}},
		{in: "password", out: Val{StringID, "password"}},
		{in: "password", out: Val{BinaryID, []byte("password")}},
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
		{in: Val{StringID, []byte("testing")}, out: "$2a$10$"},
		{in: Val{PasswordID, []byte("testing")}, out: "testing"},
		{in: Val{DefaultID, []byte("testing")}, out: "$2a$10$"},
		{
			in:      Val{StringID, []byte("")},
			failure: `Password too short, i.e. should have at least 6 chars`,
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

func TestSameConversionBool(t *testing.T) {
	tests := []struct {
		in bool
	}{
		{in: true},
		{in: false},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BinaryID, Value: bs(tc.in)}, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: BoolID, Value: tc.in}, out)
	}
}

func TestConvertIntToBool(t *testing.T) {
	tests := []struct {
		in  int64
		out bool
	}{
		{in: int64(3), out: true},
		{in: int64(-3), out: true},
		{in: int64(0), out: false},
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
		{in: float64(3.0), out: true},
		{in: float64(-3.5), out: true},
		{in: float64(0), out: false},
		{in: math.NaN(), out: true},
		{in: math.Inf(0), out: true},
		{in: math.Inf(-1), out: true},
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
		{in: "1", out: true},
		{in: "true", out: true},
		{in: "True", out: true},
		{in: "T", out: true},
		{in: "F", out: false},
		{in: "0", out: false},
		{in: "false", out: false},
		{in: "False", out: false},
		{
			in:      "",
			failure: `strconv.ParseBool: parsing "": invalid syntax`,
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
		{in: true, out: int64(1)},
		{in: false, out: int64(0)},
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
		{in: Val{Tid: StringID, Value: []byte("true")}},
		{in: Val{Tid: DefaultID, Value: []byte("true")}},
		{in: Val{Tid: IntID, Value: bs(int64(1))}},
		{in: Val{Tid: IntID, Value: bs(int64(-1))}},
		{in: Val{Tid: FloatID, Value: bs(float64(1.0))}},
		{in: Val{Tid: FloatID, Value: bs(float64(-1.0))}},
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
		{in: Val{Tid: StringID, Value: []byte("false")}},
		{in: Val{Tid: DefaultID, Value: []byte("false")}},
		{in: Val{Tid: IntID, Value: bs(int64(0))}},
		{in: Val{Tid: FloatID, Value: bs(float64(0.0))}},
		{in: Val{Tid: BinaryID, Value: []byte("")}},
	}
	for _, tc := range tests {
		out, err := Convert(tc.in, BoolID)
		require.NoError(t, err)
		require.EqualValues(t, false, out.Value)
	}
}

func TestSameConversionFloat(t *testing.T) {
	tests := []struct {
		in float64
	}{
		{in: float64(3.4434)},
		{in: float64(-3.0)},
		{in: float64(0.5e2)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BinaryID, Value: bs(tc.in)}, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: FloatID, Value: tc.in}, out)
	}
}

func TestSameConversionInt(t *testing.T) {
	tests := []struct {
		in int64
	}{
		{in: int64(3)},
		{in: int64(-3)},
		{in: int64(9999)},
		{in: int64(0)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BinaryID, Value: bs(tc.in)}, IntID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: IntID, Value: tc.in}, out)
	}
}

func TestConvertBoolToFloat(t *testing.T) {
	tests := []struct {
		in  bool
		out float64
	}{
		{in: true, out: float64(1.0)},
		{in: false, out: float64(0.0)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BoolID, Value: bs(tc.in)}, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: FloatID, Value: tc.out}, out)
	}
}

func TestConvertBoolToString(t *testing.T) {
	tests := []struct {
		in  bool
		out string
	}{
		{in: true, out: "true"},
		{in: false, out: "false"},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: BoolID, Value: bs(tc.in)}, StringID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: StringID, Value: tc.out}, out)
	}
}

func TestConvertIntToFloat(t *testing.T) {
	tests := []struct {
		in  int64
		out float64
	}{
		{in: int64(3), out: float64(3.0)},
		{in: int64(-3), out: float64(-3.0)},
		{in: int64(0), out: float64(0.0)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: IntID, Value: bs(tc.in)}, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: FloatID, Value: tc.out}, out)
	}
}

func TestConvertFloatToInt(t *testing.T) {
	tests := []struct {
		in      float64
		out     int64
		failure string
	}{
		{in: float64(3), out: int64(3)},
		{in: float64(-3.0), out: int64(-3)},
		{in: float64(0), out: int64(0)},
		{in: float64(522638295213.3243), out: int64(522638295213)},
		{in: float64(-522638295213.3243), out: int64(-522638295213)},
		{
			in:      math.NaN(),
			failure: "Float out of int64 range",
		},
		{
			in:      math.Inf(0),
			failure: "Float out of int64 range",
		},
		{
			in:      math.Inf(-1),
			failure: "Float out of int64 range",
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: FloatID, Value: bs(tc.in)}, IntID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: IntID, Value: tc.out}, out)
	}
}

func TestConvertStringToInt(t *testing.T) {
	tests := []struct {
		in      string
		out     int64
		failure string
	}{
		{in: "1", out: int64(1)},
		{in: "13816", out: int64(13816)},
		{in: "-1221", out: int64(-1221)},
		{in: "0", out: int64(0)},
		{in: "203716381366627", out: int64(203716381366627)},
		{
			in:      "",
			failure: `strconv.ParseInt: parsing "": invalid syntax`,
		},
		{
			in:      "srfrog",
			failure: `strconv.ParseInt: parsing "srfrog": invalid syntax`,
		},
		{
			in:      "3.0",
			failure: `strconv.ParseInt: parsing "3.0": invalid syntax`,
		},
		{
			in:      "-3a.5",
			failure: `strconv.ParseInt: parsing "-3a.5": invalid syntax`,
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: StringID, Value: []byte(tc.in)}, IntID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: IntID, Value: tc.out}, out)
	}
}

func TestConvertStringToFloat(t *testing.T) {
	tests := []struct {
		in      string
		out     float64
		failure string
	}{
		{in: "1", out: float64(1)},
		{in: "13816.251", out: float64(13816.251)},
		{in: "-1221.12", out: float64(-1221.12)},
		{in: "-0.0", out: float64(-0.0)},
		{in: "1e10", out: float64(1e10)},
		{in: "1e-2", out: float64(0.01)},
		{
			in:      "",
			failure: `strconv.ParseFloat: parsing "": invalid syntax`,
		},
		{
			in:      "srfrog",
			failure: `strconv.ParseFloat: parsing "srfrog": invalid syntax`,
		},
		{
			in:      "-3a.5",
			failure: `strconv.ParseFloat: parsing "-3a.5": invalid syntax`,
		},
		{
			in:      "1e400",
			failure: `strconv.ParseFloat: parsing "1e400": value out of range`,
		},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: StringID, Value: []byte(tc.in)}, FloatID)
		if tc.failure != "" {
			require.Error(t, err)
			require.EqualError(t, err, tc.failure)
			continue
		}
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: FloatID, Value: tc.out}, out)
	}
}

func TestConvertFloatToDateTime(t *testing.T) {
	tests := []struct {
		in  float64
		out time.Time
	}{
		{in: float64(1257894000), out: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{in: float64(-4410000), out: time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{in: float64(2204578800), out: time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{in: float64(-2150326800), out: time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{in: float64(1257894000.001), out: time.Date(2009, time.November, 10, 23, 0, 0, 999927, time.UTC)},
		{in: float64(-4409999.999), out: time.Date(1969, time.November, 10, 23, 0, 0, 1000001, time.UTC)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: FloatID, Value: bs(tc.in)}, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: DateTimeID, Value: tc.out}, out)
	}
}

func TestConvertDateTimeToFloat(t *testing.T) {
	tests := []struct {
		in  time.Time
		out float64
	}{
		{in: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC), out: float64(1257894000)},
		{in: time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC), out: float64(-4410000)},
		{in: time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC), out: float64(2204578800)},
		{in: time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC), out: float64(-2150326800)},
		{in: time.Date(2009, time.November, 10, 23, 0, 0, 1000000, time.UTC), out: float64(1257894000.001)},
		{in: time.Date(1969, time.November, 10, 23, 0, 0, 1000000, time.UTC), out: float64(-4409999.999)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: DateTimeID, Value: bs(tc.in)}, FloatID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: FloatID, Value: tc.out}, out)
	}
}

func TestConvertIntToDateTime(t *testing.T) {
	tests := []struct {
		in  int64
		out time.Time
	}{
		{in: int64(1257894000), out: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)},
		{in: int64(-4410000), out: time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: IntID, Value: bs(tc.in)}, DateTimeID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: DateTimeID, Value: tc.out}, out)
	}
}

func TestConvertDateTimeToInt(t *testing.T) {
	tests := []struct {
		in  time.Time
		out int64
	}{
		{in: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC), out: int64(1257894000)},
		{in: time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC), out: int64(-4410000)},
		{in: time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC), out: int64(2204578800)},
		{in: time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC), out: int64(-2150326800)},
	}

	for _, tc := range tests {
		out, err := Convert(Val{Tid: DateTimeID, Value: bs(tc.in)}, IntID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: IntID, Value: tc.out}, out)
	}
}

func TestConvertToString(t *testing.T) {
	tests := []struct {
		in  Val
		out string
	}{
		{in: Val{Tid: FloatID, Value: bs(float64(13816.251))}, out: "13816.251"},
		{in: Val{Tid: IntID, Value: bs(int64(-1221))}, out: "-1221"},
		{in: Val{Tid: BoolID, Value: bs(true)}, out: "true"},
		{in: Val{Tid: StringID, Value: []byte("srfrog")}, out: "srfrog"},
		{in: Val{Tid: PasswordID, Value: []byte("password")}, out: "password"},
		{in: Val{Tid: DateTimeID, Value: bs(time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC))}, out: "2006-01-02T15:04:05Z"},
	}

	for _, tc := range tests {
		out, err := Convert(tc.in, StringID)
		require.NoError(t, err)
		require.EqualValues(t, Val{Tid: StringID, Value: tc.out}, out)
	}
}
