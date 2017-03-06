/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package x

type ValueTypeInfo int32

// Type of a data inside DirectedEdge, Posting or NQuad
const (
	ValueUnknown ValueTypeInfo = iota // unknown type of value
	ValueEmpty                        // no UID and no value
	ValueUid                          // UID
	ValuePlain                        // plain old value without defined language tag
	ValueLang                         // value with defined language tag
)

// Helper function, to decide value type of DirectedEdge/Posting/NQuad
func ValueType(hasValue, hasLang, hasSpecialId bool) ValueTypeInfo {
	switch {
	case hasValue && hasLang:
		return ValueLang
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
