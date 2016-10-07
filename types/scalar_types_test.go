/*
 * Copyright 2016 DGraph Labs, Inc.
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
	"testing"
	"time"
)

func testBinary(val Value, un Unmarshaler, t *testing.T) {
	bytes, err := val.MarshalBinary()
	if err != nil {
		t.Error(err)
	}
	val2, err := un.FromBinary(bytes)
	if err != nil {
		t.Error(err)
	}
	if val != val2 {
		t.Errorf("Expected marshal/unmarshal binary to parse to the same value: %v == %v", val2, val)
	}
}

func testText(val Value, un Unmarshaler, t *testing.T) {
	bytes, err := val.MarshalText()
	if err != nil {
		t.Error(err)
	}
	val2, err := un.FromText(bytes)
	if err != nil {
		t.Error(err)
	}
	if val != val2 {
		t.Errorf("Expected marshal/unmarshal text to parse to the same value: %v == %v", val2, val)
	}
}

func TestInt32(t *testing.T) {
	array := []Int32{1532, -122911, 0}
	for _, v := range array {
		testBinary(v, Int32Type.Unmarshaler, t)
		testText(v, Int32Type.Unmarshaler, t)
	}
}

func TestParseInt(t *testing.T) {
	array := []string{"abc", "5a", "2.5"}
	for _, v := range array {
		if _, err := Int32Type.Unmarshaler.FromText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"52", "0", "-7"}
	for _, v := range array {
		if _, err := Int32Type.Unmarshaler.FromText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}

	data := []byte{0, 3, 1}
	if _, err := Int32Type.Unmarshaler.FromBinary(data); err == nil {
		t.Errorf("Expected error parsing %v", data)
	}
}

func TestFloat(t *testing.T) {
	array := []Float{15.32, -12.2911, 42, 0.0}
	for _, v := range array {
		testBinary(v, FloatType.Unmarshaler, t)
		testText(v, FloatType.Unmarshaler, t)
	}
}

func TestParseFloat(t *testing.T) {
	array := []string{"abc", "5a", "2.5a"}
	for _, v := range array {
		if _, err := FloatType.Unmarshaler.FromText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"52", "0", "-7", "-2.5", "1e4"}
	for _, v := range array {
		if _, err := FloatType.Unmarshaler.FromText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
	data := []byte{0, 3, 1, 5, 1}
	if _, err := FloatType.Unmarshaler.FromBinary(data); err == nil {
		t.Errorf("Expected error parsing %v", data)
	}

}

func TestString(t *testing.T) {
	array := []String{"abc", ""}
	for _, v := range array {
		testBinary(v, StringType.Unmarshaler, t)
		testText(v, StringType.Unmarshaler, t)
	}
}

func TestBool(t *testing.T) {
	array := []Bool{true, false}
	for _, v := range array {
		testBinary(v, BooleanType.Unmarshaler, t)
		testText(v, BooleanType.Unmarshaler, t)
	}
}

func TestParseBool(t *testing.T) {
	array := []string{"abc", "5", ""}
	for _, v := range array {
		if _, err := BooleanType.Unmarshaler.FromText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"true", "F", "0", "False"}
	for _, v := range array {
		if _, err := BooleanType.Unmarshaler.FromText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
}

func TestParseTime(t *testing.T) {
	array := []string{"2014-10-28T04:00:10", "2002-05-30T09:30:10Z",
		"2002-05-30T09:30:10.5", "2002-05-30T09:30:10-06:00"}
	for _, v := range array {
		if _, err := DateTimeType.Unmarshaler.FromText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
}

func TestTime(t *testing.T) {
	v := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	testBinary(Time{v}, DateTimeType.Unmarshaler, t)
	testText(Time{v}, DateTimeType.Unmarshaler, t)
}
