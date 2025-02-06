/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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
