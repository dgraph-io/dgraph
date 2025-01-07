/*
 * Copyright 2025 Hypermode Inc. and Contributors
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

package constraints

// Signed interface taken from https://pkg.go.dev/golang.org/x/exp/constraints.
// We copy it here since constraints package future is uncertain.
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Unsigned interface taken from https://pkg.go.dev/golang.org/x/exp/constraints.
// We copy it here since constraints package future is uncertain.
type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Integer interface taken from https://pkg.go.dev/golang.org/x/exp/constraints.
// We copy it here since constraints package future is uncertain.
type Integer interface {
	Signed | Unsigned
}

// created for values of vfloat to support []float32 or []float64
type Float interface {
	float32 | float64
}

// Float interface
type AnyFloat interface {
	~float32 | ~float64
}

type Simple interface {
	Integer | AnyFloat | ~bool | ~string
}
