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

	"github.com/dgraph-io/dgraph/x"

	gqlSchema "github.com/dgraph-io/dgraph/graphql/schema"
)

// ToGraphQLJson builds the data and error sub-parts of the JSON response returned from the GraphQL
// layer for the given field.
//func ToGraphQLJson(l *Latency, gqlField gqlSchema.Field, sgl []*SubGraph) ([]byte, x.GqlErrorList) {
//	// There can only be one subgraph with same name as the gqlField,
//	// find that and build JSON for it.
//	dgAlias := gqlField.DgraphAlias()
//	for _, sg := range sgl {
//		if sg.fieldName() == dgAlias {
//			return sg.toFastGraphQLJson(l, gqlField)
//		}
//	}
//	// if can't find the subgraph corresponding to gqlField,
//	// then return a null data response for the given field with appropriate errors.
//	// TODO: return non-nullable errors as required
//	return []byte("null"), nil
//}
//
//func (sg *SubGraph) toFastGraphQLJson(l *Latency, field gqlSchema.Field) ([]byte, x.GqlErrorList) {
//	encodingStart := time.Now()
//	defer func() {
//		l.Json = time.Since(encodingStart)
//	}()
//
//	enc := newEncoder()
//	defer func() {
//		// Put encoder's arena back to arena pool.
//		arenaPool.Put(enc.arena)
//		enc.alloc.Release()
//	}()
//
//	var err error
//	n := enc.newNode(enc.idForAttr("_root_"))
//	for _, sg := range sg.Children {
//		err = processNodeUids(n, enc, sg)
//		if err != nil {
//			return []byte("null"), x.GqlErrorList{field.BuildError(err.Error(), []interface{}{field.ResponseName()})}
//		}
//	}
//	enc.fixOrder(n)
//
//	var bufw bytes.Buffer
//	if enc.children(n) == nil {
//		return []byte("null"), nil
//	} else {
//		if err := enc.encode(n, &bufw); err != nil {
//			return []byte("null"), x.GqlErrorList{field.BuildError(err.Error(), []interface{}{field.ResponseName()})}
//		}
//	}
//
//	// Return error if encoded buffer size exceeds than a threshold size.
//	if uint64(bufw.Len()) > maxEncodedSize {
//		return []byte("null"), x.GqlErrorList{field.BuildError(fmt.Sprintf(
//			"while writing to buffer. Encoded response size: %d"+
//				" is bigger than threshold: %d", bufw.Len(), maxEncodedSize),
//			[]interface{}{field.ResponseName()})}
//	}
//
//	return bufw.Bytes(), nil
//}

func (enc *encoder) writeKeyGraphQL(field gqlSchema.Field, out *bytes.Buffer) {
	x.Check2(out.WriteRune('"'))
	x.Check2(out.WriteString(field.ResponseName()))
	x.Check2(out.WriteString(`":`))
}

// TODO:
//  * change query rewriting for scalar fields asked multiple times
//  * Scalar coercion
//  * Geo fields
//  * Aggregate fields/queries
//    * empty data in Aggregate queries
//  * Password queries
func (enc *encoder) encodeGraphQL(fj fastJsonNode, out *bytes.Buffer, errList *x.GqlErrorList,
	dgraphTypeAttrId uint16, childSelectionSet []gqlSchema.Field,
	parentField gqlSchema.Field, parentPath []interface{}) bool {
	child := enc.children(fj)
	// This is a scalar value.
	if child == nil {
		val, err := enc.getScalarVal(fj)
		if err != nil {
			*errList = append(*errList, parentField.GqlErrorf(parentPath, err.Error()))
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
			// Note that we don't need to add any errors to the errList here.
			if parentField.Type().ListType() != nil {
				return true
			}
			return false
		} else {
			x.Check2(out.Write(val))
		}
		// we have successfully written the scalar value, lets return true to indicate that this
		// call to encodeGraphQL() was successful.
		return true
	}

	// if we are here, ensure that GraphQL was expecting an object, otherwise return error.
	if len(childSelectionSet) == 0 {
		*errList = append(*errList, parentField.GqlErrorf(parentPath, gqlSchema.ErrExpectedScalar))
		// return false so that the caller can appropriately handle null writing.
		return false
	}

	// This is an internal node. Write the opening { for the JSON object
	x.Check2(out.WriteRune('{'))

	// if GraphQL layer requested dgraph.type predicate, then it would always be the first child in
	// the response as it is always written first in DQL query. So, if we get data for dgraph.type
	// predicate then just save it in dgraphTypes slice, no need to write it to JSON yet.
	var dgraphTypes []interface{}
	for enc.getAttr(child) == dgraphTypeAttrId {
		val, err := enc.getScalarVal(child) // val is a quoted string like: "Human"
		if err != nil {
			// TODO: correctly format error, it should be on __typename field if present?
			*errList = append(*errList, x.GqlErrorf(err.Error()).WithPath(parentPath))
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

		if skipField(curSelection, dgraphTypes) {
			cnt = 0 // Reset the count,
			// indicating that we need to write the JSON key in next iteration.
			i++
			// if this is the last field and shouldn't be included,
			// then need to remove comma from the buffer if one was present.
			if i == len(childSelectionSet) {
				checkAndStripComma(out)
			}
			// also if there was any data for this field, need to skip that
			// there may not be data in case this field was added from a fragment.
			attrId := enc.idForAttr(curSelection.DgraphAlias())
			if enc.getAttr(cur) == attrId {
				for next != nil && enc.getAttr(next) == attrId {
					next = next.next
				}
				child = next
			}
			continue
		}

		// Step-1: Write JSON key and opening [ for JSON arrays
		if cnt == 1 {
			enc.writeKeyGraphQL(curSelection, out)
			keyEndPos = out.Len()
			curSelectionIsList = curSelection.Type().ListType() != nil
			if curSelectionIsList {
				x.Check2(out.WriteRune('['))
			}
		}

		// Step-2: Write JSON value
		if curSelection.Name() == gqlSchema.Typename {
			// If the current selection is __typename then we find out the typename using the
			// dgraphTypes slice saved earlier.
			x.Check2(out.Write([]byte(`"` + curSelection.TypeName(dgraphTypes) + `"`)))
			// We don't need to iterate to next fastJson node in this case,
			// as the current node will have data for the next field in the selection set.
		} else if curSelection.DgraphAlias() != enc.attrForID(enc.getAttr(cur)) {
			// TODO: use the correct alias everywhere
			// if the current fastJson node doesn't hold data for the current GraphQL selection,
			// then there can be two cases:
			// 1. The current fastJson node holds data for a next selection and there was no data
			//    present for the current GraphQL selection, so need to write null for the current
			//    GraphQL selection with appropriate errors.
			// 2. The current fastJson node holds data which wasn't requested by any GraphQL
			//    selection, but instead by a DQL selection added by GraphQL layer; and the data
			//    for current selection may be present in an upcoming fastJson node.
			//    Point to note is that this case doesn't happen as the GraphQL layer adds such
			//    DQL selections only at the beginning (dgraph.type) or end (dgraph.uid: uid) of a
			//    DQL selection set, but not in middle. The beginning case we have already handled,
			//    and the end case would either be ignored by this for loop or handled as case 1.
			// So, we don't have a need to handle case 2, and need to always write null with
			// appropriate errors.
			if nullWritten = writeGraphQLNull(curSelection, out, keyEndPos); !nullWritten {
				*errList = append(*errList, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedNonNull, curSelection.Name(),
					curSelection.Type()))
				return false
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
			if curSelectionIsList && enc.getList(cur) {
				// handles case 1
				itemPos := out.Len()
				// List items which are scalars will never have null as a value returned
				// from Dgraph, but there can be coercion errors due to which their encoding
				// may return false and we will need to write null as a value for them.
				// Similarly, List items which are objects will also not have null as a
				// value returned from Dgraph, but there can be a nested non-nullable field
				// which may trigger the object to turn out to be null.
				if !enc.encodeGraphQL(cur, out, errList, dgraphTypeAttrId,
					curSelection.SelectionSet(), curSelection, append(parentPath,
						curSelection.ResponseName(), cnt-1)) {
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
						out.Truncate(itemPos)
						x.Check2(out.WriteString("null"))
					} else if typ.Nullable() {
						out.Truncate(keyEndPos)
						x.Check2(out.WriteString("null"))
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
			} else if !curSelectionIsList && (!enc.getList(cur) || enc.getAttr(fj) == enc.
				idForAttr("_root_")) {
				// handles case 4
				if !enc.encodeGraphQL(cur, out, errList, dgraphTypeAttrId,
					curSelection.SelectionSet(), curSelection, append(parentPath,
						curSelection.ResponseName())) {
					if nullWritten = writeGraphQLNull(curSelection, out, keyEndPos); !nullWritten {
						return false
					}
				}
				// we need to iterate to the next fastJson node because we have used the data from
				// the current fastJson node.
				child = child.next
			} else if !curSelectionIsList {
				// handles case 3
				*errList = append(*errList, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedSingleItem))
				if nullWritten = writeGraphQLNull(curSelection, out, keyEndPos); !nullWritten {
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
				*errList = append(*errList, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedList))
				if nullWritten = writeGraphQLNull(curSelection, out, keyEndPos); !nullWritten {
					return false
				}
				// need to skip the only data point for the current selection, as it is of no use.
				child = child.next
			}
		}

		// Step-3: Write closing ] for JSON arrays
		if next == nil || enc.getAttr(cur) != enc.getAttr(next) {
			if curSelectionIsList && !nullWritten {
				x.Check2(out.WriteRune(']'))
			}
			cnt = 0 // Reset the count,
			// indicating that we need to write the JSON key in next iteration.
			i++ // all the results for curSelection have been picked up,
			// so iterate to the next field in the child selection set.
		}

		// Step-4: Print comma except for the last field.
		if i < len(childSelectionSet) {
			x.Check2(out.WriteRune(','))
		}
	}

	// We have iterated over all the useful data, and corresponding GraphQL fields.
	// But, the GraphQL query may still have some fields which haven't been iterated upon.
	// We need to encode these null valued fields appropriately with errors.
	for i < len(childSelectionSet) {
		curSelection = childSelectionSet[i]

		if skipField(curSelection, dgraphTypes) {
			i++
			// if this is the last field and shouldn't be included,
			// then need to remove comma from the buffer if one was present.
			if i == len(childSelectionSet) {
				checkAndStripComma(out)
			}
			continue
		}

		// Step-1: Write JSON key
		enc.writeKeyGraphQL(curSelection, out)

		// Step-2: Write JSON value
		if curSelection.Name() == gqlSchema.Typename {
			x.Check2(out.Write([]byte(`"` + curSelection.TypeName(dgraphTypes) + `"`)))
		} else {
			if !writeGraphQLNull(curSelection, out, out.Len()) {
				*errList = append(*errList, curSelection.GqlErrorf(append(parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedNonNull, curSelection.Name(),
					curSelection.Type()))
				return false
			}
		}

		i++ // iterate to next field
		// Step-3: Print comma except for the last field.
		if i < len(childSelectionSet) {
			x.Check2(out.WriteRune(','))
		}
	}

	// write the closing } for the JSON object
	x.Check2(out.WriteRune('}'))

	// encoding has successfully finished for this call to encodeGraphQL().
	// Lets return true to indicate that.
	return true
}

// TODO: add seenField logic
func skipField(f gqlSchema.Field, dgraphTypes []interface{}) bool {
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
	return false
}

func checkAndStripComma(buf *bytes.Buffer) {
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == ',' {
		buf.Truncate(buf.Len() - 1)
	}
}

func writeGraphQLNull(f gqlSchema.Field, out *bytes.Buffer, keyEndPos int) bool {
	out.Truncate(keyEndPos) // truncate to make sure we write null correctly
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
		x.Check2(out.Write([]byte("[]")))
	} else if f.Type().Nullable() {
		x.Check2(out.Write([]byte("null")))
	} else {
		return false
	}
	return true
}
