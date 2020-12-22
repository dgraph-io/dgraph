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

func (enc *encoder) encodeGraphQL(fj fastJsonNode, out *bytes.Buffer,
	children []gqlSchema.Field, parentPath []interface{}) error {
	var errs x.GqlErrorList
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

	// This is an internal node.
	if _, err := out.WriteRune('{'); err != nil {
		return err
	}

	// if GraphQL layer requested dgraph.type predicate, then it would always be the first child in
	// the response as it is always written first in DQL query. So, if we get data for dgraph.type
	// predicate then just save it in dgraphTypes slice, no need to write it to JSON yet.
	var dgraphTypes []interface{}
	for enc.attrForID(enc.getAttr(child)) == "dgraph.type" {
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

	cnt := 0
	i := 0
	var cur, next fastJsonNode
	for child != nil && i < len(children) {
		cnt++
		curField := children[i]
		validNext := false
		cur = child
		if cur.next != nil {
			next = cur.next
			validNext = true
		}

		// write JSON key and opening [ for JSON arrays
		if cnt == 1 {
			if err := enc.writeKeyGraphQL(curField, out); err != nil {
				return err
			}
			if curField.Type().ListType() != nil {
				if _, err := out.WriteRune('['); err != nil {
					return err
				}
			}
		}

		// write JSON value
		if curField.Name() == gqlSchema.Typename {
			if _, err := out.Write([]byte(`"` + curField.TypeName(dgraphTypes) + `"`)); err != nil {
				return err
			}
		} else if curField.DgraphAlias() != enc.attrForID(enc.getAttr(cur)) {
			if validNext && curField.DgraphAlias() == enc.attrForID(enc.getAttr(next)) {
				i--
			} else {
				out.Write([]byte("null"))
			}
			child = child.next
		} else {
			if err := enc.encodeGraphQL(cur, out, curField.SelectionSet(),
				append(parentPath, curField.ResponseName())); err != nil {
				return err
			}
			// iterate to the next child only when we have used cur to write JSON
			child = child.next
		}

		// write closing ] for JSON arrays
		if !(validNext && enc.getAttr(cur) == enc.getAttr(next)) {
			if curField.Type().ListType() != nil {
				if _, err := out.WriteRune(']'); err != nil {
					return err
				}
			}
			cnt = 0 // Reset the count.
			i++     // all the results for curField have been picked up,
			// so iterate to the next field
		}

		// print comma except for the last field.
		if i < len(children) {
			if _, err := out.WriteRune(','); err != nil {
				return err
			}
		}
	}

	// We have iterated over all the data, and corresponding GraphQL fields.
	// But, the GraphQL query may still have some fields which haven't been iterated upon.
	// We need to encode these fields appropriately.
	for i < len(children) {
		// TODO: first check whether this field should be included, same for the above block.
		// TODO: if this is the last field and shouldn't be included, remove the comma.
		field := children[i]

		// write JSON key
		if err := enc.writeKeyGraphQL(field, out); err != nil {
			return err
		}

		// write JSON value
		if field.Name() == gqlSchema.Typename {
			if _, err := out.Write([]byte(`"` + field.TypeName(dgraphTypes) + `"`)); err != nil {
				return err
			}
		} else {
			if field.Type().ListType() != nil {
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
			} else if field.Type().Nullable() {
				if _, err := out.Write([]byte("null")); err != nil {
					return err
				}
			} else {
				errs = append(errs, field.BuildError(append(parentPath, field.ResponseName()),
					gqlSchema.ErrExpectedNonNull, field.Name(), field.Type()))
			}
		}

		i++
		// print comma except for the last field.
		if i < len(children) {
			if _, err := out.WriteRune(','); err != nil {
				return err
			}
		}
	}

	if _, err := out.WriteRune('}'); err != nil {
		return err
	}

	if errs == nil {
		return nil
	}
	return errs
}
