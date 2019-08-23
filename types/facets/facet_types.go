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

package facets

import "github.com/dgraph-io/dgo/protos/api"

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
	}
	panic("Unhandled case in ValTypeForTypeID.")
}
