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
	children []gqlSchema.Field) error {
	child := enc.children(fj)
	// This is a scalar value.
	if child == nil {
		val, err := enc.getScalarVal(fj)
		if err != nil {
			return err
		}
		if val == nil {
			// this would only be the case when the query has no results,
			// TODO: so write null only for non-list fields (get queries).
			//List fields would anyways turn out to be [].
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
	cnt := 0
	i := 0
	var cur, next fastJsonNode
	for child != nil {
		cnt++
		curField := children[i]
		validNext := false
		cur = child
		if cur.next != nil {
			next = cur.next
			validNext = true
		}

		if validNext && enc.getAttr(cur) == enc.getAttr(next) {
			if cnt == 1 {
				if err := enc.writeKeyGraphQL(curField, out); err != nil {
					return err
				}
				if _, err := out.WriteRune('['); err != nil {
					return err
				}
			}
			if err := enc.encodeGraphQL(cur, out, curField.SelectionSet()); err != nil {
				return err
			}
		} else {
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
			if err := enc.encodeGraphQL(cur, out, curField.SelectionSet()); err != nil {
				return err
			}
			if cnt > 1 || curField.Type().ListType() != nil {
				if _, err := out.WriteRune(']'); err != nil {
					return err
				}
			}
			cnt = 0 // Reset the count.
			i++     // all the results for curField have been picked up,
			// so pick the next field in next iteration
		}
		// We need to print comma except for the last attribute.
		if child.next != nil {
			if _, err := out.WriteRune(','); err != nil {
				return err
			}
		}

		child = child.next
	}
	if _, err := out.WriteRune('}'); err != nil {
		return err
	}

	return nil
}
