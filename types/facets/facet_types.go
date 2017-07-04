/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
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
