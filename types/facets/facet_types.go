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

import "github.com/dgraph-io/dgraph/protos/facetsp"

const (
	Int32ID    = TypeID(facetsp.Facet_INT32)
	FloatID    = TypeID(facetsp.Facet_FLOAT)
	BoolID     = TypeID(facetsp.Facet_BOOL)
	DateTimeID = TypeID(facetsp.Facet_DATETIME)
	StringID   = TypeID(facetsp.Facet_STRING)
)

type TypeID facetsp.Facet_ValType

// ValTypeForTypeID gives facetsp.Facet_ValType for given TypeID
func ValTypeForTypeID(typId TypeID) facetsp.Facet_ValType {
	switch typId {
	case Int32ID:
		return facetsp.Facet_INT32
	case FloatID:
		return facetsp.Facet_FLOAT
	case BoolID:
		return facetsp.Facet_BOOL
	case DateTimeID:
		return facetsp.Facet_DATETIME
	case StringID:
		return facetsp.Facet_STRING
	}
	panic("Unhandled case in ValTypeForTypeID.")
}

// TypeIDForValType gives TypeID for facetsp.Facet_ValType
func TypeIDForValType(valType facetsp.Facet_ValType) TypeID {
	switch valType {
	case facetsp.Facet_INT32:
		return Int32ID
	case facetsp.Facet_FLOAT:
		return FloatID
	case facetsp.Facet_BOOL:
		return BoolID
	case facetsp.Facet_DATETIME:
		return DateTimeID
	case facetsp.Facet_STRING:
		return StringID
	}
	panic("Unhandled case in TypeIDForValType.")
}
