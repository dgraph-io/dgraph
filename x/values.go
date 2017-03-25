/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
