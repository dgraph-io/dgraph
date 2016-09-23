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

func testBinary(val TypeValue, un Unmarshaler, t *testing.T) {
	bytes, err := val.MarshalBinary()
	if err != nil {
		t.Error(err)
	}
	val2, err := un.UnmarshalBinary(bytes)
	if err != nil {
		t.Error(err)
	}
	if val != val2 {
		t.Errorf("Expected marshal/unmarshal binary to parse to the same value: %v == %v", val2, val)
	}
}

func testText(val TypeValue, un Unmarshaler, t *testing.T) {
	bytes, err := val.MarshalText()
	if err != nil {
		t.Error(err)
	}
	val2, err := un.UnmarshalText(bytes)
	if err != nil {
		t.Error(err)
	}
	if val != val2 {
		t.Errorf("Expected marshal/unmarshal text to parse to the same value: %v == %v", val2, val)
	}
}

func TestInt32(t *testing.T) {
	array := []int32{1532, -122911, 0}
	for _, v := range array {
		testBinary(Int32Type(v), Int32.Unmarshaler, t)
		testText(Int32Type(v), Int32.Unmarshaler, t)
	}
}

func TestParseInt(t *testing.T) {
	array := []string{"abc", "5a", "2.5"}
	for _, v := range array {
		if _, err := Int32.Unmarshaler.UnmarshalText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"52", "0", "-7"}
	for _, v := range array {
		if _, err := Int32.Unmarshaler.UnmarshalText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}

	data := []byte{0, 3, 1}
	if _, err := Int32.Unmarshaler.UnmarshalBinary(data); err == nil {
		t.Errorf("Expected error parsing %v", data)
	}
}

func TestFloat(t *testing.T) {
	array := []float64{15.32, -12.2911, 42, 0.0}
	for _, v := range array {
		testBinary(FloatType(v), Float.Unmarshaler, t)
		testText(FloatType(v), Float.Unmarshaler, t)
	}
}

func TestParseFloat(t *testing.T) {
	array := []string{"abc", "5a", "2.5a"}
	for _, v := range array {
		if _, err := Float.Unmarshaler.UnmarshalText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"52", "0", "-7", "-2.5", "1e4"}
	for _, v := range array {
		if _, err := Float.Unmarshaler.UnmarshalText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
	data := []byte{0, 3, 1, 5, 1}
	if _, err := Float.Unmarshaler.UnmarshalBinary(data); err == nil {
		t.Errorf("Expected error parsing %v", data)
	}

}

func TestString(t *testing.T) {
	array := []string{"abc", ""}
	for _, v := range array {
		testBinary(StringType(v), String.Unmarshaler, t)
		testText(StringType(v), String.Unmarshaler, t)
	}
}

func TestBool(t *testing.T) {
	array := []bool{true, false}
	for _, v := range array {
		testBinary(BoolType(v), Boolean.Unmarshaler, t)
		testText(BoolType(v), Boolean.Unmarshaler, t)
	}
}

func TestParseBool(t *testing.T) {
	array := []string{"abc", "5", ""}
	for _, v := range array {
		if _, err := Boolean.Unmarshaler.UnmarshalText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"true", "F", "0", "False"}
	for _, v := range array {
		if _, err := Boolean.Unmarshaler.UnmarshalText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
}

func TestTime(t *testing.T) {
	v := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	testBinary(v, DateTime.Unmarshaler, t)
	testText(v, DateTime.Unmarshaler, t)
}
