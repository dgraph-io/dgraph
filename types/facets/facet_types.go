/*
 * Copyright 2017 Dgraph Labs, Inc.
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

package facets

const (
	Int32ID    = TypeID(Facet_INT32)
	FloatID    = TypeID(Facet_FLOAT)
	BoolID     = TypeID(Facet_BOOL)
	DateTimeID = TypeID(Facet_DATETIME)
	StringID   = TypeID(Facet_STRING)
)

type TypeID Facet_ValType

// ValTypeForTypeID gives Facet_ValType for given TypeID
func ValTypeForTypeID(typId TypeID) Facet_ValType {
	switch typId {
	case Int32ID:
		return Facet_INT32
	case FloatID:
		return Facet_FLOAT
	case BoolID:
		return Facet_BOOL
	case DateTimeID:
		return Facet_DATETIME
	case StringID:
		return Facet_STRING
	}
	panic("Unhandled case in ValTypeForTypeID.")
}

// TypeIDForValType gives TypeID for Facet_ValType
func TypeIDForValType(valType Facet_ValType) TypeID {
	switch valType {
	case Facet_INT32:
		return Int32ID
	case Facet_FLOAT:
		return FloatID
	case Facet_BOOL:
		return BoolID
	case Facet_DATETIME:
		return DateTimeID
	case Facet_STRING:
		return StringID
	}
	panic("Unhandled case in TypeIDForValType.")
}
