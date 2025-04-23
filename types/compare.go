/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package types

import "github.com/hypermodeinc/dgraph/v25/x"

// CompareVals compares two values using the given comparison type.
// Should be used only in filtering arg1 by comparing with arg2.
// arg2 is reference Val to which arg1 is compared.
func CompareVals(op string, arg1, arg2 Val) bool {
	negateRes := func(b bool, e error) (bool, error) { // reverses result
		return !b, e
	}
	noError := func(b bool, e error) bool {
		return b && e == nil
	}
	switch op {
	case "ge":
		return noError(negateRes(Less(arg1, arg2)))
	case "gt":
		return noError(Less(arg2, arg1))
	case "le":
		return noError(negateRes(Less(arg2, arg1)))
	case "lt":
		return noError(Less(arg1, arg2))
	case "eq":
		return noError(Equal(arg1, arg2))
	default:
		// should have been checked at query level.
		x.Fatalf("Unknown ineqType %v", op)
	}
	return false
}

// CompareBetween compares if the dst value lie between
// two values val1 and val2(both inclusive).
func CompareBetween(dst, val1, val2 Val) bool {
	return CompareVals("ge", dst, val1) &&
		CompareVals("le", dst, val2)
}
