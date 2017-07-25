/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import "math"

func Uint64(l int, b []byte) uint64 {
	switch l {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(b[1]) | uint64(b[0])<<8
	case 3:
		return uint64(b[2]) | uint64(b[1])<<8 | uint64(b[0])<<16
	case 4:
		return uint64(b[3]) | uint64(b[2])<<8 | uint64(b[1])<<16 | uint64(b[0])<<24
	case 5:
		return uint64(b[4]) | uint64(b[3])<<8 | uint64(b[2])<<16 | uint64(b[1])<<24 |
			uint64(b[0])<<32
	case 6:
		return uint64(b[5]) | uint64(b[4])<<8 | uint64(b[3])<<16 | uint64(b[2])<<24 |
			uint64(b[1])<<32 | uint64(b[0])<<40
	case 7:
		return uint64(b[6]) | uint64(b[5])<<8 | uint64(b[4])<<16 | uint64(b[3])<<24 |
			uint64(b[2])<<32 | uint64(b[1])<<40 | uint64(b[0])<<48
	case 8:
		return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
			uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
	default:
		Fatalf("Unknown length passed")
	}
	return 0
}

func PutUint64(l int, b []byte, v uint64) {
	switch l {
	case 1:
		b[0] = byte(v)
	case 2:
		b[0] = byte(v >> 8)
		b[1] = byte(v)
	case 3:
		b[0] = byte(v >> 16)
		b[1] = byte(v >> 8)
		b[2] = byte(v)
	case 4:
		b[0] = byte(v >> 24)
		b[1] = byte(v >> 16)
		b[2] = byte(v >> 8)
		b[3] = byte(v)
	case 5:
		b[0] = byte(v >> 32)
		b[1] = byte(v >> 24)
		b[2] = byte(v >> 16)
		b[3] = byte(v >> 8)
		b[4] = byte(v)
	case 6:
		b[0] = byte(v >> 40)
		b[1] = byte(v >> 32)
		b[2] = byte(v >> 24)
		b[3] = byte(v >> 16)
		b[4] = byte(v >> 8)
		b[5] = byte(v)
	case 7:
		b[0] = byte(v >> 48)
		b[1] = byte(v >> 40)
		b[2] = byte(v >> 32)
		b[3] = byte(v >> 24)
		b[4] = byte(v >> 16)
		b[5] = byte(v >> 8)
		b[6] = byte(v)
	case 8:
		b[0] = byte(v >> 56)
		b[1] = byte(v >> 48)
		b[2] = byte(v >> 40)
		b[3] = byte(v >> 32)
		b[4] = byte(v >> 24)
		b[5] = byte(v >> 16)
		b[6] = byte(v >> 8)
		b[7] = byte(v)
	default:
		Fatalf("Unknown length passed")
	}
}

func BytesForEncoding(val uint64) int {
	return int(math.Ceil(math.Log2((float64(val)))))
}
