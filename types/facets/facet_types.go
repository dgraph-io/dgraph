/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package facets

import (
	"github.com/dgraph-io/dgo/v250/protos/api"
)

const (
	// IntID represents the integer type.
	IntID = TypeID(api.Facet_INT)
	// FloatID represents the floating-point number type.
	FloatID = TypeID(api.Facet_FLOAT)
	// BoolID represents the boolean type.
	BoolID = TypeID(api.Facet_BOOL)
	// DateTimeID represents the datetime type.
	DateTimeID = TypeID(api.Facet_DATETIME)
	// StringID represents the string type.
	StringID = TypeID(api.Facet_STRING)
)

// TypeID represents the type of a facet.
type TypeID api.Facet_ValType

// ValTypeForTypeID gives api.Facet_ValType for given TypeID
func ValTypeForTypeID(typId TypeID) api.Facet_ValType {
	switch typId {
	case IntID:
		return api.Facet_INT
	case FloatID:
		return api.Facet_FLOAT
	case BoolID:
		return api.Facet_BOOL
	case DateTimeID:
		return api.Facet_DATETIME
	case StringID:
		return api.Facet_STRING
	default:
		panic("unhandled case in ValTypeForTypeID")
	}
}
