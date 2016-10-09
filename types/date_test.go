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
	//	"math"
	"testing"
	"time"
)

func TestConvertDateToBool(t *testing.T) {
	dt := createDate(2007, 1, 31)
	if _, err := booleanType.Convert(&dt); err == nil {
		t.Errorf("Expected error converting date to bool")
	}
}

func TestConvertDateToInt32(t *testing.T) {
	data := []struct {
		in  Date
		out Int32
	}{
		{createDate(2009, time.November, 10), 1257811200},
		{createDate(1969, time.November, 10), -4492800},
	}
	for _, tc := range data {
		if out, err := int32Type.Convert(&tc.in); err != nil {
			t.Errorf("Unexpected error converting date to int: %v", err)
		} else if *(out.(*Int32)) != tc.out {
			t.Errorf("Converting time to int: Expected %v, got %v", tc.out, out)
		}
	}

	errData := []Date{
		createDate(2039, time.November, 10),
		createDate(1901, time.November, 10),
	}

	for _, tc := range errData {
		if out, err := int32Type.Convert(&tc); err == nil {
			t.Errorf("Expected error converting date %s to int %v", tc, out)
		}
	}
}

func TestConvertDateToFloat(t *testing.T) {
	data := []struct {
		in  Date
		out Float
	}{
		{createDate(2009, time.November, 10), 1257811200},
		{createDate(1969, time.November, 10), -4492800},
		{createDate(2039, time.November, 10), 2204496000},
		{createDate(1901, time.November, 10), -2150409600},
	}
	for _, tc := range data {
		if out, err := floatType.Convert(&tc.in); err != nil {
			t.Errorf("Unexpected error converting date to float: %v", err)
		} else if *(out.(*Float)) != tc.out {
			t.Errorf("Converting date to float: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertDateToTime(t *testing.T) {
	data := []struct {
		in  Date
		out time.Time
	}{
		{createDate(2009, time.November, 10),
			time.Date(2009, time.November, 10, 0, 0, 0, 0, time.UTC)},
		{createDate(1969, time.November, 10),
			time.Date(1969, time.November, 10, 0, 0, 0, 0, time.UTC)},
		{createDate(2039, time.November, 10),
			time.Date(2039, time.November, 10, 0, 0, 0, 0, time.UTC)},
		{createDate(1901, time.November, 10),
			time.Date(1901, time.November, 10, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range data {
		tout := Time{tc.out}
		if out, err := dateTimeType.Convert(&tc.in); err != nil {
			t.Errorf("Unexpected error converting date to time: %v", err)
		} else if *(out.(*Time)) != tout {
			t.Errorf("Converting date to time: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertBoolToDate(t *testing.T) {
	b := Bool(false)
	if _, err := dateType.Convert(&b); err == nil {
		t.Errorf("Expected error converting bool to date")
	}
}

func TestConvertInt32ToDate(t *testing.T) {
	data := []struct {
		in  Int32
		out Date
	}{
		{1257811200, createDate(2009, time.November, 10)},
		{1257894000, createDate(2009, time.November, 10)}, //truncation
		{-4492800, createDate(1969, time.November, 10)},
		{0, createDate(1970, time.January, 1)},
	}
	for _, tc := range data {
		if out, err := dateType.Convert(&tc.in); err != nil {
			t.Errorf("Unexpected error converting int to date: %v", err)
		} else if *(out.(*Date)) != tc.out {
			t.Errorf("Converting int to date: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertFloatToDate(t *testing.T) {
	data := []struct {
		in  Float
		out Date
	}{
		{1257811200, createDate(2009, time.November, 10)},
		{1257894000.001, createDate(2009, time.November, 10)}, //truncation
		{-4492800, createDate(1969, time.November, 10)},
		{0, createDate(1970, time.January, 1)},
		{2204578800.5, createDate(2039, time.November, 10)},
		{-2150326800.12, createDate(1901, time.November, 10)},
	}
	for _, tc := range data {
		if out, err := dateType.Convert(&tc.in); err != nil {
			t.Errorf("Unexpected error converting float to date: %v", err)
		} else if *(out.(*Date)) != tc.out {
			t.Errorf("Converting float to date: Expected %v, got %v", tc.out, out)
		}
	}
}

func TestConvertDateTimeToDate(t *testing.T) {
	data := []struct {
		in  time.Time
		out Date
	}{
		{time.Date(2009, time.November, 10, 0, 0, 0, 0, time.UTC),
			createDate(2009, time.November, 10)},
		{time.Date(2009, time.November, 10, 21, 2, 50, 1000000, time.UTC),
			createDate(2009, time.November, 10)}, // truncation
		{time.Date(1969, time.November, 10, 23, 0, 0, 0, time.UTC),
			createDate(1969, time.November, 10)},
	}
	for _, tc := range data {
		if out, err := dateType.Convert(&Time{tc.in}); err != nil {
			t.Errorf("Unexpected error converting time to date: %v", err)
		} else if *(out.(*Date)) != tc.out {
			t.Errorf("Converting time to date: Expected %v, got %v", tc.out, out)
		}
	}
}
