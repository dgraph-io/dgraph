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

package query

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	gqlSchema "github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

// graphQLEncoder is used to encode JSON response for GraphQL queries.
type graphQLEncoder struct {
	*encoder
	// ctx stores GraphQL authorization information
	ctx context.Context
	// errs stores GraphQL errors, if any. Even if there are GraphQL errors,
	// the buffer will always have a valid JSON response.
	errs x.GqlErrorList
	// typeAttrId stores the id for attr `dgraph.type`.
	// dgraph.type predicate is requested by GraphQL layer for abstract types. We are caching
	// the id for this attr here so that instead of doing this:
	// 		enc.attrForID(enc.getAttr(child)) == "dgraph.type"
	// we would now be able to do just this:
	// 		enc.getAttr(child) == typeAttrId
	// Meaning, instead of looking up an int in a map and then comparing strings,
	// we can just compare ints directly.
	typeAttrId uint16
	// errCh is used to process the errors resulting from custom field resolution.
	// It simply appends all those errors to errs.
	errCh chan x.GqlErrorList
	// customFieldResultCh is used to process the fastJson tree updates resulting from custom
	// field resolution
	customFieldResultCh chan customFieldResult
	// entityRepresentations stores the representations for the `_entities` query
	entityRepresentations *gqlSchema.EntityRepresentations
}

// customFieldResult represents the fastJson tree updates for custom fields.
type customFieldResult struct {
	// parents are all the parents which have the same resolved value for the custom childField
	parents []fastJsonNode
	// childField is the custom field which has been resolved
	childField gqlSchema.Field
	// childVal is the result of resolving the custom childField from remote HTTP endpoint.
	// A child node is attached to all the parents with this value.
	childVal []byte
}

// encodeInput represents the input required for the encode call.
type encodeInput struct {
	parentField gqlSchema.Field   // parentField is the parent of all the fields in childSelSet
	parentPath  []interface{}     // parentPath is the path for parent field in JSON response
	fj          fastJsonNode      // fj is the fastJson node corresponding to parentField
	fjIsRoot    bool              // fjIsRoot tells whether fj is the root fastJson node or not
	childSelSet []gqlSchema.Field // childSelSet contains all the fields in the selection set of
	// parentField. Data corresponding to childSelSet will be present in fj's children.
}

// newGraphQLEncoder returns a ready to use graphQLEncoder, given the context and a base encoder
func newGraphQLEncoder(ctx context.Context, enc *encoder) *graphQLEncoder {
	return &graphQLEncoder{
		encoder:    enc,
		ctx:        ctx,
		typeAttrId: enc.idForAttr("dgraph.type"),
	}
}

// encode creates a JSON encoded GraphQL response.
func (genc *graphQLEncoder) encode(encInp encodeInput) bool {
	child := genc.children(encInp.fj)
	// This is a scalar value for DQL.
	if child == nil {
		val, err := genc.getScalarVal(encInp.fj)
		if err != nil {
			genc.errs = append(genc.errs, encInp.parentField.GqlErrorf(encInp.parentPath,
				err.Error()))
			// return false so that the caller can appropriately handle null writing.
			return false
		}
		if val == nil {
			// val being nil here can only be the case for a top-level query and not for a nested
			// field. val being nil indicates that the top-level query has no value to resolve
			// to, and we need to write null/[]/raise an error depending on the return type of the
			// query. Now, for queries which return a list (whether nullable or not), [] would
			// anyways be written by the parent encode() call. If we return false from here,
			// then too the parent encode() call will write [], but then we won't be able to
			// distinguish between whether the first item of the list was null or the whole query
			// had no results.
			// So, for lists lets return true.
			// We will return false for single valued cases so that the caller can correctly write
			// null or raise an error.
			// Note that we don't need to add any errors to the errs here.
			return encInp.parentField.Type().ListType() != nil
		}

		// here we have a valid value, lets write it to buffer appropriately.
		if encInp.parentField.Type().IsGeo() {
			var geoVal map[string]interface{}
			x.Check(json.Unmarshal(val, &geoVal)) // this unmarshal can't error
			if err := completeGeoObject(encInp.parentPath, encInp.parentField, geoVal,
				genc.buf); err != nil {
				genc.errs = append(genc.errs, err)
				return false
			}
		} else {
			// we got a GraphQL scalar
			// check coercion rules to see if it matches the GraphQL spec requirements.
			if cantCoerceScalar(val, encInp.parentField) {
				genc.errs = append(genc.errs, encInp.parentField.GqlErrorf(encInp.parentPath,
					"Error coercing value '%s' for field '%s' to type %s.",
					string(val), encInp.parentField.Name(), encInp.parentField.Type().Name()))
				// if it can't be coerced, return false so that the caller can appropriately
				// handle null writing
				return false
			}
			x.Check2(genc.buf.Write(val))
		}
		// we have successfully written the value, lets return true to indicate that this
		// call to encode() was successful.
		return true
	}

	// if we are here, ensure that GraphQL was expecting an object, otherwise return error.
	if len(encInp.childSelSet) == 0 {
		genc.errs = append(genc.errs, encInp.parentField.GqlErrorf(encInp.parentPath,
			gqlSchema.ErrExpectedScalar))
		// return false so that the caller can appropriately handle null writing.
		return false
	}

	// If the parent field had any immediate @custom(http: {...}) children, then we need to
	// find the custom fastJson nodes which should be used for encoding those custom fields.
	// The custom fastJson nodes will always be at the start of the list.
	var customNodes map[uint16]fastJsonNode
	if genc.getCustom(child) {
		// allocate memory for the map only when there are custom nodes
		customNodes = make(map[uint16]fastJsonNode)
		for ; child != nil && genc.getCustom(child); child = child.next {
			customNodes[genc.getAttr(child)] = child
		}
	}

	// if GraphQL layer requested dgraph.type predicate, then it would always be the first child in
	// the response as it is always written first in DQL query. So, if we get data for dgraph.type
	// predicate then just save it in dgraphTypes slice, no need to write it to JSON yet.
	child, dgraphTypes := genc.extractDgraphTypes(child)

	// This is an internal node. Write the opening { for the JSON object
	x.Check2(genc.buf.WriteRune('{'))

	cnt := 0       // used to figure out how many times continuously we have seen the current attr
	i := 0         // used to iterate over childSelSet
	keyEndPos := 0 // used to store the length of output buffer at which a JSON key ends to
	// correctly write value as null, if need be.
	nullWritten := false // indicates whether null has been written as value for the current
	// selection or not. Used to figure out whether to write the closing ] for JSON arrays.
	seenField := make(map[string]bool) // seenField map keeps track of fields which have been seen
	// as part of interface to avoid double entry in the resulting response

	var curSelection gqlSchema.Field // used to store the current selection in the childSelSet
	var curSelectionIsDgList bool    // indicates whether the curSelection is list stored in Dgraph
	var cur, next fastJsonNode       // used to iterate over data in fastJson nodes

	// We need to keep iterating only if:
	// 1. There is data to be processed for the current level. AND,
	// 2. There are unprocessed fields in the childSelSet
	// These are the respective examples to consider for each case:
	// 1. Sometimes GraphQL layer requests `dgraph.uid: uid` in the rewritten DQL query as the last
	//    field at every level. This is not part of GraphQL selection set at any level,
	//    but just exists as the last field in the DQL query resulting as the last fastJsonNode
	//    child, and hence we need to ignore it so as not to put it in the user facing JSON.
	//    This is case 1 where we have data to be processed but no child left in the selection set
	//    to use it. The condition `i < len(childSelSet)` comes handy in this case.
	// 2. It may happen that a field requested in a GraphQL query, may not have any data for some
	//    nodes in the result. If such a field is the last field in the selection set or there is a
	//    set of such fields which are at the end of selection set, then we need to write null
	//    values for them with appropriate errors. This is case 2 where there are unprocessed fields
	//    but no data for them. This is handled after this for loop.
	for child != nil && i < len(encInp.childSelSet) {
		cnt++
		nullWritten = false // reset it at every iteration
		curSelection = encInp.childSelSet[i]
		cur = child
		next = cur.next

		// Step-1: Skip the field OR Write JSON key and opening [ for JSON arrays
		if cnt == 1 {
			// we need to check if the field should be skipped only when it is encountered for
			// the first time
			if curSelection.SkipField(dgraphTypes, seenField) {
				cnt = 0 // Reset the count,
				// indicating that we need to write the JSON key in next iteration.
				i++
				// if this is the last field and shouldn't be included,
				// then need to remove comma from the buffer if one was present.
				if i == len(encInp.childSelSet) {
					checkAndStripComma(genc.buf)
				}
				// also if there was any data for this field, need to skip that. There may not be
				// data in case this field was added from a fragment on another type.
				attrId := genc.idForAttr(curSelection.DgraphAlias())
				if genc.getAttr(cur) == attrId {
					for next != nil && genc.getAttr(next) == attrId {
						next = next.next
					}
					child = next
				}
				continue
			}

			// Write JSON key and opening [ for JSON arrays
			curSelection.CompleteAlias(genc.buf)
			keyEndPos = genc.buf.Len()
			curSelectionIsDgList = (curSelection.Type().ListType() != nil) && !curSelection.
				IsCustomHTTP()
			if curSelectionIsDgList {
				x.Check2(genc.buf.WriteRune('['))
			}
		}

		// Step-2: Write JSON value
		if curSelection.Name() == gqlSchema.Typename {
			// If the current selection is __typename then we find out the typename using the
			// dgraphTypes slice saved earlier.
			x.Check2(genc.buf.Write(getTypename(curSelection, dgraphTypes)))
			// We don't need to iterate to next fastJson node in this case,
			// as the current node will have data for the next field in the selection set.
		} else if curSelection.IsCustomHTTP() {
			// if the current field had @custom(http: {...}), then need to write it using
			// the customNodes mapping stored earlier.
			if !genc.writeCustomField(curSelection, customNodes, encInp.parentPath) {
				// if custom field wasn't written successfully, need to write null
				if nullWritten = writeGraphQLNull(curSelection, genc.buf,
					keyEndPos); !nullWritten {
					genc.errs = append(genc.errs, curSelection.GqlErrorf(append(
						encInp.parentPath, curSelection.ResponseName()),
						gqlSchema.ErrExpectedNonNull, curSelection.Name(), curSelection.Type()))
					return false
				}
			}
			// We don't need to iterate to next fastJson node in this case,
			// as the current node will have data for the next field in the selection set.
		} else if curSelection.DgraphAlias() != genc.attrForID(genc.getAttr(cur)) {
			// if the current fastJson node doesn't hold data for the current GraphQL selection,
			// then there can be two cases:
			// 1. The current fastJson node holds data for a next selection and there was no data
			//    present for the current GraphQL selection, so need to write null for the current
			//    GraphQL selection with appropriate errors.
			// 2. The current fastJson node holds data for count(pred), the current GraphQL
			//    selection is an aggregate field at child level and there was no data present for
			//    it. So, need to write null for the children of current GraphQL selection but also
			//    need to skip all the count(pred) fastJson nodes which were requested from within
			//    the current GraphQL selection.
			// 3. The current fastJson node holds data which wasn't requested by any GraphQL
			//    selection, but instead by a DQL selection added by GraphQL layer; and the data
			//    for current selection may be present in an upcoming fastJson node.
			//    Point to note is that this case doesn't happen as the GraphQL layer adds such
			//    DQL selections only at the beginning (dgraph.type) or end (dgraph.uid: uid) of a
			//    DQL selection set, but not in middle. The beginning case we have already handled,
			//    and the end case would either be ignored by this for loop or handled as case 1.
			// So, we don't have a need to handle case 3, and need to always write null with
			// appropriate errors.
			// TODO: once @custom(dql: "") is fixed, check if case 3 can happen for it with the new
			//  way of rewriting.

			if !encInp.fjIsRoot && curSelection.IsAggregateField() {
				// handles null writing for case 2
				child = genc.completeAggregateChildren(cur, curSelection,
					append(encInp.parentPath, curSelection.ResponseName()), true)
			} else {
				// handles null writing for case 1
				if nullWritten = writeGraphQLNull(curSelection, genc.buf,
					keyEndPos); !nullWritten {
					genc.errs = append(genc.errs, curSelection.GqlErrorf(append(
						encInp.parentPath, curSelection.ResponseName()),
						gqlSchema.ErrExpectedNonNull, curSelection.Name(), curSelection.Type()))
					return false
				}
				// we don't need to iterate to next fastJson node here.
			}
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
			if curSelectionIsDgList && genc.getList(cur) {
				// handles case 1
				itemPos := genc.buf.Len()
				// List items which are scalars will never have null as a value returned
				// from Dgraph, but there can be coercion errors due to which their encoding
				// may return false and we will need to write null as a value for them.
				// Similarly, List items which are objects will also not have null as a
				// value returned from Dgraph, but there can be a nested non-nullable field
				// which may trigger the object to turn out to be null.
				if !genc.encode(encodeInput{
					parentField: curSelection,
					parentPath:  append(encInp.parentPath, curSelection.ResponseName(), cnt-1),
					fj:          cur,
					fjIsRoot:    false,
					childSelSet: curSelection.SelectionSet(),
				}) {
					// Unlike the choice in curSelection.NullValue(), where we turn missing
					// list fields into [], the spec explicitly calls out:
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
						genc.buf.Truncate(itemPos)
						x.Check2(genc.buf.Write(gqlSchema.JsonNull))
					} else if typ.Nullable() {
						genc.buf.Truncate(keyEndPos)
						x.Check2(genc.buf.Write(gqlSchema.JsonNull))
						// set nullWritten to true so we don't write closing ] for this list
						nullWritten = true
						// skip all data for the current list selection
						attrId := genc.idForAttr(curSelection.DgraphAlias())
						for next != nil && genc.getAttr(next) == attrId {
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
			} else if !curSelectionIsDgList && (!genc.getList(cur) || (encInp.fjIsRoot &&
				(next == nil || genc.getAttr(cur) != genc.getAttr(next)) &&
				!curSelection.Type().IsAggregateResult())) {
				// handles case 4
				// Root fastJson node's children contain the results for top level GraphQL queries.
				// They are marked as list during fastJson node pre-processing even though they
				// may not be list. So, we also need to consider such nodes if they actually have
				// only one value and the current selection is not an aggregate field.

				if !genc.encode(encodeInput{
					parentField: curSelection,
					parentPath:  append(encInp.parentPath, curSelection.ResponseName()),
					fj:          cur,
					fjIsRoot:    false,
					childSelSet: curSelection.SelectionSet(),
				}) {
					if nullWritten = writeGraphQLNull(curSelection, genc.buf,
						keyEndPos); !nullWritten {
						return false
					}
				}
				// we need to iterate to the next fastJson node because we have used the data from
				// the current fastJson node.
				child = child.next
			} else if !curSelectionIsDgList && genc.getList(cur) && curSelection.Type().
				IsAggregateResult() {
				// handles special case of aggregate fields
				if encInp.fjIsRoot {
					// this is the case of aggregate query at root
					next = genc.completeRootAggregateQuery(cur, curSelection,
						append(encInp.parentPath, curSelection.ResponseName()))
				} else {
					// this case is of deep aggregate fields
					next = genc.completeAggregateChildren(cur, curSelection,
						append(encInp.parentPath, curSelection.ResponseName()), false)
				}
				child = next
			} else if !curSelectionIsDgList {
				// handles case 3
				genc.errs = append(genc.errs, curSelection.GqlErrorf(append(encInp.parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedSingleItem))
				if nullWritten = writeGraphQLNull(curSelection, genc.buf,
					keyEndPos); !nullWritten {
					return false
				}
				// need to skip all data points for the current selection, as they are of no use.
				attrId := genc.idForAttr(curSelection.DgraphAlias())
				for next != nil && genc.getAttr(next) == attrId {
					next = next.next
				}
				child = next
			} else {
				// handles case 2
				genc.errs = append(genc.errs, curSelection.GqlErrorf(append(encInp.parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedList))
				if nullWritten = writeGraphQLNull(curSelection, genc.buf,
					keyEndPos); !nullWritten {
					return false
				}
				// need to skip the only data point for the current selection, as it is of no use.
				child = child.next
			}
		}

		// Step-3: Update counters and Write closing ] for JSON arrays
		// We perform this step in any of the 4 conditions is satisfied.
		// 1. The current selection is not a Dgraph List (It's of custom type or a single JSON object)
		// 2. We are at the end of json encoding process and there is no fastjson node ahead (next == nil)
		// 3. We are at the end of list writing and the type of next fastJSON node is not equal to
		//    type of curr fastJSON node.
		// 4. The current selection set which we are encoding is not equal to the type of
		//    current fastJSON node.
		if !curSelectionIsDgList || next == nil ||
			genc.getAttr(cur) != genc.getAttr(next) ||
			curSelection.DgraphAlias() != genc.attrForID(genc.getAttr(cur)) {
			if curSelectionIsDgList && !nullWritten {
				x.Check2(genc.buf.WriteRune(']'))
			}
			cnt = 0 // Reset the count,
			// indicating that we need to write the JSON key in next iteration.
			i++ // all the results for curSelection have been picked up,
			// so iterate to the next field in the child selection set.
		}

		// Step-4: Print comma except for the last field.
		if i < len(encInp.childSelSet) {
			x.Check2(genc.buf.WriteRune(','))
		}
	}

	// We have iterated over all the useful data from Dgraph, and corresponding GraphQL fields.
	// But, the GraphQL query may still have some fields which haven't been iterated upon.
	// If there are un-iterated custom fields, then need to encode them using the data obtained
	// from fastJson nodes stored in customNodes.
	// For rest of the fields, we need to encode them as null valued fields.
	for i < len(encInp.childSelSet) {
		curSelection = encInp.childSelSet[i]

		if curSelection.SkipField(dgraphTypes, seenField) {
			i++
			// if this is the last field and shouldn't be included,
			// then need to remove comma from the buffer if one was present.
			if i == len(encInp.childSelSet) {
				checkAndStripComma(genc.buf)
			}
			continue
		}

		// Step-1: Write JSON key
		curSelection.CompleteAlias(genc.buf)

		// Step-2: Write JSON value
		if curSelection.Name() == gqlSchema.Typename {
			x.Check2(genc.buf.Write(getTypename(curSelection, dgraphTypes)))
		} else if curSelection.IsCustomHTTP() && genc.writeCustomField(curSelection, customNodes,
			encInp.parentPath) {
			// do nothing, value for field has already been written.
			// If the value weren't written, the next else would write null.
		} else {
			if !writeGraphQLNull(curSelection, genc.buf, genc.buf.Len()) {
				genc.errs = append(genc.errs, curSelection.GqlErrorf(append(encInp.parentPath,
					curSelection.ResponseName()), gqlSchema.ErrExpectedNonNull, curSelection.Name(),
					curSelection.Type()))
				return false
			}
		}

		i++ // iterate to next field
		// Step-3: Print comma except for the last field.
		if i < len(encInp.childSelSet) {
			x.Check2(genc.buf.WriteRune(','))
		}
	}

	// write the closing } for the JSON object
	x.Check2(genc.buf.WriteRune('}'))

	// encoding has successfully finished for this call to encode().
	// Lets return true to indicate that.
	return true
}

// extractDgraphTypes extracts the all values for dgraph.type predicate from the given child
// fastJson node. It returns the next fastJson node which doesn't store value for dgraph.type
// predicate along with the extracted values for dgraph.type.
func (genc *graphQLEncoder) extractDgraphTypes(child fastJsonNode) (fastJsonNode, []string) {
	var dgraphTypes []string
	for ; child != nil && genc.getAttr(child) == genc.typeAttrId; child = child.next {
		if val, err := genc.getScalarVal(child); err == nil {
			// val is a quoted string like: "Human"
			dgraphTypes = append(dgraphTypes, toString(val))
		}

	}
	return child, dgraphTypes
}

// extractRequiredFieldsData is used to extract the data of fields which are required to resolve
// a custom field from a given parentNode.
// It returns a map containing the extracted data along with the dgraph.type values for parentNode.
// The keys in the returned map correspond to the name of a required field.
// Values in the map correspond to the extracted data for a required field.
func (genc *graphQLEncoder) extractRequiredFieldsData(parentNode fastJsonNode,
	rfDefs map[string]gqlSchema.FieldDefinition) (map[string]interface{}, []string) {
	child := genc.children(parentNode)
	// first, just skip all the custom nodes
	for ; child != nil && genc.getCustom(child); child = child.next {
		// do nothing
	}
	// then, extract data for dgraph.type
	child, dgraphTypes := genc.extractDgraphTypes(child)

	// now, iterate over rest of the children of the parentNode and find out the data for
	// requiredFields. We can stop iterating as soon as we have the data for all the requiredFields.
	rfData := make(map[string]interface{})
	for fj := child; fj != nil && len(rfData) < len(rfDefs); fj = fj.next {
		// check if this node has the data for a requiredField. If yes, we need to
		// extract that in the rfData map to be used later in substitution.
		if rfDef := rfDefs[genc.attrForID(genc.getAttr(fj))]; rfDef != nil {
			// if the requiredField is of list type, then need to extract all the data for the list.
			// using enc.getList() instead of `rfDef.Type().ListType() != nil` as for custom fields
			// both have the same behaviour and enc.getList() is fast.
			if genc.getList(fj) {
				var vals []interface{}
				for ; fj.next != nil && genc.getAttr(fj.next) == genc.getAttr(fj); fj = fj.next {
					if val, err := genc.getScalarVal(fj); err == nil {
						vals = append(vals, json.RawMessage(val))
					}
				}
				// append the last list value
				if val, err := genc.getScalarVal(fj); err == nil {
					vals = append(vals, json.RawMessage(val))
				}
				rfData[rfDef.Name()] = vals
			} else {
				// this requiredField is of non-list type, need to extract the only
				// data point for this.
				if val, err := genc.getScalarVal(fj); err == nil {
					rfData[rfDef.Name()] = json.RawMessage(val)
				}
			}
		}
	}
	return rfData, dgraphTypes
}

// writeCustomField is used to write the value when the currentSelection is a custom field.
// If the current field had @custom(http: {...}), then we need to find the fastJson node which
// stores data for this field from the customNodes mapping, and use that to write the value
// for this field.
func (genc *graphQLEncoder) writeCustomField(curSelection gqlSchema.Field,
	customNodes map[uint16]fastJsonNode, parentPath []interface{}) bool {
	if cNode := customNodes[genc.idForAttr(curSelection.DgraphAlias())]; cNode != nil {
		// if we found the custom fastJson node, then directly write the value stored
		// in that, as it would have been already completed.
		val, err := genc.getScalarVal(cNode)
		if err == nil {
			x.Check2(genc.buf.Write(val))
			// return true to indicate that the field was written successfully
			return true
		}

		// if there was an error getting the value, append the error
		genc.errs = append(genc.errs, curSelection.GqlErrorf(append(parentPath,
			curSelection.ResponseName()), err.Error()))
	}

	// if no custom fastJson node was found or there was error getting the value, return false
	return false
}

func (genc *graphQLEncoder) initChildAttrId(field gqlSchema.Field) {
	for _, f := range field.SelectionSet() {
		_ = genc.idForAttr(f.DgraphAlias())
		genc.initChildAttrId(f)
	}
}

func (genc *graphQLEncoder) processCustomFields(field gqlSchema.Field, n fastJsonNode) {
	if field.HasCustomHTTPChild() {
		// initially, create attr ids for all the descendents of this field,
		// so that they don't result in race-conditions later
		genc.initChildAttrId(field)
		// TODO(abhimanyu):
		//  * benchmark the approach of using channels vs mutex to update the fastJson tree.
		//  * benchmark and find how much load should be put on HttpClient concurrently.
		//  * benchmark and find a default buffer capacity for these channels
		genc.errCh = make(chan x.GqlErrorList, 3)
		genc.customFieldResultCh = make(chan customFieldResult, 3)
		// initialize WaitGroup for the error and result channel goroutines
		wg := &sync.WaitGroup{}
		wg.Add(2)
		// keep collecting errors arising from custom field resolution until channel is closed
		go func() {
			for errs := range genc.errCh {
				genc.errs = append(genc.errs, errs...)
			}
			wg.Done()
		}()
		// keep updating the fastJson tree as long as we get updates from the channel.
		// This is the step-7 of *genc.resolveCustomField()
		go func() {
			// this would add the custom fastJson nodes in an arbitrary order. So, they may not
			// be linked in the order the custom fields are present in selection set.
			// i.e., while encoding the GraphQL response, we will have to do one of these:
			// * a linear search to find the correct fastJson node for a custom field, or
			// * first fix the order of custom fastJson nodes and then continue the encoding, or
			// * create a map from custom fastJson node attr to the custom fastJson node,
			//   so that whenever a custom field is encountered in the selection set,
			//   just use the map to find out the fastJson node for that field.
			// The last option seems better.

			// the results slice keeps all the customFieldResults in memory as is, until all the
			// custom fields aren't resolved. Once the channel is closed, it would update the
			// fastJson tree serially, so that there are no race conditions.
			results := make([]customFieldResult, 0)
			for res := range genc.customFieldResultCh {
				results = append(results, res)
			}
			for _, res := range results {
				childAttr := genc.idForAttr(res.childField.DgraphAlias())
				for _, parent := range res.parents {
					childNode, err := genc.makeCustomNode(childAttr, res.childVal)
					if err != nil {
						genc.errCh <- x.GqlErrorList{res.childField.GqlErrorf(nil, err.Error())}
						continue
					}
					childNode.next = parent.child
					parent.child = childNode
				}
			}
			wg.Done()
		}()
		// extract the representations for Apollo _entities query and store them in GraphQL encoder
		if q, ok := field.(gqlSchema.Query); ok && q.QueryType() == gqlSchema.EntitiesQuery {
			// ignore the error here, as that should have been taken care of during query rewriting
			genc.entityRepresentations, _ = q.RepresentationsArg()
		}
		// start resolving the custom fields
		genc.resolveCustomFields(field.SelectionSet(), []fastJsonNode{genc.children(n)})
		// close the error and result channels, to terminate the goroutines started above
		close(genc.errCh)
		close(genc.customFieldResultCh)
		// wait for the above goroutines to finish
		wg.Wait()
	}
}

// resolveCustomFields resolves fields with custom directive. Here is the rough algorithm that it
// follows.
//
//	queryUser {
//		name @custom
//		age
//		school {
//			name
//			children
//			class @custom {
//				name
//				numChildren
//			}
//		}
//		cars @custom {
//			name
//		}
//	}
//
// For fields with @custom directive
// 1. There would be one query sent to the remote endpoint.
// 2. In the above example, to fetch class all the school ids would be aggregated across different
// users deduplicated and then one query sent. The results would then be filled back appropriately.
//
// For fields without custom directive we recursively call resolveCustomFields and let it do the
// work.
func (genc *graphQLEncoder) resolveCustomFields(childFields []gqlSchema.Field,
	parentNodeHeads []fastJsonNode) {
	wg := &sync.WaitGroup{}
	for _, childField := range childFields {
		if childField.Skip() || !childField.Include() {
			continue
		}

		if childField.IsCustomHTTP() {
			wg.Add(1)
			go genc.resolveCustomField(childField, parentNodeHeads, wg)
		} else if childField.HasCustomHTTPChild() {
			wg.Add(1)
			go genc.resolveNestedFields(childField, parentNodeHeads, wg)
		}
	}
	// wait for all the goroutines to finish
	wg.Wait()
}

// resolveCustomField resolves the @custom childField by making an external HTTP request and then
// updates the fastJson tree with results of that HTTP request.
// It accepts the following arguments:
//   - childField: the @custom field which needs to be resolved
//   - parentNodeHeads: a list of head pointers to the parent nodes of childField
//   - wg: a wait group to signal the calling goroutine when the execution of this goroutine is
//     finished
//
// TODO:
//   - benchmark concurrency for the worker goroutines: channels vs mutexes?
//     https://medium.com/@_orcaman/when-too-much-concurrency-slows-you-down-golang-9c144ca305a
//   - worry about path in errors and how to deal with them, specially during completion step
func (genc *graphQLEncoder) resolveCustomField(childField gqlSchema.Field,
	parentNodeHeads []fastJsonNode, wg *sync.WaitGroup) {
	defer wg.Done() // signal when this goroutine finishes execution

	fconf, err := childField.CustomHTTPConfig()
	if err != nil {
		genc.errCh <- x.GqlErrorList{childField.GqlErrorf(nil, err.Error())}
		return
	}
	// for resolving a custom field, we need to carry out following steps:
	// 1: Find the requiredFields data for uniqueParents from all the parentNodes
	// 2. Construct correct URL and body using that data
	// 3. Make the request to external HTTP endpoint using the URL and body
	// 4. Decode the HTTP response
	// 5. Run GraphQL completion on the decoded HTTP response
	// 6. Create fastJson nodes which contain the completion result for this custom field for
	//    all the duplicate parents and
	// 7. Update the fastJson tree with those fastJson nodes

	var parentNodeHeadAttr uint16
	if len(parentNodeHeads) > 0 {
		parentNodeHeadAttr = genc.getAttr(parentNodeHeads[0])
	}
	isGraphqlReq := fconf.RemoteGqlQueryName != ""
	requiredFields := childField.CustomRequiredFields()

	// we need to find the ID or @id field from requiredFields as we want to make HTTP requests
	// only for unique parent nodes. That means, we send/receive less data over the network,
	// and thus minimize the network latency as much as possible.
	idFieldName := ""
	idFieldValue := ""
	for _, fieldDef := range requiredFields {
		if fieldDef.IsID() || fieldDef.HasIDDirective() {
			idFieldName = fieldDef.Name()
			break
		}
	}
	if idFieldName == "" {
		// This should not happen as we only allow custom fields which either use ID field or a
		// field with @id directive.
		genc.errCh <- x.GqlErrorList{childField.GqlErrorf(nil,
			"unable to find a required field with type ID! or @id directive for @custom field %s.",
			childField.Name())}
		return
	}

	// we don't know the number of unique parents in advance,
	// so can't allocate this list with a pre-defined size
	var uniqueParents []interface{}
	// uniqueParentIdxToIdFieldVal stores the idFieldValue for each unique rfData
	var uniqueParentIdxToIdFieldVal []string
	// parentNodes is a map from idFieldValue to all the parentNodes for that idFieldValue.
	parentNodes := make(map[string][]fastJsonNode)

	// Step-1: Find the requiredFields data for uniqueParents from all the parentNodes
	for _, parentNodeHead := range parentNodeHeads {
		// iterate over all the siblings of this parentNodeHead which have the same attr as this
		for parentNode := parentNodeHead; parentNode != nil && genc.getAttr(
			parentNode) == parentNodeHeadAttr; parentNode = parentNode.next {
			// find the data for requiredFields from parentNode
			rfData, dgraphTypes := genc.extractRequiredFieldsData(parentNode, requiredFields)

			// check if this childField needs to be included for this parent node
			if !childField.IncludeAbstractField(dgraphTypes) {
				continue
			}

			if val, _ := rfData[idFieldName].(json.RawMessage); val != nil {
				idFieldValue = string(val)
			} else {
				// this case can't happen as ID or @id fields are not list values
				continue
			}

			// let's see if this field also had @requires directive. If so, we need to get the data
			// for the fields specified in @requires from the correct object from representations
			// list argument in the _entities query and pass that data to rfData.
			// This would override any data returned for that field from dgraph.
			apolloRequiredFields := childField.ApolloRequiredFields()
			if len(apolloRequiredFields) > 0 && genc.entityRepresentations != nil {
				keyFldName := genc.entityRepresentations.KeyField.Name()
				// key fields will always have a non-list value, so it must be json.RawMessage
				keyFldVal := toString(rfData[keyFldName].(json.RawMessage))
				representation, ok := genc.entityRepresentations.KeyValToRepresentation[keyFldVal]
				if ok {
					for _, fName := range apolloRequiredFields {
						rfData[fName] = representation[fName]
					}
				}
			}

			// add rfData to uniqueParents only if we haven't encountered any parentNode before
			// with this idFieldValue
			if len(parentNodes[idFieldValue]) == 0 {
				uniqueParents = append(uniqueParents, rfData)
				uniqueParentIdxToIdFieldVal = append(uniqueParentIdxToIdFieldVal, idFieldValue)
			}
			// always add the parent node to the slice for this idFieldValue, so that we can
			// build the response for all the duplicate parents
			parentNodes[idFieldValue] = append(parentNodes[idFieldValue], parentNode)
		}
	}

	if len(uniqueParents) == 0 {
		return
	}

	switch fconf.Mode {
	case gqlSchema.SINGLE:
		// In SINGLE mode, we can consider steps 2-5 as a single isolated unit of computation,
		// which can be executed in parallel for each uniqueParent.
		// Step 6-7 can be executed in parallel to Step 2-5 in a separate goroutine to minimize
		// contention.

		// used to wait on goroutines started for each uniqueParent
		uniqueParentWg := &sync.WaitGroup{}
		// iterate over all the uniqueParents to make HTTP requests
		for i := range uniqueParents {
			uniqueParentWg.Add(1)
			go func(idx int) {
				defer uniqueParentWg.Done() // signal when this goroutine finishes execution

				// Step-2: Construct correct URL and body using the data of requiredFields
				url := fconf.URL
				var body interface{}
				if isGraphqlReq {
					// If it is a remote GraphQL request, then URL can't have variables.
					// So, we only need to construct the body.
					body = map[string]interface{}{
						"query":     fconf.RemoteGqlQuery,
						"variables": uniqueParents[idx],
					}
				} else {
					// for REST requests, we need to correctly construct both URL & body
					var err error
					url, err = gqlSchema.SubstituteVarsInURL(url,
						uniqueParents[idx].(map[string]interface{}))
					if err != nil {
						genc.errCh <- x.GqlErrorList{childField.GqlErrorf(nil,
							"Evaluation of custom field failed while substituting variables "+
								"into URL for remote endpoint with an error: %s for field: %s "+
								"within type: %s.", err, childField.Name(),
							childField.GetObjectName())}
						return
					}
					body = gqlSchema.SubstituteVarsInBody(fconf.Template,
						uniqueParents[idx].(map[string]interface{}))
				}

				// Step-3 & 4: Make the request to external HTTP endpoint using the URL and
				// body. Then, Decode the HTTP response.
				response, errs, hardErrs := fconf.MakeAndDecodeHTTPRequest(nil, url, body,
					childField)
				if hardErrs != nil {
					genc.errCh <- hardErrs
					return
				}

				// Step-5. Run GraphQL completion on the decoded HTTP response
				b, gqlErrs := gqlSchema.CompleteValue(nil, childField, response)
				errs = append(errs, gqlErrs...)

				// finally, send the fastJson tree update over the channel
				if b != nil {
					genc.customFieldResultCh <- customFieldResult{
						parents:    parentNodes[uniqueParentIdxToIdFieldVal[idx]],
						childField: childField,
						childVal:   b,
					}
				}

				// if we are here, it means the fastJson tree update was successfully sent.
				// i.e., this custom childField was successfully resolved for given parentNode.

				// now, send all the collected errors together
				genc.errCh <- errs
			}(i)
		}
		uniqueParentWg.Wait()
	case gqlSchema.BATCH:
		// In BATCH mode, we can break the above steps into following isolated units of computation:
		// a. Step 2-4
		// b. Step 5
		// c. Step 6-7
		// i.e., step-a has to be executed only once irrespective of the number of parentNodes.
		// Then, step-b can be executed in parallel for each parentNode.
		// step-c can run in parallel to step-b in a separate goroutine to minimize contention.

		// Step-2: Construct correct body for the batch request
		var body interface{}
		if isGraphqlReq {
			body = map[string]interface{}{
				"query":     fconf.RemoteGqlQuery,
				"variables": map[string]interface{}{fconf.GraphqlBatchModeArgument: uniqueParents},
			}
		} else {
			for i := range uniqueParents {
				uniqueParents[i] = gqlSchema.SubstituteVarsInBody(fconf.Template,
					uniqueParents[i].(map[string]interface{}))
			}
			if childField.HasLambdaDirective() {
				body = gqlSchema.GetBodyForLambda(genc.ctx, childField, uniqueParents, nil)
			} else {
				body = uniqueParents
			}
		}

		// Step-3 & 4: Make the request to external HTTP endpoint using the URL and
		// body. Then, Decode the HTTP response.
		response, errs, hardErrs := fconf.MakeAndDecodeHTTPRequest(nil, fconf.URL, body, childField)
		if hardErrs != nil {
			genc.errCh <- hardErrs
			return
		}
		batchedResult, ok := response.([]interface{})
		if !ok {
			genc.errCh <- append(errs, childField.GqlErrorf(nil,
				"Evaluation of custom field failed because expected result of external"+
					" BATCH request to be of list type, got: %v for field: %s within type: %s.",
				reflect.TypeOf(response).Name(), childField.Name(), childField.GetObjectName()))
			return
		}
		if len(batchedResult) != len(uniqueParents) {
			genc.errCh <- append(errs, childField.GqlErrorf(nil,
				"Evaluation of custom field failed because expected result of "+
					"external request to be of size %v, got: %v for field: %s within type: %s.",
				len(uniqueParents), len(batchedResult), childField.Name(),
				childField.GetObjectName()))
			return
		}

		batchedErrs := make([]x.GqlErrorList, len(batchedResult))
		batchedResultWg := &sync.WaitGroup{}
		for i := range batchedResult {
			batchedResultWg.Add(1)
			go func(idx int) {
				defer batchedResultWg.Done() // signal when this goroutine finishes execution
				// Step-5. Run GraphQL completion on the decoded HTTP response
				b, gqlErrs := gqlSchema.CompleteValue(nil, childField, batchedResult[idx])

				// finally, send the fastJson tree update over the channel
				if b != nil {
					genc.customFieldResultCh <- customFieldResult{
						parents:    parentNodes[uniqueParentIdxToIdFieldVal[idx]],
						childField: childField,
						childVal:   b,
					}
				}

				// set the errors obtained from completion
				batchedErrs[idx] = gqlErrs
			}(i)
		}
		batchedResultWg.Wait()

		// we are doing this just to send all the related errors together, otherwise if we directly
		// send it over the error channel, they may get spread here and there in errors.
		for _, batchedErr := range batchedErrs {
			if batchedErr != nil {
				errs = append(errs, batchedErr...)
			}
		}
		// now, send all the collected errors together
		genc.errCh <- errs
	}
}

// resolveNestedFields resolves fields which themselves don't have the @custom directive but their
// children might.
//
//	queryUser {
//		id
//		classes {
//			name @custom...
//		}
//	}
//
// In the example above, resolveNestedFields would be called on classes field and parentNodeHeads
// would be the list of head pointers for all the user fastJson nodes.
func (genc *graphQLEncoder) resolveNestedFields(childField gqlSchema.Field,
	parentNodeHeads []fastJsonNode, wg *sync.WaitGroup) {
	defer wg.Done() // signal when this goroutine finishes execution

	var childNodeHeads []fastJsonNode
	var parentNodeHeadAttr uint16
	if len(parentNodeHeads) > 0 {
		parentNodeHeadAttr = genc.getAttr(parentNodeHeads[0])
	}
	childFieldAttr := genc.idForAttr(childField.DgraphAlias())
	// iterate over all the parentNodeHeads and build the list of childNodeHeads for this childField
	for _, parentNodeHead := range parentNodeHeads {
		// iterate over all the siblings of this parentNodeHead which have the same attr as this
		for parentNode := parentNodeHead; parentNode != nil && genc.getAttr(
			parentNode) == parentNodeHeadAttr; parentNode = parentNode.next {
			// find the first child node which has data for childField
			fj := genc.children(parentNode)
			for ; fj != nil && genc.getAttr(fj) != childFieldAttr; fj = fj.next {
				// do nothing, just keep skipping unnecessary data
			}
			if fj != nil {
				// we found the first node that has data for childField,
				// add that node to the list of childNodeHeads
				childNodeHeads = append(childNodeHeads, fj)
			}
		}
	}
	// if we found some data for the child field, then only we need to
	// resolve the custom fields in the selection set of childField
	if len(childNodeHeads) > 0 {
		genc.resolveCustomFields(childField.SelectionSet(), childNodeHeads)
	}
}

// completeRootAggregateQuery builds GraphQL JSON for aggregate queries at root.
// Root aggregate queries return a single object of type `TypeAggregateResult` which contains the
// aggregate properties. But, in the Dgraph results those properties are returned as a list of
// objects, each object having only one property. So we need to handle encoding root aggregate
// queries accordingly.
// Dgraph result:
//
//	{
//	  "aggregateCountry": [
//	    {
//	      "CountryAggregateResult.count": 3
//	    }, {
//	      "CountryAggregateResult.nameMin": "US1"
//	    }, {
//	      "CountryAggregateResult.nameMax": "US2"
//	    }
//	  ]
//	}
//
// GraphQL Result:
//
//	{
//	  "aggregateCountry": {
//	    "count": 3,
//	    "nameMin": "US1",
//	    "nameMax": "US2"
//	  }
//	}
//
// Note that there can't be the case when an aggregate property was requested in DQL and not
// returned by Dgraph because aggregate properties are calculated using math functions which
// always give some result.
// But, auth queries may lead to generation of following DQL:
//
//	query {
//		aggregateCountry()
//	}
//
// which doesn't request any aggregate properties. In this case the fastJson node won't have any
// children and we just need to write null as the value of the query.
func (genc *graphQLEncoder) completeRootAggregateQuery(fj fastJsonNode, query gqlSchema.Field,
	qryPath []interface{}) fastJsonNode {
	if genc.children(fj) == nil {
		x.Check2(genc.buf.Write(gqlSchema.JsonNull))
		return fj.next
	}

	var val []byte
	var err error
	comma := ""

	x.Check2(genc.buf.WriteString("{"))
	for _, f := range query.SelectionSet() {
		if f.Skip() || !f.Include() {
			if f.Name() != gqlSchema.Typename {
				fj = fj.next // need to skip data as well for this field
			}
			continue
		}

		x.Check2(genc.buf.WriteString(comma))
		f.CompleteAlias(genc.buf)

		if f.Name() == gqlSchema.Typename {
			val = getTypename(f, nil)
		} else {
			val, err = genc.getScalarVal(genc.children(fj))
			if err != nil {
				genc.errs = append(genc.errs, f.GqlErrorf(append(qryPath,
					f.ResponseName()), err.Error()))
				// all aggregate properties are nullable, so no special checks are required
				val = gqlSchema.JsonNull
			}
			fj = fj.next
		}
		x.Check2(genc.buf.Write(val))
		comma = ","
	}
	x.Check2(genc.buf.WriteString("}"))

	return fj
}

// completeAggregateChildren build GraphQL JSON for aggregate fields at child levels.
// Dgraph result:
//
//	{
//	  "Country.statesAggregate": [
//	    {
//	      "State.name": "Calgary",
//	      "dgraph.uid": "0x2712"
//	    }
//	  ],
//	  "StateAggregateResult.count_Country.statesAggregate": 1,
//	  "StateAggregateResult.nameMin_Country.statesAggregate": "Calgary",
//	  "StateAggregateResult.nameMax_Country.statesAggregate": "Calgary"
//	}
//
// GraphQL result:
//
//	{
//	  "statesAggregate": {
//	    "count": 1,
//	    "nameMin": "Calgary",
//	    "nameMax": "Calgary"
//	  }
//	}
func (genc *graphQLEncoder) completeAggregateChildren(fj fastJsonNode,
	field gqlSchema.Field, fieldPath []interface{}, respIsNull bool) fastJsonNode {
	if !respIsNull {
		// first we need to skip all the nodes returned with the attr of field as they are not
		// needed in GraphQL.
		attrId := genc.getAttr(fj)
		for fj = fj.next; attrId == genc.getAttr(fj); fj = fj.next {
			// do nothing
		}
		// there would always be some other fastJson node after the nodes for field are skipped
		// corresponding to a selection inside field that. So, no need to check above if fj != nil.
	}

	// now fj points to a node containing data for a child of field
	comma := ""
	suffix := "_" + field.DgraphAlias()
	var val []byte
	var err error
	x.Check2(genc.buf.WriteString("{"))
	for _, f := range field.SelectionSet() {
		if f.Skip() || !f.Include() {
			if f.Name() != gqlSchema.Typename && fj != nil && f.DgraphAlias()+suffix == genc.
				attrForID(genc.getAttr(fj)) {
				fj = fj.next // if data was there, need to skip that as well for this field
			}
			continue
		}

		x.Check2(genc.buf.WriteString(comma))
		f.CompleteAlias(genc.buf)

		if f.Name() == gqlSchema.Typename {
			val = getTypename(f, nil)
		} else if fj != nil && f.DgraphAlias()+suffix == genc.attrForID(genc.getAttr(fj)) {
			val, err = genc.getScalarVal(fj)
			if err != nil {
				genc.errs = append(genc.errs, f.GqlErrorf(append(fieldPath,
					f.ResponseName()), err.Error()))
				// all aggregate properties are nullable, so no special checks are required
				val = gqlSchema.JsonNull
			}
			fj = fj.next
		} else {
			val = gqlSchema.JsonNull
		}
		x.Check2(genc.buf.Write(val))
		comma = ","
	}
	x.Check2(genc.buf.WriteString("}"))

	return fj
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
		if f.Skip() || !f.Include() {
			continue
		}

		x.Check2(buf.WriteString(comma))
		f.CompleteAlias(buf)

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
// Graphql output: { coordinates: [ { points: [{ latitude: 11.11, longitude: 22.22},
// { latitude: 15.15, longitude: 16.16} , { latitude: 20.20, longitude: 21.21} ]}, { points: [
// { latitude: 11.18, longitude: 22.28}, { latitude: 15.18, longitude: 16.18} ,
// { latitude: 20.28, longitude: 21.28}]} ] }
func completePolygon(field gqlSchema.Field, polygon []interface{}, buf *bytes.Buffer) {
	comma1 := ""

	x.Check2(buf.WriteRune('{'))
	for _, f1 := range field.SelectionSet() {
		if f1.Skip() || !f1.Include() {
			continue
		}

		x.Check2(buf.WriteString(comma1))
		f1.CompleteAlias(buf)

		switch f1.Name() {
		case gqlSchema.Coordinates:
			x.Check2(buf.WriteRune('['))
			comma2 := ""

			for _, ring := range polygon {
				x.Check2(buf.WriteString(comma2))
				x.Check2(buf.WriteRune('{'))
				comma3 := ""

				for _, f2 := range f1.SelectionSet() {
					if f2.Skip() || !f2.Include() {
						continue
					}

					x.Check2(buf.WriteString(comma3))
					f2.CompleteAlias(buf)

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
		if f.Skip() || !f.Include() {
			continue
		}

		x.Check2(buf.WriteString(comma1))
		f.CompleteAlias(buf)

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

// cantCoerceScalar tells whether a scalar value can be coerced to its corresponding GraphQL scalar.
func cantCoerceScalar(val []byte, field gqlSchema.Field) bool {
	switch field.Type().Name() {
	case "Int":
		// Although GraphQL layer would have input coercion for Int,
		// we still need to do this as there can be cases like schema migration when Int64 was
		// changed to Int, or if someone was using DQL mutations but GraphQL queries. The GraphQL
		// layer must always honor the spec.
		// valToBytes() uses []byte(strconv.FormatInt(num, 10)) to convert int values to byte slice.
		// so, we should do the reverse, parse the string back to int and check that it fits in the
		// range of int32.
		if _, err := strconv.ParseInt(string(val), 0, 32); err != nil {
			return true
		}
	case "String", "ID", "Boolean", "Int64", "Float", "DateTime":
		// do nothing, as for these types the GraphQL schema is same as the dgraph schema.
		// Hence, the value coming in from fastJson node should already be in the correct form.
		// So, no need to coerce it.
	default:
		enumValues := field.EnumValues()
		// At this point we should only get fields which are of ENUM type, so we can return
		// an error if we don't get any enum values.
		if len(enumValues) == 0 {
			return true
		}
		// Lets check that the enum value is valid.
		if !x.HasString(enumValues, toString(val)) {
			return true
		}
	}
	return false
}

// toString converts the json encoded string value val to a go string.
// It should be used only in scenarios where the underlying string is simple, i.e., it doesn't
// contain any escape sequence or any other string magic. Otherwise, better to use json.Unmarshal().
func toString(val []byte) string {
	return strings.Trim(string(val), `"`) // remove `"` from beginning and end
}

// checkAndStripComma checks whether there is a comma at the end of the given buffer. If yes,
// it removes that comma from the buffer.
func checkAndStripComma(buf *bytes.Buffer) {
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == ',' {
		buf.Truncate(buf.Len() - 1)
	}
}

// getTypename returns the JSON bytes for the __typename field, given the dgraph.type values
// extracted from dgraph response.
func getTypename(f gqlSchema.Field, dgraphTypes []string) []byte {
	return []byte(`"` + f.TypeName(dgraphTypes) + `"`)
}

// writeGraphQLNull writes null value for the given field to the buffer.
// If the field is non-nullable, it returns false, otherwise it returns true.
func writeGraphQLNull(f gqlSchema.Field, buf *bytes.Buffer, keyEndPos int) bool {
	if b := f.NullValue(); b != nil {
		buf.Truncate(keyEndPos) // truncate to make sure we write null correctly
		x.Check2(buf.Write(b))
		return true
	}
	return false
}
