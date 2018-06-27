/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package x

type ValueTypeInfo int32

// Type of a data inside DirectedEdge, Posting or NQuad
const (
	ValueUnknown ValueTypeInfo = iota // unknown type of value
	ValueEmpty                        // no UID and no value
	ValueUid                          // UID
	ValuePlain                        // plain old value without defined language tag
	// Value which is part of a multi-value posting list (like language).
	ValueMulti
)

// Helper function, to decide value type of DirectedEdge/Posting/NQuad
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
