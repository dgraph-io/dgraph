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
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/x"
)

func testBinary(val Value, t *testing.T) {
	bytes, err := val.MarshalBinary()
	require.NoError(t, err)
	val2 := ValueForType(val.TypeID())
	err = val2.UnmarshalBinary(bytes)
	require.NoError(t, err)
	require.Equal(t, val, val2,
		"Expected marshal/unmarshal binary to parse to the same value: %v == %v", val2, val)
}

func testText(val Value, t *testing.T) {
	bytes, err := val.MarshalText()
	require.NoError(t, err)
	val2 := ValueForType(val.TypeID())
	err = val2.UnmarshalText(bytes)
	require.NoError(t, err)
	require.Equal(t, val, val2,
		"Expected marshal/unmarshal text to parse to the same value: %v == %v", val2, val)
}

func TestInt32(t *testing.T) {
	array := []Int32{1532, -122911, 0}
	for _, v := range array {
		testBinary(&v, t)
		testText(&v, t)
	}
}

func TestParseInt(t *testing.T) {
	array := []string{"abc", "5a", "2.5"}
	var i Int32
	for _, v := range array {
		if err := i.UnmarshalText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"52", "0", "-7"}
	for _, v := range array {
		if err := i.UnmarshalText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}

	data := []byte{0, 3, 1}
	if err := i.UnmarshalBinary(data); err == nil {
		t.Errorf("Expected error parsing %v", data)
	}
}

func TestFloat(t *testing.T) {
	array := []Float{15.32, -12.2911, 42, 0.0}
	for _, v := range array {
		testBinary(&v, t)
		testText(&v, t)
	}
}

func TestParseFloat(t *testing.T) {
	array := []string{"abc", "5a", "2.5a"}
	var f Float
	for _, v := range array {
		if err := f.UnmarshalText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"52", "0", "-7", "-2.5", "1e4"}
	for _, v := range array {
		if err := f.UnmarshalText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
	data := []byte{0, 3, 1, 5, 1}
	if err := f.UnmarshalBinary(data); err == nil {
		t.Errorf("Expected error parsing %v", data)
	}

}

func TestString(t *testing.T) {
	array := []String{"abc", ""}
	for _, v := range array {
		testBinary(&v, t)
		testText(&v, t)
	}
}

func TestBool(t *testing.T) {
	array := []Bool{true, false}
	for _, v := range array {
		testBinary(&v, t)
		testText(&v, t)
	}
}

func TestParseBool(t *testing.T) {
	array := []string{"abc", "5", ""}
	var b Bool
	for _, v := range array {
		if err := b.UnmarshalText([]byte(v)); err == nil {
			t.Errorf("Expected error parsing %s", v)
		}
	}
	array = []string{"true", "F", "0", "False"}
	for _, v := range array {
		if err := b.UnmarshalText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
}

func TestParseTime(t *testing.T) {
	array := []string{"2014-10-28T04:00:10", "2002-05-30T09:30:10Z",
		"2002-05-30T09:30:10.5", "2002-05-30T09:30:10-06:00"}
	var tm Time
	for _, v := range array {
		if err := tm.UnmarshalText([]byte(v)); err != nil {
			t.Errorf("Error parsing %s: %v", v, err)
		}
	}
}

func TestTime(t *testing.T) {
	v := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	testBinary(&Time{v}, t)
	testText(&Time{v}, t)
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
