/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBasicTypes(t *testing.T) {
	require.Equal(t, uintptr(1), DeepSizeOf(false))
	require.Equal(t, uintptr(1), DeepSizeOf(uint8(0)))
	require.Equal(t, uintptr(1), DeepSizeOf(int8(0)))
	require.Equal(t, uintptr(2), DeepSizeOf(uint16(0)))
	require.Equal(t, uintptr(2), DeepSizeOf(int16(0)))
	require.Equal(t, uintptr(4), DeepSizeOf(uint32(0)))
	require.Equal(t, uintptr(4), DeepSizeOf(int32(0)))
	require.Equal(t, uintptr(8), DeepSizeOf(uint64(0)))
	require.Equal(t, uintptr(8), DeepSizeOf(int64(0)))
	require.Equal(t, uintptr(4), DeepSizeOf(float32(0)))
	require.Equal(t, uintptr(8), DeepSizeOf(float64(0)))
	require.Equal(t, uintptr(8), DeepSizeOf(complex64(0)))
	require.Equal(t, uintptr(16), DeepSizeOf(complex128(0)))
	require.Equal(t, uintptr(16), DeepSizeOf(""))
	require.Equal(t, uintptr(19), DeepSizeOf("abc"))
}

func TestSlice(t *testing.T) {
	require.Equal(t, uintptr(0), DeepSizeOf([]byte(nil)))
	require.Equal(t, uintptr(0), DeepSizeOf([]byte{0, 0}))
	require.Equal(t, uintptr(0), DeepSizeOf([]uint64(nil)))
}

type intStruct struct {
	A int64
	B int32
	C uint16
}

func TestIntStruct(t *testing.T) {
	val := intStruct{}
	require.Equal(t, uintptr(14), DeepSizeOf(val))
}

type pointerStruct struct {
	A int64
	B *string
}

func TestPointerStruct(t *testing.T) {
	val := pointerStruct{}
	require.Equal(t, uintptr(16), DeepSizeOf(val))

	testString := "1234"
	val.B = &testString
	require.Equal(t, uintptr(16), DeepSizeOf(val))
}
