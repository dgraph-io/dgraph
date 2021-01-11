/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"bytes"
	"encoding/json"
	"fmt"

	gqlSchema "github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

var (
	jsonNull = []byte("null")
)

type graphQLEncodingCtx struct {
	// buf is the buffer which stores the encoded GraphQL response.
	buf *bytes.Buffer
	// gqlErrs stores GraphQL errors, if any. Even if there are GraphQL errors,
	// the buffer will always have a valid JSON response.
	gqlErrs x.GqlErrorList
	// dgraphTypeAttrId stores the id for attr `dgraph.type`.
	// dgraph.type predicate is requested by GraphQL layer for abstract types. We are caching
	// the id for this attr here so that instead of doing this:
	// 		enc.attrForID(enc.getAttr(child)) == "dgraph.type"
	// we would now be able to do just this:
	// 		enc.getAttr(child) == dgraphTypeAttrId
	// Meaning, instead of looking up an int in a map and then comparing strings,
	// we can just compare ints directly.
	dgraphTypeAttrId uint16
}

func writeKeyGraphQL(field gqlSchema.Field, out *bytes.Buffer) {
	x.Check2(out.WriteRune('"'))
	x.Check2(out.WriteString(field.ResponseName()))
	x.Check2(out.WriteString(`":`))
}

// TODO:
//  * Scalar coercion
//  * Enums
//  * Password queries
//  * (cleanup) Cleanup code from resolve pkg
//  * (cleanup) make const args like *bytes.Buffer the first arg in funcs as a best practice.
//  * (cleanup) refactor this func as `func (ctx *graphQLEncodingCtx) encode(enc *encoder, ...)`.
func (enc *encoder) encodeGraphQL(ctx *graphQLEncodingCtx, fj fastJsonNode, fjIsRoot bool,
	childSelectionSet []gqlSchema.Field, parentField gqlSchema.Field,
	parentPath []interface{}) bool {
	child := enc.children(fj)
	// This is a scalar value.
	if child == nil {
		val, err := enc.getScalarVal(fj)
		if err != nil {
			ctx.gqlErrs = append(ctx.gqlErrs, parentField.GqlErrorf(parentPath, err.Error()))
			// return false so that the caller can appropriately handle null writing.
			return false
		}
		if val == nil {
			// val being nil here can only be the case for a top-level query and not for a nested
			// field. val being nil indicates that the top-level query has no value to resolve
			// to, and we need to write null/[]/raise an error depending on the return type of the
			// query. Now, for queries which return a list (whether nullable or not), [] would
			// anyways be written by the parent encodeGraphQL() call. If we return false from here,
			// then too the parent encodeGraphQL() call will write [], but then we won't be able to
			// distinguish between whether the first item of the list was null or the whole query
			// had no results.
			// So, for lists lets return true.
			// We will return false for single valued cases so that the caller can correctly write
			// null or raise an error.
			// Note that we don't need to add any errors to the gqlErrs here.
			if parentField.Type().ListType() != nil {
				return true
			}
			return false
		}

		// here we have a valid value, lets write it to buffer appropriately.
		if parentField.Type().IsGeo() {
			var geoVal map[string]interface{}
			x.Check(json.Unmarshal(val, &geoVal)) // this unmarshal can't error
			if err := completeGeoObject(parentPath, parentField, geoVal, ctx.buf); err != nil {
				ctx.gqlErrs = append(ctx.gqlErrs, err)
				return false
			}
		} else {
			x.Check2(ctx.buf.Write(val))
		}
		// we have successfully written the scalar value, lets return true to indicate that this
		// call to encodeGraphQL() was successful.
		return true
	}

	// if we are here, ensure that GraphQL was expecting an object, otherwise return error.
	if len(childSelectionSet) == 0 {
		ctx.gqlErrs = append(ctx.gqlErrs, parentField.GqlErrorf(parentPath,
			gqlSchema.ErrExpectedScalar))
		// return false so that the caller can appropriately handle null writing.
		return false
	}

	// This is an internal node. Write the opening { for the JSON object
	x.Check2(ctx.buf.WriteRune('{'))

	// if GraphQL layer requested dgraph.type predicate, then it would always be the first child in
	// the response as it is always written first in DQL query. So, if we get data for dgraph.type
	// predicate then just save it in dgraphTypes slice, no need to write it to JSON yet.
	var dgraphTypes []interface{}
	for enc.getAttr(child) == ctx.dgraphTypeAttrId {
		val, err := enc.getScalarVal(child) // val is a quoted string like: "Human"
		if err != nil {
			// TODO: correctly format error, it should be on __typename field if present?
			ctx.gqlErrs = append(ctx.gqlErrs, x.GqlErrorf(err.Error()).WithPath(parentPath))
			child = child.next
			if child == nil {
				break
			}
			continue
		}

		typeStr := string(val)
		typeStr = typeStr[1 : len(typeStr)-1] // remove `"` from beginning and end
		dgraphTypes = append(dgraphTypes, typeStr)

		child = child.next
		if child == nil {
			break
		}
	}

	cnt := 0       // used to figure out how many times continuously we have seen the current attr
	i := 0         // used to iterate over childSelectionSet
	keyEndPos := 0 // used to store the length of output buffer at which a JSON key ends to
	// correctly write value as null, if need be.
	nullWritten := false // indicates whether null has been written as value for the current
	// selection or not. Used to figure out whether to write the closing ] for JSON arrays.
	seenField := make(map[string]bool) // seenField map keeps track of fields which have been seen
	// as part of interface to avoid double entry in the resulting response

	var curSelection gqlSchema.Field // used to store the current selection in the childSelectionSet
	var curSelectionIsList bool      // indicates whether the type of curSelection is list or not
	var cur, next fastJsonNode       // used to iterate over data in fastJson nodes

	// We need to keep iterating only if:
	// 1. There is data to be processed for the current level. AND,
	// 2. There are unprocessed fields in the childSelectionSet
	// These are the respective examples to consider for each case:
	// 1. Sometimes GraphQL layer requests `dgraph.uid: uid` in the rewritten DQL query as the last
	//    field at every level. This is not part of GraphQL selection set at any level,
	//    but just exists as the last field in the DQL query resulting as the last fastJsonNode
	//    child, and hence we need to ignore it so as not to put it in the user facing JSON.
	//    This is case 1 where we have data to be processed but no child left in the selection set
	//    to use it. The condition `i < len(childSelectionSet)` comes handy in this case.
	// 2. It may happen that a field requested in a GraphQL query, may not have any data for some
	//    nodes in the result. If such a field is the last field in the selection set or there is a
	//    set of such fields which are at the end of selection set, then we need to write null
	//    values for them with appropriate errors. This is case 2 where there are unprocessed fields
	//    but no data for them. This is handled after this for loop.
	for child != nil && i < len(childSelectionSet) {
		cnt++
		nullWritten = false // reset it at every iteration
		curSelection = childSelectionSet[i]
		cur = child
		next = cur.next

		// Step-1: Skip the field OR Write JSON key and opening [ for JSON arrays
		if cnt == 1 {
			// we need to check if the field should be skipped only when it is encountered for
			// the first time
			if skipField(curSelection, dgraphTypes, seenField) {
				cnt = 0 // Reset the count,
				// indicating that we need to write the JSON key in next iteration.
				i++
				// if this is the last field and shouldn't be included,
				// then need to remove comma from the buffer if one was present.
				if i == len(childSelectionSet) {
					checkAndStripComma(ctx.buf)
				}
				// also if there was any data for this field, need to skip that. There may not be
				// data in case this field was added from a fragment on another type.
				attrId := enc.idForAttr(curSelection.DgraphAlias())
				if enc.getAttr(cur) == attrId {
					for next != nil && enc.getAttr(next) == attrId {
						next = next.next
					}
					child = next
				}
				continue
			}

			// Write JSON key and opening [ for JSON arrays
			writeKeyGraphQL(curSelection, ctx.buf)
			keyEndPos = ctx.buf.Len()
			curSelectionIsList = curSelection.Type().ListType() != nil
			if curSelectionIsList {
				x.Check2(ctx.buf.WriteRune('['))
			}
		}

		// Step-2: Write JSON value
		if curSelection.Name() == gqlSchema.Typename {
			// If the current selection is __typename then we find out the typename using the
			// dgraphTypes slice saved earlier.
			// TODO (cleanup): refactor the TypeName method to accept []string
			x.Check2(ctx.buf.Write([]byte(`"` + curSelection.TypeName(dgraphTypes) + `"`)))
			// We don't need to iterate to next fastJson node in this case,
			// as the current node will have data for the next field in the selection set.
		} else if curSelection.DgraphAlias() != enc.attrForID(enc.getAttr(cur)) {
			// if the current fastJson node doesn't hold data for the current GraphQL selection,
			// then there can be two cases:
			// 1. The current fastJson node holds data for a next selection and there was no data
			//    present for the current GraphQL selection, so need to write null for the current
			//    GraphQL selection with appropriate errors.
			// 2. The current fastJson node holds data for count(pred), the current GraphQL
			//    selection is an aggregate field at child level and there was no data present for
			//    it. So, need to write null for the current GraphQL selection but also need to
			//    skip all the count(pred) fastJson nodes which were requested from within the
			//    current GraphQL selection.
			// 3. The current fastJson node holds data which wasn't requested by any GraphQL
			//    selection, but instead by a DQL selection added by GraphQL layer; and the data
			//    for current selection may be present in an upcoming fastJson node.
			//    Point to note is that this case doesn't happen as the GraphQL layer adds such
			//    DQL selections only at the beginning (dgraph.type) or end (dgraph.uid: uid) of a
			//    DQL selection set, but not in middle. The beginning case we have already handled,
			//    and the end case would either be ignored by this for loop or handled as case 1.
			// So, we don't have a need to handle case 3, and need to always write null with
			// appropriate errors.
			// TODO: check if case 3 can happen for @custom(dql: "")

			// handles null writing for case 1 & 2
			if nullWritten = writeGraphQLNull(curSelection, ctx.buf, keyEndPos); !nullWritten {
				ctx.gqlErrs = append(ctx.gqlErrs, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedNonNull, curSelection.Name(),
					curSelection.Type()))
				return false
			}
			// handles skipping for case 2
			if !fjIsRoot && curSelection.IsAggregateField() {
				child = skipCountAggregateChildren(curSelection, cur)
			}
			// we don't need to iterate to next fastJson node here.
		} else {
			// This is the case where the current fastJson node holds data for the current
			// GraphQL selection. There are following possible sub-cases:
			// 1. current GraphQL selection == list type
			//    current fastJson node == list type
			//    => Both GraphQL and DQL schema are in list form, recursively encode it.
			// 2. current GraphQL selection == list type
			//    current fastJson node != list type
			//    => There is a mismatch between the GraphQL and DQL schema. Raise a field error.
			// 3. current GraphQL selection != list type
			//    current fastJson node == list type
			//    => There is a mismatch between the GraphQL and DQL schema. Raise a field error.
			// 4. current GraphQL selection != list type
			//    current fastJson node != list type
			//    => Both GraphQL and DQL schema are in non-list form, recursively encode it.
			// Apart from these there is a special case of aggregate queries/fields where:
			//    current GraphQL selection != list type
			//    current fastJson node == list type
			//    => This is not a mismatch between the GraphQL and DQL schema and should be
			//       handled appropriately.
			if curSelectionIsList && enc.getList(cur) {
				// handles case 1
				itemPos := ctx.buf.Len()
				// List items which are scalars will never have null as a value returned
				// from Dgraph, but there can be coercion errors due to which their encoding
				// may return false and we will need to write null as a value for them.
				// Similarly, List items which are objects will also not have null as a
				// value returned from Dgraph, but there can be a nested non-nullable field
				// which may trigger the object to turn out to be null.
				if !enc.encodeGraphQL(ctx, cur, false, curSelection.SelectionSet(), curSelection,
					append(parentPath, curSelection.ResponseName(), cnt-1)) {
					// Unlike the choice in writeGraphQLNull(), where we turn missing
					// lists into [], the spec explicitly calls out:
					//  "If a List type wraps a Non-Null type, and one of the
					//  elements of that list resolves to null, then the entire list
					//  must resolve to null."
					//
					// The list gets reduced to null, but an error recording that must
					// already be in errs.  See
					// https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
					// "If the field returns null because of an error which has already
					// been added to the "errors" list in the response, the "errors"
					// list must not be further affected."
					// The behavior is also in the examples in here:
					// https://graphql.github.io/graphql-spec/June2018/#sec-Errors
					typ := curSelection.Type()
					if typ.ListType().Nullable() {
						ctx.buf.Truncate(itemPos)
						x.Check2(ctx.buf.Write(jsonNull))
					} else if typ.Nullable() {
						ctx.buf.Truncate(keyEndPos)
						x.Check2(ctx.buf.Write(jsonNull))
						// set nullWritten to true so we don't write closing ] for this list
						nullWritten = true
						// skip all data for the current list selection
						attrId := enc.idForAttr(curSelection.DgraphAlias())
						for next != nil && enc.getAttr(next) == attrId {
							cur = next
							next = next.next
						}
						// just set the child to point to the data for last item in the list and not
						// the data for next field in the selection set as child would anyways be
						// moved forward later.
						child = cur
					} else {
						// this is the case of [T!]!, where we can't write null either for a
						// list item or the list itself. So, mark the encoding as failed,
						// and let the parent handle null writing.
						return false
					}
				}
				// we need to iterate to the next fastJson node because we have used the data from
				// the current fastJson node.
				child = child.next
			} else if !curSelectionIsList && (!enc.getList(cur) || (fjIsRoot && (next == nil || enc.
				getAttr(cur) != enc.getAttr(next)) && !curSelection.Type().IsAggregateResult())) {
				// handles case 4
				// Root fastJson node's children contain the results for top level GraphQL queries.
				// They are marked as list during fastJson node pre-processing even though they
				// may not be list. So, we also need to consider such nodes if they actually have
				// only one value and the current selection is not an aggregate field.

				if !enc.encodeGraphQL(ctx, cur, false, curSelection.SelectionSet(), curSelection,
					append(parentPath, curSelection.ResponseName())) {
					if nullWritten = writeGraphQLNull(curSelection, ctx.buf,
						keyEndPos); !nullWritten {
						return false
					}
				}
				// we need to iterate to the next fastJson node because we have used the data from
				// the current fastJson node.
				child = child.next
			} else if !curSelectionIsList && enc.getList(cur) && curSelection.Type().
				IsAggregateResult() {
				// handles special case of aggregate fields
				if fjIsRoot {
					// this is the case of aggregate query at root
					next = completeRootAggregateQuery(enc, ctx, cur, curSelection,
						append(parentPath, curSelection.ResponseName()))
				} else {
					// this case is of deep aggregate fields
					next = completeAggregateChildren(enc, ctx, cur, curSelection,
						append(parentPath, curSelection.ResponseName()))
				}
				child = next
			} else if !curSelectionIsList {
				// handles case 3
				ctx.gqlErrs = append(ctx.gqlErrs, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedSingleItem))
				if nullWritten = writeGraphQLNull(curSelection, ctx.buf, keyEndPos); !nullWritten {
					return false
				}
				// need to skip all data points for the current selection, as they are of no use.
				attrId := enc.idForAttr(curSelection.DgraphAlias())
				for next != nil && enc.getAttr(next) == attrId {
					next = next.next
				}
				child = next
			} else {
				// handles case 2
				ctx.gqlErrs = append(ctx.gqlErrs, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedList))
				if nullWritten = writeGraphQLNull(curSelection, ctx.buf, keyEndPos); !nullWritten {
					return false
				}
				// need to skip the only data point for the current selection, as it is of no use.
				child = child.next
			}
		}

		// Step-3: Write closing ] for JSON arrays
		if next == nil || enc.getAttr(cur) != enc.getAttr(next) {
			if curSelectionIsList && !nullWritten {
				x.Check2(ctx.buf.WriteRune(']'))
			}
			cnt = 0 // Reset the count,
			// indicating that we need to write the JSON key in next iteration.
			i++ // all the results for curSelection have been picked up,
			// so iterate to the next field in the child selection set.
		}

		// Step-4: Print comma except for the last field.
		if i < len(childSelectionSet) {
			x.Check2(ctx.buf.WriteRune(','))
		}
	}

	// We have iterated over all the useful data, and corresponding GraphQL fields.
	// But, the GraphQL query may still have some fields which haven't been iterated upon.
	// We need to encode these null valued fields appropriately with errors.
	for i < len(childSelectionSet) {
		curSelection = childSelectionSet[i]

		if skipField(curSelection, dgraphTypes, seenField) {
			i++
			// if this is the last field and shouldn't be included,
			// then need to remove comma from the buffer if one was present.
			if i == len(childSelectionSet) {
				checkAndStripComma(ctx.buf)
			}
			continue
		}

		// Step-1: Write JSON key
		writeKeyGraphQL(curSelection, ctx.buf)

		// Step-2: Write JSON value
		if curSelection.Name() == gqlSchema.Typename {
			x.Check2(ctx.buf.Write([]byte(`"` + curSelection.TypeName(dgraphTypes) + `"`)))
		} else {
			if !writeGraphQLNull(curSelection, ctx.buf, ctx.buf.Len()) {
				ctx.gqlErrs = append(ctx.gqlErrs, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedNonNull, curSelection.Name(),
					curSelection.Type()))
				return false
			}
		}

		i++ // iterate to next field
		// Step-3: Print comma except for the last field.
		if i < len(childSelectionSet) {
			x.Check2(ctx.buf.WriteRune(','))
		}
	}

	// write the closing } for the JSON object
	x.Check2(ctx.buf.WriteRune('}'))

	// encoding has successfully finished for this call to encodeGraphQL().
	// Lets return true to indicate that.
	return true
}

func skipField(f gqlSchema.Field, dgraphTypes []interface{}, seenField map[string]bool) bool {
	if f.Skip() || !f.Include() {
		return true
	}
	// If typ is an interface, and dgraphTypes contains another type, then we ignore
	// fields which don't start with that type. This would happen when multiple
	// fragments (belonging to different types) are requested within a query for an interface.

	// If the dgraphPredicate doesn't start with the typ.Name(), then this field belongs to
	// a concrete type, lets check that it has inputType as the prefix, otherwise skip it.
	if len(dgraphTypes) > 0 && !f.IncludeInterfaceField(dgraphTypes) {
		return true
	}
	// if the field has already been seen at the current level, then we need to skip it.
	// Otherwise, mark it seen.
	if seenField[f.ResponseName()] {
		return true
	}
	seenField[f.ResponseName()] = true
	return false
}

func checkAndStripComma(buf *bytes.Buffer) {
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == ',' {
		buf.Truncate(buf.Len() - 1)
	}
}

func writeGraphQLNull(f gqlSchema.Field, buf *bytes.Buffer, keyEndPos int) bool {
	buf.Truncate(keyEndPos) // truncate to make sure we write null correctly
	if f.Type().ListType() != nil {
		// We could choose to set this to null.  This is our decision, not
		// anything required by the GraphQL spec.
		//
		// However, if we query, for example, for a person's friends with
		// some restrictions, and there aren't any, is that really a case to
		// set this at null and error if the list is required?  What
		// about if a person has just been added and doesn't have any friends?
		// Doesn't seem right to add null and cause error propagation.
		//
		// Seems best if we pick [], rather than null, as the list value if
		// there's nothing in the Dgraph result.
		x.Check2(buf.Write([]byte("[]")))
	} else if f.Type().Nullable() {
		x.Check2(buf.Write(jsonNull))
	} else {
		return false
	}
	return true
}

// completeRootAggregateQuery builds GraphQL JSON for aggregate queries at root.
// Root aggregate queries return a single object of type `TypeAggregateResult` which contains the
// aggregate properties. But, in the Dgraph results those properties are returned as a list of
// objects, each object having only one property. So we need to handle encoding root aggregate
// queries accordingly.
// Dgraph result:
// 		{
// 		  "aggregateCountry": [
// 		    {
// 		      "CountryAggregateResult.count": 3
// 		    }, {
// 		      "CountryAggregateResult.nameMin": "US1"
// 		    }, {
// 		      "CountryAggregateResult.nameMax": "US2"
// 		    }
// 		  ]
// 		}
// GraphQL Result:
// 		{
// 		  "aggregateCountry": {
// 		    "count": 3,
// 		    "nameMin": "US1",
// 		    "nameMax": "US2"
// 		  }
// 		}
// Note that there can't be the case when an aggregate property was requested in DQL and not
// returned by Dgraph because aggregate properties are calculated using math functions which
// always give some result.
// TODO:
//  * handle __typename for root & child query. Also send results for child query for null case?
//    add testcase for __typename with aggregate.
func completeRootAggregateQuery(enc *encoder, ctx *graphQLEncodingCtx, fj fastJsonNode,
	query gqlSchema.Field, qryPath []interface{}) fastJsonNode {
	comma := ""

	x.Check2(ctx.buf.WriteString("{"))
	for _, f := range query.SelectionSet() {
		x.Check2(ctx.buf.WriteString(comma))
		writeKeyGraphQL(f, ctx.buf)
		val, err := enc.getScalarVal(enc.children(fj))
		if err != nil {
			ctx.gqlErrs = append(ctx.gqlErrs, f.GqlErrorf(append(qryPath, f.ResponseName()),
				err.Error()))
			// all aggregate properties are nullable, so no special checks are required
			val = jsonNull
		}
		fj = fj.next
		x.Check2(ctx.buf.Write(val))
		comma = ","
	}
	x.Check2(ctx.buf.WriteString("}"))

	return fj
}

// completeAggregateChildren build GraphQL JSON for aggregate fields at child levels.
// Dgraph result:
// 		{
// 		  "Country.statesAggregate": [
// 		    {
// 		      "State.name": "Calgary",
// 		      "dgraph.uid": "0x2712"
// 		    }
// 		  ],
// 		  "StateAggregateResult.count_Country.statesAggregate": 1,
// 		  "StateAggregateResult.nameMin_Country.statesAggregate": "Calgary",
// 		  "StateAggregateResult.nameMax_Country.statesAggregate": "Calgary"
// 		}
// GraphQL result:
// 		{
// 		  "statesAggregate": {
// 		    "count": 1,
// 		    "nameMin": "Calgary",
// 		    "nameMax": "Calgary"
// 		  }
// 		}
func completeAggregateChildren(enc *encoder, ctx *graphQLEncodingCtx, fj fastJsonNode,
	field gqlSchema.Field, fieldPath []interface{}) fastJsonNode {
	// first we need to skip all the nodes returned with the attr of field as they are not needed
	// in GraphQL.
	attrId := enc.getAttr(fj)
	for fj = fj.next; attrId == enc.getAttr(fj); fj = fj.next {
		// do nothing
	}

	// now fj points to a node containing data for a child of field
	comma := ""
	suffix := "_" + field.DgraphAlias()
	var val []byte
	var err error
	x.Check2(ctx.buf.WriteString("{"))
	for _, f := range field.SelectionSet() {
		x.Check2(ctx.buf.WriteString(comma))
		writeKeyGraphQL(f, ctx.buf)
		if f.DgraphAlias()+suffix == enc.attrForID(enc.getAttr(fj)) {
			val, err = enc.getScalarVal(fj)
			if err != nil {
				ctx.gqlErrs = append(ctx.gqlErrs, f.GqlErrorf(append(fieldPath, f.ResponseName()),
					err.Error()))
				// all aggregate properties are nullable, so no special checks are required
				val = jsonNull
			}
			fj = fj.next
		} else {
			val = jsonNull
		}
		x.Check2(ctx.buf.Write(val))
		comma = ","
	}
	x.Check2(ctx.buf.WriteString("}"))

	return fj
}

// skipCountAggregateChildren skips fastJson nodes returned for count field inside child aggregate
// field. This needs to be done as the results for count are returned at the same level as the
// field for which aggregation is being done. These results are returned even if the parent doesn't
// have any edges to the field for which aggregation is being done. So, we need to skip them.
func skipCountAggregateChildren(curSelection gqlSchema.Field, cur fastJsonNode) fastJsonNode {
	for _, f := range curSelection.SelectionSet() {
		if f.Name() == "count" {
			cur = cur.next
		}
	}
	return cur
}

// completeGeoObject builds a json GraphQL result object for the underlying geo type.
// Currently, it supports Point, Polygon and MultiPolygon.
func completeGeoObject(path []interface{}, field gqlSchema.Field, val map[string]interface{},
	buf *bytes.Buffer) *x.GqlError {
	coordinate, _ := val[gqlSchema.Coordinates].([]interface{})
	if coordinate == nil {
		return field.GqlErrorf(path, "missing coordinates in geojson value: %v", val)
	}

	typ, _ := val["type"].(string)
	switch typ {
	case gqlSchema.Point:
		completePoint(field, coordinate, buf)
	case gqlSchema.Polygon:
		completePolygon(field, coordinate, buf)
	case gqlSchema.MultiPolygon:
		completeMultiPolygon(field, coordinate, buf)
	default:
		return field.GqlErrorf(path, "unsupported geo type: %s", typ)
	}

	return nil
}

// completePoint takes in coordinates from dgraph response like [12.32, 123.32], and builds
// a JSON GraphQL result object for Point like { "longitude" : 12.32 , "latitude" : 123.32 }.
func completePoint(field gqlSchema.Field, coordinate []interface{}, buf *bytes.Buffer) {
	comma := ""

	x.Check2(buf.WriteRune('{'))
	for _, f := range field.SelectionSet() {
		x.Check2(buf.WriteString(comma))
		writeKeyGraphQL(f, buf)

		switch f.Name() {
		case gqlSchema.Longitude:
			x.Check2(buf.WriteString(fmt.Sprintf("%v", coordinate[0])))
		case gqlSchema.Latitude:
			x.Check2(buf.WriteString(fmt.Sprintf("%v", coordinate[1])))
		case gqlSchema.Typename:
			x.Check2(buf.WriteString(`"Point"`))
		}
		comma = ","
	}
	x.Check2(buf.WriteRune('}'))
}

// completePolygon converts the Dgraph result to GraphQL Polygon type.
// Dgraph output: coordinate: [[[22.22,11.11],[16.16,15.15],[21.21,20.2]],[[22.28,11.18],[16.18,15.18],[21.28,20.28]]]
// Graphql output: { coordinates: [ { points: [{ latitude: 11.11, longitude: 22.22}, { latitude: 15.15, longitude: 16.16} , { latitude: 20.20, longitude: 21.21} ]}, { points: [{ latitude: 11.18, longitude: 22.28}, { latitude: 15.18, longitude: 16.18} , { latitude: 20.28, longitude: 21.28}]} ] }
func completePolygon(field gqlSchema.Field, polygon []interface{}, buf *bytes.Buffer) {
	comma1 := ""

	x.Check2(buf.WriteRune('{'))
	for _, f1 := range field.SelectionSet() {
		x.Check2(buf.WriteString(comma1))
		writeKeyGraphQL(f1, buf)

		switch f1.Name() {
		case gqlSchema.Coordinates:
			x.Check2(buf.WriteRune('['))
			comma2 := ""

			for _, ring := range polygon {
				x.Check2(buf.WriteString(comma2))
				x.Check2(buf.WriteRune('{'))
				comma3 := ""

				for _, f2 := range f1.SelectionSet() {
					x.Check2(buf.WriteString(comma3))
					writeKeyGraphQL(f2, buf)

					switch f2.Name() {
					case gqlSchema.Points:
						x.Check2(buf.WriteRune('['))
						comma4 := ""

						r, _ := ring.([]interface{})
						for _, point := range r {
							x.Check2(buf.WriteString(comma4))

							p, _ := point.([]interface{})
							completePoint(f2, p, buf)

							comma4 = ","
						}
						x.Check2(buf.WriteRune(']'))
					case gqlSchema.Typename:
						x.Check2(buf.WriteString(`"PointList"`))
					}
					comma3 = ","
				}
				x.Check2(buf.WriteRune('}'))
				comma2 = ","
			}
			x.Check2(buf.WriteRune(']'))
		case gqlSchema.Typename:
			x.Check2(buf.WriteString(`"Polygon"`))
		}
		comma1 = ","
	}
	x.Check2(buf.WriteRune('}'))
}

// completeMultiPolygon converts the Dgraph result to GraphQL MultiPolygon type.
func completeMultiPolygon(field gqlSchema.Field, multiPolygon []interface{}, buf *bytes.Buffer) {
	comma1 := ""

	x.Check2(buf.WriteRune('{'))
	for _, f := range field.SelectionSet() {
		x.Check2(buf.WriteString(comma1))
		writeKeyGraphQL(f, buf)

		switch f.Name() {
		case gqlSchema.Polygons:
			x.Check2(buf.WriteRune('['))
			comma2 := ""

			for _, polygon := range multiPolygon {
				x.Check2(buf.WriteString(comma2))

				p, _ := polygon.([]interface{})
				completePolygon(f, p, buf)

				comma2 = ","
			}
			x.Check2(buf.WriteRune(']'))
		case gqlSchema.Typename:
			x.Check2(buf.WriteString(`"MultiPolygon"`))
		}
		comma1 = ","
	}
	x.Check2(buf.WriteRune('}'))
}
