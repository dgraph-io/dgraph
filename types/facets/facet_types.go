/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package facets

import "github.com/dgraph-io/dgo/protos/api"

const (
	IntID      = TypeID(api.Facet_INT)
	FloatID    = TypeID(api.Facet_FLOAT)
	BoolID     = TypeID(api.Facet_BOOL)
	DateTimeID = TypeID(api.Facet_DATETIME)
	StringID   = TypeID(api.Facet_STRING)
)

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
	}
	panic("Unhandled case in ValTypeForTypeID.")
}

// TypeIDForValType gives TypeID for api.Facet_ValType
func TypeIDForValType(valType api.Facet_ValType) TypeID {
	switch valType {
	case api.Facet_INT:
		return IntID
	case api.Facet_FLOAT:
		return FloatID
	case api.Facet_BOOL:
		return BoolID
	case api.Facet_DATETIME:
		return DateTimeID
	case api.Facet_STRING:
		return StringID
	}
	panic("Unhandled case in TypeIDForValType.")
}
