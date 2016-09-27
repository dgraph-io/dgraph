/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
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
	"testing"
	"time"
)

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
		if out, err := BooleanType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting int to bool: %v", err)
		} else if out != tc.out {
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
		if out, err := BooleanType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting float to bool: %v", err)
		} else if out != tc.out {
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
		if out, err := BooleanType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting string to bool: %v", err)
		} else if out != tc.out {
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
		if out, err := BooleanType.Convert(tc); err == nil {
			t.Errorf("Expected error converting string %s to bool %v", tc, out)
		}
	}
}

func TestConvertDateTimeToBool(t *testing.T) {
	tm := time.Now()
	if _, err := BooleanType.Convert(tm); err == nil {
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
		if out, err := Int32Type.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting bool to int: %v", err)
		} else if out != tc.out {
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
		if out, err := Int32Type.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting float to int: %v", err)
		} else if out != tc.out {
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
		if out, err := Int32Type.Convert(Float(tc)); err == nil {
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
		if out, err := Int32Type.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting string to int: %v", err)
		} else if out != tc.out {
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
		if out, err := Int32Type.Convert(tc); err == nil {
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
		if out, err := Int32Type.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting time to int: %v", err)
		} else if out != tc.out {
			t.Errorf("Converting time to int: Expected %v, got %v", tc.out, out)
		}
	}

	errData := []time.Time{
		time.Date(2039, time.November, 10, 23, 0, 0, 0, time.UTC),
		time.Date(1901, time.November, 10, 23, 0, 0, 0, time.UTC),
	}

	for _, tc := range errData {
		if out, err := Int32Type.Convert(tc); err == nil {
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
		if out, err := FloatType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting bool to float: %v", err)
		} else if out != tc.out {
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
		if out, err := FloatType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting int to float: %v", err)
		} else if out != tc.out {
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
		if out, err := FloatType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting string to float: %v", err)
		} else if out != tc.out {
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
		if out, err := FloatType.Convert(tc); err == nil {
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
		if out, err := FloatType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting time to int: %v", err)
		} else if out != tc.out {
			t.Errorf("Converting time to int: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertBoolToTime(t *testing.T) {
	b := Bool(false)
	if _, err := DateTimeType.Convert(b); err == nil {
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
		if out, err := DateTimeType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting time to int: %v", err)
		} else if out != tc.out {
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
		if out, err := DateTimeType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting float to int: %v", err)
		} else if out != tc.out {
			t.Errorf("Converting float to int: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertToString(t *testing.T) {
	data := []struct {
		in  TypeValue
		out String
	}{
		{time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC), "2006-01-02T15:04:05Z"},
		{Float(13816.251), "1.3816251E+04"},
		{Int32(-1221), "-1221"},
		{Bool(true), "true"},
	}
	for _, tc := range data {
		if out, err := StringType.Convert(tc.in); err != nil {
			t.Errorf("Unexpected error converting to string: %v", err)
		} else if out != tc.out {
			t.Errorf("Converting to string: Expected %v, got %v", tc.out, out)
		}
	}
}
