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
