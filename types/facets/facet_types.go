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

import "github.com/dgraph-io/dgraph/protos"

const (
	IntID      = TypeID(protos.Facet_INT)
	FloatID    = TypeID(protos.Facet_FLOAT)
	BoolID     = TypeID(protos.Facet_BOOL)
	DateTimeID = TypeID(protos.Facet_DATETIME)
	StringID   = TypeID(protos.Facet_STRING)
)

type TypeID protos.Facet_ValType

// ValTypeForTypeID gives protos.Facet_ValType for given TypeID
func ValTypeForTypeID(typId TypeID) protos.Facet_ValType {
	switch typId {
	case IntID:
		return protos.Facet_INT
	case FloatID:
		return protos.Facet_FLOAT
	case BoolID:
		return protos.Facet_BOOL
	case DateTimeID:
		return protos.Facet_DATETIME
	case StringID:
		return protos.Facet_STRING
	}
	panic("Unhandled case in ValTypeForTypeID.")
}

// TypeIDForValType gives TypeID for protos.Facet_ValType
func TypeIDForValType(valType protos.Facet_ValType) TypeID {
	switch valType {
	case protos.Facet_INT:
		return IntID
	case protos.Facet_FLOAT:
		return FloatID
	case protos.Facet_BOOL:
		return BoolID
	case protos.Facet_DATETIME:
		return DateTimeID
	case protos.Facet_STRING:
		return StringID
	}
	panic("Unhandled case in TypeIDForValType.")
}
