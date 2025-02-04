/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

// ValueTypeInfo represents information about the type of values in DirectedEdge/Posting/N-Quad.
type ValueTypeInfo int32

// Type of a data inside DirectedEdge, Posting or N-Quad
const (
	// ValueUnknown represents an unknown type of value.
	ValueUnknown ValueTypeInfo = iota
	// ValueEmpty represents a value with no UID and no value.
	ValueEmpty
	// ValueUid represents a value with an UID.
	ValueUid
	// ValuePlain represents a plain old value without defined language tag.
	ValuePlain
	// ValueMulti represents a value which is part of a multi-value posting list (like language).
	ValueMulti
)

// ValueType s a helper function to decide value type of DirectedEdge/Posting/N-Quad.
func ValueType(hasValue, hasLang, hasSpecialId bool) ValueTypeInfo {
	switch {
	case hasValue && hasLang:
		return ValueMulti
	case hasValue && !hasLang:
		return ValuePlain
	case !hasValue && hasSpecialId:
		return ValueEmpty
	case !hasValue && !hasSpecialId:
		return ValueUid
	default:
		return ValueUnknown
	}
}
