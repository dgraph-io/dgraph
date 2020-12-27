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

func (enc *encoder) writeKeyGraphQL(field gqlSchema.Field, out *bytes.Buffer) error {
	if _, err := out.WriteRune('"'); err != nil {
		return err
	}
	if _, err := out.WriteString(field.ResponseName()); err != nil {
		return err
	}
	if _, err := out.WriteRune('"'); err != nil {
		return err
	}
	if _, err := out.WriteRune(':'); err != nil {
		return err
	}
	return nil
}

// TODO:
// * Non-null errors for values in a list
// * Scalar coercion
// * Geo fields
// * Aggregate fields/queries
//   * empty data in Aggregate queries
// * Password queries
func (enc *encoder) encodeGraphQL(fj fastJsonNode, out *bytes.Buffer, errs *x.GqlErrorList,
	dgraphTypeAttrId uint16, childSelectionSet []gqlSchema.Field, parentPath []interface{}) error {
	child := enc.children(fj)
	// This is a scalar value.
	if child == nil {
		val, err := enc.getScalarVal(fj)
		if err != nil {
			return err
		}
		if val == nil {
			// val would be nil only when a user query (not a field) has no results,
			// TODO: so write null only for non-list fields (like get queries).
			// List fields should always turn out to be [] in case they have no children.
			_, err = out.Write([]byte("null"))
		} else {
			_, err = out.Write(val)
		}
		return err
	}

	// This is an internal node. Write the opening { for the JSON object
	if _, err := out.WriteRune('{'); err != nil {
		return err
	}

	// if GraphQL layer requested dgraph.type predicate, then it would always be the first child in
	// the response as it is always written first in DQL query. So, if we get data for dgraph.type
	// predicate then just save it in dgraphTypes slice, no need to write it to JSON yet.
	var dgraphTypes []interface{}
	for enc.getAttr(child) == dgraphTypeAttrId {
		val, err := enc.getScalarVal(child) // val is a quoted string like: "Human"
		if err != nil {
			return err
		}
		typeStr := string(val)
		typeStr = typeStr[1 : len(typeStr)-1] // remove `"` from beginning and end
		dgraphTypes = append(dgraphTypes, typeStr)

		child = child.next
		if child == nil {
			break
		}
	}

	cnt := 0                   // used to figure out how many times continuously we have seen the current attr
	i := 0                     // used to iterate over childSelectionSet
	var cur, next fastJsonNode // used to iterate over data in fastJson nodes

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
		curSelection := childSelectionSet[i]
		cur = child
		next = cur.next

		if skipField(curSelection, dgraphTypes) {
			i++
			// if this is the last field and shouldn't be included,
			// then need to remove comma from the buffer if one was present.
			if i == len(childSelectionSet) {
				checkAndStripComma(out)
			}
			// also need to skip all the data for this field
			attrId := enc.idForAttr(curSelection.DgraphAlias())
			for next != nil && enc.getAttr(next) == attrId {
				next = next.next
			}
			child = next
			continue
		}

		// Step-1: Write JSON key and opening [ for JSON arrays
		if cnt == 1 {
			if err := enc.writeKeyGraphQL(curSelection, out); err != nil {
				return err
			}
			if curSelection.Type().ListType() != nil {
				if _, err := out.WriteRune('['); err != nil {
					return err
				}
			}
		}

		// Step-2: Write JSON value
		if curSelection.Name() == gqlSchema.Typename {
			// If the current selection is __typename then we find out the typename from the
			// dgraphTypes slice saved earlier.
			if _, err := out.Write([]byte(`"` + curSelection.TypeName(dgraphTypes) + `"`)); err != nil {
				return err
			}
			// We don't need to iterate to next fastJson node in this case,
			// as the current node will have data for the next field in the selection set.
		} else if curSelection.DgraphAlias() != enc.attrForID(enc.getAttr(cur)) {
			// TODO: use the correct alias
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
			//    and the end case would always be ignored by this for loop.
			// So, we don't have a need to handle case 2, and need to always write null with
			// appropriate errors.
			errsLen := len(*errs)
			if err := writeGraphQLNull(curSelection, out, parentPath, errs); err != nil {
				return err
			}
			// we should propagate non-null errors to parent as soon as possible
			if len(*errs) > errsLen {
				return nil
			}
			// we don't need to iterate to next fastJson node here.
		} else {
			// This is the case where the current fastJson node holds data for the current
			// GraphQL selection. Just recursively encode it.
			errsLen := len(*errs)
			outLen := out.Len()
			if err := enc.encodeGraphQL(cur, out, errs, dgraphTypeAttrId,
				curSelection.SelectionSet(), append(parentPath,
					curSelection.ResponseName())); err != nil {
				return err
			}
			if len(*errs) > errsLen {
				out.Truncate(outLen)
				errsLen = len(*errs)
				if err := writeGraphQLNull(curSelection, out, parentPath, errs); err != nil {
					return err
				}
				// we should propagate non-null errors to parent as soon as possible
				if len(*errs) > errsLen {
					return nil
				}
			}
			// we need to iterate to the next fastJson node because we have used the data from
			// the current fastJson node.
			child = child.next
		}

		// Step-3: Write closing ] for JSON arrays
		if next == nil || enc.getAttr(cur) != enc.getAttr(next) {
			if curSelection.Type().ListType() != nil {
				if _, err := out.WriteRune(']'); err != nil {
					return err
				}
			}
			cnt = 0 // Reset the count,
			// indicating that we need to write the JSON key in next iteration.
			i++ // all the results for curSelection have been picked up,
			// so iterate to the next field in the child selection set.
		}

		// Step-4: Print comma except for the last field.
		if i < len(childSelectionSet) {
			if _, err := out.WriteRune(','); err != nil {
				return err
			}
		}
	}

	// We have iterated over all the useful data, and corresponding GraphQL fields.
	// But, the GraphQL query may still have some fields which haven't been iterated upon.
	// We need to encode these null valued fields appropriately with errors.
	for i < len(childSelectionSet) {
		f := childSelectionSet[i]

		if skipField(f, dgraphTypes) {
			i++
			// if this is the last field and shouldn't be included,
			// then need to remove comma from the buffer if one was present.
			if i == len(childSelectionSet) {
				checkAndStripComma(out)
			}
			continue
		}

		// Step-1: Write JSON key
		if err := enc.writeKeyGraphQL(f, out); err != nil {
			return err
		}

		// Step-2: Write JSON value
		if f.Name() == gqlSchema.Typename {
			if _, err := out.Write([]byte(`"` + f.TypeName(dgraphTypes) + `"`)); err != nil {
				return err
			}
		} else {
			errsLen := len(*errs)
			if err := writeGraphQLNull(f, out, parentPath, errs); err != nil {
				return err
			}
			// we should propagate non-null errors to parent as soon as possible
			if len(*errs) > errsLen {
				return nil
			}
		}

		i++ // iterate to next field
		// Step-3: Print comma except for the last field.
		if i < len(childSelectionSet) {
			if _, err := out.WriteRune(','); err != nil {
				return err
			}
		}
	}

	// write the closing } for the JSON object
	if _, err := out.WriteRune('}'); err != nil {
		return err
	}

	return nil
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

func writeGraphQLNull(f gqlSchema.Field, out *bytes.Buffer, parentPath []interface{},
	errList *x.GqlErrorList) error {
	if f.Type().ListType() != nil {
		// We could choose to set this to null.  This is our decision, not
		// anything required by the GraphQL spec.
		//
		// However, if we query, for example, for a person's friends with
		// some restrictions, and there aren't any, is that really a case to
		// set this at null and error if the list is required?  What
		// about if an person has just been added and doesn't have any friends?
		// Doesn't seem right to add null and cause error propagation.
		//
		// Seems best if we pick [], rather than null, as the list value if
		// there's nothing in the Dgraph result.
		if _, err := out.Write([]byte("[]")); err != nil {
			return err
		}
	} else if f.Type().Nullable() {
		if _, err := out.Write([]byte("null")); err != nil {
			return err
		}
	} else {
		*errList = append(*errList, f.GqlErrorf(parentPath, gqlSchema.ErrExpectedNonNull,
			f.Name(), f.Type()))
	}
	return nil
}
