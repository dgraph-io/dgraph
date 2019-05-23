/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
