/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package schema

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/golang/glog"

	dgTypes "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	ErrExpectedScalar = "An object type was returned, but GraphQL was expecting a scalar. " +
		"This indicates an internal error - " +
		"probably a mismatch between the GraphQL and Dgraph/remote schemas. " +
		"The value was resolved as null (which may trigger GraphQL error propagation) " +
		"and as much other data as possible returned."

	ErrExpectedSingleItem = "A list was returned, but GraphQL was expecting just one item. " +
		"This indicates an internal error - " +
		"probably a mismatch between the GraphQL and Dgraph/remote schemas. " +
		"The value was resolved as null (which may trigger GraphQL error propagation) " +
		"and as much other data as possible returned."

	ErrExpectedList = "An item was returned, but GraphQL was expecting a list of items. " +
		"This indicates an internal error - " +
		"probably a mismatch between the GraphQL and Dgraph/remote schemas. " +
		"The value was resolved as null (which may trigger GraphQL error propagation) " +
		"and as much other data as possible returned."

	ErrExpectedNonNull = "Non-nullable field '%s' (type %s) was not present in result from Dgraph.  " +
		"GraphQL error propagation triggered."
)

var (
	// JsonNull are the bytes to represent null in JSON.
	JsonNull = []byte("null")
	// JsonEmptyList are the bytes to represent an empty list in JSON.
	JsonEmptyList = []byte("[]")
)

// Unmarshal is like json.Unmarshal() except it uses a custom decoder which preserves number
// precision by unmarshalling them into json.Number.
func Unmarshal(data []byte, v interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(v)
}

// CompleteObject builds a json GraphQL result object for the current query level.
// It returns a bracketed json object like { f1:..., f2:..., ... }.
// At present, it is only used for building custom results by:
//   - Admin Server
//   - @custom(http: {...}) query/mutation
//   - @custom(dql: ...) queries
//
// fields are all the fields from this bracketed level in the GraphQL  query, e.g:
//
//	{
//	  name
//	  dob
//	  friends {...}
//	}
//
// If it's the top level of a query then it'll be the top level query name.
//
// typ is the expected type matching those fields, e.g. above that'd be something
// like the `Person` type that has fields name, dob and friends.
//
// res is the results map from Dgraph for this level of the query.  This map needn't
// contain values for all the requested fields, e.g. if there's no corresponding
// values in the store or if the query contained a filter that excluded a value.
// So res might be the map : name->"A Name", friends -> []interface{}
//
// CompleteObject fills out this result putting in null for any missing values
// (dob above) and applying GraphQL error propagation for any null fields that the
// schema says can't be null.
//
// Example:
//
// if the map is name->"A Name", friends -> []interface{}
//
// and "dob" is nullable then the result should be json object
// {"name": "A Name", "dob": null, "friends": ABC}
// where ABC is the result of applying CompleteValue to res["friends"]
//
// if "dob" were non-nullable (maybe it's type is DateTime!), then the result is
// nil and the error propagates to the enclosing level.
func CompleteObject(
	path []interface{},
	fields []Field,
	res map[string]interface{}) ([]byte, x.GqlErrorList) {

	var errs x.GqlErrorList
	var buf bytes.Buffer
	comma := ""

	// seenField keeps track of fields which have been seen as part of
	// interface to avoid double entry in the resulting response
	seenField := make(map[string]bool)

	x.Check2(buf.WriteRune('{'))
	var dgraphTypes []string
	if typename, ok := res[Typename].(string); ok && len(typename) > 0 {
		// @custom(http: {...}) query/mutation results may return __typename in response for
		// abstract fields, lets use that information if present.
		dgraphTypes = []string{typename}
	} else if dgTypeVals, ok := res["dgraph.type"].([]interface{}); ok {
		// @custom(dql: ...) query results may return dgraph.type in response for abstract fields
		for _, val := range dgTypeVals {
			if typename, ok = val.(string); ok {
				dgraphTypes = append(dgraphTypes, typename)
			}
		}
	}

	for _, f := range fields {
		if f.SkipField(dgraphTypes, seenField) {
			continue
		}

		x.Check2(buf.WriteString(comma))
		f.CompleteAlias(&buf)

		val := res[f.RemoteResponseName()]
		if f.RemoteResponseName() == Typename {
			// From GraphQL spec:
			// https://graphql.github.io/graphql-spec/June2018/#sec-Type-Name-Introspection
			// "GraphQL supports type name introspection at any point within a query by the
			// meta‐field  __typename: String! when querying against any Object, Interface,
			// or Union. It returns the name of the object type currently being queried."

			// If we have __typename information, we will use that to figure out the type
			// otherwise we will get it from the schema.
			val = f.TypeName(dgraphTypes)
		}

		// Check that data should be of list type when we expect f.Type().ListType() to be non-nil.
		if val != nil && f.Type().ListType() != nil {
			switch val.(type) {
			case []interface{}, []map[string]interface{}:
			default:
				// We were expecting a list but got a value which wasn't a list. Lets return an
				// error.
				return nil, x.GqlErrorList{f.GqlErrorf(path, ErrExpectedList)}
			}
		}

		completed, err := CompleteValue(append(path, f.ResponseName()), f, val)
		errs = append(errs, err...)
		if completed == nil {
			if !f.Type().Nullable() {
				return nil, errs
			}
			completed = JsonNull
		}
		x.Check2(buf.Write(completed))
		comma = ","
	}
	x.Check2(buf.WriteRune('}'))

	return buf.Bytes(), errs
}

// CompleteValue applies the value completion algorithm to a single value, which
// could turn out to be a list or object or scalar value.
func CompleteValue(
	path []interface{},
	field Field,
	val interface{}) ([]byte, x.GqlErrorList) {

	switch val := val.(type) {
	case map[string]interface{}:
		switch field.Type().Name() {
		case "String", "ID", "Boolean", "Float", "Int", "Int64", "DateTime":
			return nil, x.GqlErrorList{field.GqlErrorf(path, ErrExpectedScalar)}
		}
		enumValues := field.EnumValues()
		if len(enumValues) > 0 {
			return nil, x.GqlErrorList{field.GqlErrorf(path, ErrExpectedScalar)}
		}
		return CompleteObject(path, field.SelectionSet(), val)
	case []interface{}:
		return completeList(path, field, val)
	case []map[string]interface{}:
		// This case is different from the []interface{} case above and is true for admin queries
		// where we built the val ourselves.
		listVal := make([]interface{}, 0, len(val))
		for _, v := range val {
			listVal = append(listVal, v)
		}
		return completeList(path, field, listVal)
	default:
		if val == nil {
			if b := field.NullValue(); b != nil {
				return b, nil
			}

			return nil, x.GqlErrorList{field.GqlErrorf(path, ErrExpectedNonNull,
				field.Name(), field.Type())}
		}

		// val is a scalar
		val, gqlErr := coerceScalar(val, field, path)
		if len(gqlErr) > 0 {
			return nil, gqlErr
		}

		// Can this ever error?  We can't have an unsupported type or value because
		// we just unmarshalled this val.
		b, err := json.Marshal(val)
		if err != nil {
			gqlErr := x.GqlErrorList{field.GqlErrorf(path,
				"Error marshalling value for field '%s' (type %s).  "+
					"Resolved as null (which may trigger GraphQL error propagation) ",
				field.Name(), field.Type())}

			if field.Type().Nullable() {
				return JsonNull, gqlErr
			}

			return nil, gqlErr
		}

		return b, nil
	}
}

// completeList applies the completion algorithm to a list field and result.
//
// field is one field from the query - which should have a list type in the
// GraphQL schema.
//
// values is the list of values found by the query for this field.
//
// CompleteValue() is applied to every list element, but
// the type of field can only be a scalar list like [String], or an object
// list like [Person], so schematically the final result is either
// [ CompleteValue("..."), CompleteValue("..."), ... ]
// or
// [ CompleteObject({...}), CompleteObject({...}), ... ]
// depending on the type of list.
//
// If the list has non-nullable elements (a type like [T!]) and any of those
// elements resolve to null, then the whole list is crushed to null.
func completeList(
	path []interface{},
	field Field,
	values []interface{}) ([]byte, x.GqlErrorList) {

	if field.Type().ListType() == nil {
		// lets coerce a one item list to a single value in case the type of this field wasn't list.
		if len(values) == 1 {
			return CompleteValue(path, field, values[0])
		}
		// This means either a bug on our part - in admin server.
		// or @custom query/mutation returned something unexpected.
		//
		// Let's crush it to null so we still get something from the rest of the
		// query and log the error.
		return mismatched(path, field)
	}

	var buf bytes.Buffer
	var errs x.GqlErrorList
	comma := ""

	x.Check2(buf.WriteRune('['))
	for i, b := range values {
		r, err := CompleteValue(append(path, i), field, b)
		errs = append(errs, err...)
		x.Check2(buf.WriteString(comma))
		if r == nil {
			if !field.Type().ListType().Nullable() {
				// Unlike the choice in CompleteValue() above, where we turn missing
				// lists into [], the spec explicitly calls out:
				//  "If a List type wraps a Non-Null type, and one of the
				//  elements of that list resolves to null, then the entire list
				//  must resolve to null."
				//
				// The list gets reduced to nil, but an error recording that must
				// already be in errs.  See
				// https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
				// "If the field returns null because of an error which has already
				// been added to the "errors" list in the response, the "errors"
				// list must not be further affected."
				// The behavior is also in the examples in here:
				// https://graphql.github.io/graphql-spec/June2018/#sec-Errors
				return nil, errs
			}
			x.Check2(buf.Write(JsonNull))
		} else {
			x.Check2(buf.Write(r))
		}
		comma = ","
	}
	x.Check2(buf.WriteRune(']'))

	return buf.Bytes(), errs
}

func mismatched(path []interface{}, field Field) ([]byte, x.GqlErrorList) {
	glog.Errorf("completeList() called in resolving %s (Line: %v, Column: %v), "+
		"but its type is %s.\n"+
		"That could indicate the Dgraph schema doesn't match the GraphQL schema.",
		field.Name(), field.Location().Line, field.Location().Column, field.Type().Name())

	val, errs := CompleteValue(path, field, nil)
	return val, append(errs, field.GqlErrorf(path, ErrExpectedSingleItem))
}

// coerceScalar coerces a scalar value to field.Type() if possible according to the coercion rules
// defined in the GraphQL spec. If this is not possible, then it returns an error. The crux of
// coercion rules defined in the spec is to not lose information during coercion.
// Note that, admin server specifically uses these:
//   - json.Number
//   - schema.Unmarshal() everywhere else
//
// And, @custom(http: {...}) query/mutation would always use schema.Unmarshal().
// Now, schema.Unmarshal() can only give one of the following types for scalars:
//   - bool
//   - string
//   - json.Number (because it uses custom JSON decoder which preserves number precision)
//
// So, we need to consider only these cases at present.
func coerceScalar(val interface{}, field Field, path []interface{}) (interface{},
	x.GqlErrorList) {

	valueCoercionError := func(val interface{}) x.GqlErrorList {
		return x.GqlErrorList{field.GqlErrorf(path,
			"Error coercing value '%+v' for field '%s' to type %s.",
			val, field.Name(), field.Type().Name())}
	}

	switch field.Type().Name() {
	case "String", "ID":
		switch v := val.(type) {
		case bool:
			val = strconv.FormatBool(v)
		case string:
			// do nothing, val is already string
		case json.Number:
			val = v.String()
		default:
			return nil, valueCoercionError(v)
		}
	case "Boolean":
		switch v := val.(type) {
		case bool:
			// do nothing, val is already bool
		case string:
			val = len(v) > 0
		case json.Number:
			valFloat, _ := v.Float64()
			val = valFloat != 0
		default:
			return nil, valueCoercionError(v)
		}
	case "Int":
		switch v := val.(type) {
		case bool:
			if v {
				val = 1
			} else {
				val = 0
			}
		case string:
			floatVal, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, valueCoercionError(v)
			}
			i32Val := int32(floatVal)
			if floatVal == float64(i32Val) {
				val = i32Val
			} else {
				return nil, valueCoercionError(v)
			}
		case json.Number:
			// float64 can always contain 32 bit integers without any information loss
			floatVal, err := v.Float64()
			if err != nil {
				return nil, valueCoercionError(v)
			}
			i32Val := int32(floatVal) // convert the float64 value to int32
			// now if converting the int32 back to float64 results in a mismatch means we lost
			// information during conversion, so return error.
			if floatVal != float64(i32Val) {
				return nil, valueCoercionError(v)
			}
			// otherwise, do nothing as val is already a valid number in int32 range
		default:
			return nil, valueCoercionError(v)
		}
	case "Int64":
		switch v := val.(type) {
		case bool:
			if v {
				val = 1
			} else {
				val = 0
			}
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		case json.Number:
			if _, err := v.Int64(); err != nil {
				return nil, valueCoercionError(v)
			}
			// do nothing, as val is already a valid number in int64 range
		default:
			return nil, valueCoercionError(v)
		}
	// UInt64 is present only in admin schema.
	case "UInt64":
		switch v := val.(type) {
		case json.Number:
			if _, err := strconv.ParseUint(v.String(), 10, 64); err != nil {
				return nil, valueCoercionError(v)
			}
			// do nothing, as val is already a valid number in UInt64 range
		default:
			return nil, valueCoercionError(v)
		}
	case "Float":
		switch v := val.(type) {
		case bool:
			if v {
				val = 1.0
			} else {
				val = 0.0
			}
		case string:
			i, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		case json.Number:
			_, err := v.Float64()
			if err != nil {
				return nil, valueCoercionError(v)
			}
			// do nothing, as val is already a valid number in float
		default:
			return nil, valueCoercionError(v)
		}
	case "DateTime":
		switch v := val.(type) {
		case string:
			if t, err := dgTypes.ParseTime(v); err != nil {
				return nil, valueCoercionError(v)
			} else {
				// let's make sure that we always return a string in RFC3339 format as the original
				// string could have been in some other format.
				val = t.Format(time.RFC3339)
			}
		case json.Number:
			valFloat, err := v.Float64()
			if err != nil {
				return nil, valueCoercionError(v)
			}
			truncated := math.Trunc(valFloat)
			if truncated == valFloat {
				// Lets interpret int values as unix timestamp.
				t := time.Unix(int64(truncated), 0).UTC()
				val = t.Format(time.RFC3339)
			} else {
				return nil, valueCoercionError(v)
			}
		default:
			return nil, valueCoercionError(v)
		}
	default:
		enumValues := field.EnumValues()
		// At this point we should only get fields which are of ENUM type, so we can return
		// an error if we don't get any enum values.
		if len(enumValues) == 0 {
			return nil, valueCoercionError(val)
		}
		switch v := val.(type) {
		case string:
			// Lets check that the enum value is valid.
			if !x.HasString(enumValues, v) {
				return nil, valueCoercionError(val)
			}
			// do nothing, as val already has a valid value
		default:
			return nil, valueCoercionError(v)
		}
	}
	return val, nil
}
