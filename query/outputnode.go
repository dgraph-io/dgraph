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

package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

// ToJson converts the list of subgraph into a JSON response by calling toFastJSON.
func ToJson(l *Latency, sgl []*SubGraph) ([]byte, error) {
	sgr := &SubGraph{}
	for _, sg := range sgl {
		if sg.Params.Alias == "var" || sg.Params.Alias == "shortest" {
			continue
		}
		if sg.Params.GetUid {
			sgr.Params.GetUid = true
		}
		sgr.Children = append(sgr.Children, sg)
	}
	return sgr.toFastJSON(l)
}

// We are capping maxEncoded size to 4GB, as grpc encoding fails
// for a response size > math.MaxUint32.
const maxEncodedSize = uint64(4 << 30)

type encoder struct {
	// attrMap has mapping of string predicates to uint16 ids.
	// For each predicate one unique id is assigned to save space.
	attrMap map[string]uint16
	// idSlice contains mapping from predicate id to predicate.
	idSlice []string
	// arena is used to store scalarVal for fastJsonNodes. Offset of scalarVal inside arena buffer
	// is stored in fastJsonNode meta.
	arena *arena
	// curSize is current estimated size of the encoded response. It should be less than actual
	// response size after encoding. If curSize exceeds a threshold size(maxEncodedSize), we return
	// query response with error saying response is too big. Currently curSize tracking has been
	// kept very simple. curSize is crossing threshold value or not is only checked at leaf(scalar)
	// nodes as of now. curSize is updated in following cases:
	// 1. By adding predicate len, while expanding it for an uid in preTraverse().
	// 2. By adding scalarVal len in setScalarVal function for a leaf(scalar) node.
	// TODO(Ashish): currently we are not including facets/groupby/aggregations fields in curSize
	// for simplicity. curSize can be made more accurate by adding these fields.
	curSize uint64

	// metaSlice has meta data for all fastJsonNodes.
	// meta stores meta information for a fastJsonNode in an uint64. Layout is as follows.
	// Bytes 4-1 contains offset(uint32) for Arena.
	// Bytes 7-6 contains attr.
	// Bit MSB(first bit in Byte-8) contains list field value.
	// Bit SecondMSB(second bit in Byte-8) contains facetsParent field value.
	// Byte-5 is not getting used as of now.
	// |-----------------------------------------------------------------------|
	// |    8        |    7   |    6   |    5   |    4   |    3   |    2   |    1   |
	// |-----------------------------------------------------------------------|
	// | MSB - list  |                 | Unused |                                   |
	// | SecondMSB - |     Attr ID     | For    |        Offset inside Arena        |
	// | facetsParent|                 | Now    |                                   |
	// |-----------------------------------------------------------------------|
	metaSlice []uint64
	// childrenMap contains mapping of fastJsonNode to its children.
	childrenMap map[fastJsonNode][]fastJsonNode
}

func newEncoder() *encoder {
	// Append dummy entry, to avoid getting meta for a fastJsonNode with default value(0).
	metaSlice := make([]uint64, 1)
	idSlice := make([]string, 1)

	a := (arenaPool.Get()).(*arena)
	a.reset()

	return &encoder{
		attrMap:     make(map[string]uint16),
		idSlice:     idSlice,
		arena:       a,
		metaSlice:   metaSlice,
		childrenMap: make(map[fastJsonNode][]fastJsonNode),
	}
}

func (enc *encoder) idForAttr(attr string) uint16 {
	if id, ok := enc.attrMap[attr]; ok {
		return id
	}

	enc.idSlice = append(enc.idSlice, attr)
	enc.attrMap[attr] = uint16(len(enc.idSlice) - 1) // TODO(Ashish): check for overflow.
	return uint16(len(enc.idSlice) - 1)
}

func (enc *encoder) attrForID(id uint16) string {
	// For now we are not returning error from here.
	if id == 0 || id >= uint16(len(enc.idSlice)) {
		return ""
	}

	return enc.idSlice[id]
}

// makeScalarNode returns a fastJsonNode with all of its meta data, scalarVal populated.
func (enc *encoder) makeScalarNode(attr uint16, val []byte, list bool) (fastJsonNode, error) {
	fj := enc.newNode(attr)
	if err := enc.setScalarVal(fj, val); err != nil {
		return 0, err
	}
	enc.setList(fj, list)

	return fj, nil
}

const (
	// Value with most significant bit set to 1.
	msbBit = 0x8000000000000000
	// Value with second most significant bit set to 1.
	secondMsbBit = 0x4000000000000000
	// Value with all bits set to 1 for bytes 7 and 6.
	setBytes76 = uint64(0x00FFFF0000000000)
	// Compliment value of setBytes76.
	unsetBytes76 = uint64(^setBytes76)
	// Value with all bits set to 1 for bytes 4 to 1.
	setBytes4321 = 0x00000000FFFFFFFF
)

// fastJsonNode represents node of a tree, which is formed to convert a subgraph into json response
// for a query. A fastJsonNode has following meta data:
// 1. Attr => predicate associated with this node.
// 2. ScalarVal => Any value associated with node, if it is a leaf node.
// 3. List => Stores boolean value, true if this node is part of list.
// 4. FacetsParent => Stores boolean value, true if this node is a facetsParent. facetsParent is
//    node which is parent for facets values for a scalar list predicate. Eg: node "city|country"
//    will have FacetsParent value as true.
//    {
//		"city": ["Bengaluru", "San Francisco"],
//		"city|country": {
//			"0": "india",
//			"1": "US"
//		}
//	  }
// 5. Children(Attrs) => List of all children.
//
// All of the data for fastJsonNode tree is stored in encoder to optimise memory usage. fastJsonNode
// type only stores one uint32(can be thought of id for this node). A fastJsonNode is created in
// below steps:
// 1. Default meta(0) is appened to metaSlice of encoder and index of this meta
// 	becomes fastJsonNode value(id).
// 2. Now any meta for this node can be updated using setXXX functions.
// 3. Children for this node are store in encoder's children map.
type fastJsonNode uint32

// newNode returns a fastJsonNode with its attr set to attr,
// and all other meta set to their default value.
func (enc *encoder) newNode(attr uint16) fastJsonNode {
	enc.metaSlice = append(enc.metaSlice, 0)
	nn := fastJsonNode(len(enc.metaSlice) - 1)
	enc.setAttr(nn, attr)
	return nn
}

func (enc *encoder) setAttr(fj fastJsonNode, attr uint16) {
	meta := enc.metaSlice[fj]
	// There can be some cases where we change name of attr for fastJsoNode and
	// hence first clear the existing attr, then store new one.
	meta &= unsetBytes76
	meta |= (uint64(attr) << 40)

	enc.metaSlice[fj] = meta
}

func (enc *encoder) setScalarVal(fj fastJsonNode, sv []byte) error {
	offset, err := enc.arena.put(sv)
	if err != nil {
		return err
	}
	enc.metaSlice[fj] |= uint64(offset)

	// Also increase curSize.
	enc.curSize += uint64(len(sv))

	// check if it exceeds threshold size.
	if enc.curSize > maxEncodedSize {
		return fmt.Errorf("encoded response size: %d is bigger than threshold: %d",
			enc.curSize, maxEncodedSize)
	}

	return nil
}

func (enc *encoder) setList(fj fastJsonNode, list bool) {
	if list {
		enc.metaSlice[fj] |= msbBit
	}
}

func (enc *encoder) setFacetsParent(fj fastJsonNode) {
	enc.metaSlice[fj] |= secondMsbBit
}

// appendAttrs appends attrs to existing fj's attrs.
func (enc *encoder) appendAttrs(fj fastJsonNode, attrs ...fastJsonNode) {
	cs, ok := enc.childrenMap[fj]
	if !ok {
		cs = make([]fastJsonNode, 0, len(attrs))
	}
	cs = append(cs, attrs...)
	enc.childrenMap[fj] = cs
}

func (enc *encoder) getAttr(fj fastJsonNode) uint16 {
	meta := enc.metaSlice[fj]
	return uint16((meta & setBytes76) >> 40)
}

func (enc *encoder) getScalarVal(fj fastJsonNode) ([]byte, error) {
	meta := enc.metaSlice[fj]
	offset := uint32(meta & setBytes4321)
	return enc.arena.get(offset)
}

func (enc *encoder) getList(fj fastJsonNode) bool {
	return (enc.metaSlice[fj] & msbBit) > 0
}

func (enc *encoder) getFacetsParent(fj fastJsonNode) bool {
	return (enc.metaSlice[fj] & secondMsbBit) > 0
}

func (enc *encoder) getAttrs(fj fastJsonNode) []fastJsonNode {
	// Return nil if no attrs are found.
	return enc.childrenMap[fj]
}

func (enc *encoder) AddValue(fj fastJsonNode, attr uint16, v types.Val) error {
	return enc.AddListValue(fj, attr, v, false)
}

func (enc *encoder) AddListValue(fj fastJsonNode, attr uint16, v types.Val, list bool) error {
	bs, err := valToBytes(v)
	if err != nil {
		return nil // Ignore this.
	}
	sn, err := enc.makeScalarNode(attr, bs, list)
	if err != nil {
		return err
	}
	enc.appendAttrs(fj, sn)
	return nil
}

func (enc *encoder) AddMapChild(fj, val fastJsonNode) {
	var childNode fastJsonNode
	for _, c := range enc.getAttrs(fj) {
		if enc.getAttr(c) == enc.getAttr(val) {
			childNode = c
			break
		}
	}

	if childNode > 0 {
		enc.appendAttrs(childNode, enc.getAttrs(val)...)
	} else {
		enc.appendAttrs(fj, val)
	}
}

func (enc *encoder) AddListChild(fj, child fastJsonNode) {
	enc.setList(child, true)
	enc.appendAttrs(fj, child)
}

func (enc *encoder) SetUID(fj fastJsonNode, uid uint64, attr uint16) error {
	// if we're in debug mode, uid may be added second time, skip this
	uidAttrID := enc.idForAttr("uid")
	if attr == uidAttrID {
		fjAttrs := enc.getAttrs(fj)
		for _, a := range fjAttrs {
			if enc.getAttr(a) == attr {
				return nil
			}
		}
	}

	sn, err := enc.makeScalarNode(attr, []byte(fmt.Sprintf("\"%#x\"", uid)), false)
	if err != nil {
		return err
	}
	enc.appendAttrs(fj, sn)
	return nil
}

func (enc *encoder) IsEmpty(fj fastJsonNode) bool {
	return len(enc.getAttrs(fj)) == 0
}

var (
	boolTrue    = []byte("true")
	boolFalse   = []byte("false")
	emptyString = []byte(`""`)

	// Below variables are used in stringJsonMarshal function.
	bufferPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	hex        = "0123456789abcdef"
	escapeHTML = true
)

// stringJsonMarshal is replacement for json.Marshal() function only for string type.
// This function is encodeState.string(string, escapeHTML) in "encoding/json/encode.go".
// It should be in sync with encodeState.string function.
func stringJsonMarshal(s string) []byte {
	e := bufferPool.Get().(*bytes.Buffer)
	e.Reset()

	e.WriteByte('"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if htmlSafeSet[b] || (!escapeHTML && safeSet[b]) {
				i++
				continue
			}
			if start < i {
				e.WriteString(s[start:i])
			}
			e.WriteByte('\\')
			switch b {
			case '\\', '"':
				e.WriteByte(b)
			case '\n':
				e.WriteByte('n')
			case '\r':
				e.WriteByte('r')
			case '\t':
				e.WriteByte('t')
			default:
				// This encodes bytes < 0x20 except for \t, \n and \r.
				// If escapeHTML is set, it also escapes <, >, and &
				// because they can lead to security holes when
				// user-controlled strings are rendered into JSON
				// and served to some browsers.
				e.WriteString(`u00`)
				e.WriteByte(hex[b>>4])
				e.WriteByte(hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				e.WriteString(s[start:i])
			}
			e.WriteString(`\ufffd`)
			i += size
			start = i
			continue
		}
		// U+2028 is LINE SEPARATOR.
		// U+2029 is PARAGRAPH SEPARATOR.
		// They are both technically valid characters in JSON strings,
		// but don't work in JSONP, which has to be evaluated as JavaScript,
		// and can lead to security holes there. It is valid JSON to
		// escape them, so we do so unconditionally.
		// See http://timelessrepo.com/json-isnt-a-javascript-subset for discussion.
		if c == '\u2028' || c == '\u2029' {
			if start < i {
				e.WriteString(s[start:i])
			}
			e.WriteString(`\u202`)
			e.WriteByte(hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		e.WriteString(s[start:])
	}
	e.WriteByte('"')
	buf := append([]byte(nil), e.Bytes()...)
	bufferPool.Put(e)
	return buf
}

func valToBytes(v types.Val) ([]byte, error) {
	switch v.Tid {
	case types.StringID, types.DefaultID:
		switch str := v.Value.(type) {
		case string:
			return stringJsonMarshal(str), nil
		default:
			return json.Marshal(str)
		}
	case types.BinaryID:
		return []byte(fmt.Sprintf("%q", v.Value)), nil
	case types.IntID:
		// In types.Convert(), we always convert to int64 for IntID type. fmt.Sprintf is slow
		// and hence we are using strconv.FormatInt() here. Since int64 and int are most common int
		// types we are using FormatInt for those.
		switch num := v.Value.(type) {
		case int64:
			return []byte(strconv.FormatInt(num, 10)), nil
		case int:
			return []byte(strconv.FormatInt(int64(num), 10)), nil
		default:
			return []byte(fmt.Sprintf("%d", v.Value)), nil
		}
	case types.FloatID:
		f, fOk := v.Value.(float64)

		// +Inf, -Inf and NaN are not representable in JSON.
		// Please see https://golang.org/src/encoding/json/encode.go?s=6458:6501#L573
		if !fOk || math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, errors.New("Unsupported floating point number in float field")
		}

		return []byte(fmt.Sprintf("%f", f)), nil
	case types.BoolID:
		if v.Value.(bool) {
			return boolTrue, nil
		}
		return boolFalse, nil
	case types.DateTimeID:
		// Return empty string instead of zero-time value string - issue#3166
		t := v.Value.(time.Time)
		if t.IsZero() {
			return emptyString, nil
		}
		return t.MarshalJSON()
	case types.GeoID:
		return geojson.Marshal(v.Value.(geom.T))
	case types.UidID:
		return []byte(fmt.Sprintf("\"%#x\"", v.Value)), nil
	case types.PasswordID:
		return []byte(fmt.Sprintf("%q", v.Value.(string))), nil
	default:
		return nil, errors.New("Unsupported types.Val.Tid")
	}
}

// nodeSlice is a slice of fastJsonNodes, mostly used for sorting of fastJsonNodes based on attrs.
// To get string representation of attr while comparing, we also need encoder here.
type nodeSlice struct {
	nodes []fastJsonNode
	enc   *encoder
}

func (n nodeSlice) Len() int {
	return len(n.nodes)
}

func (n nodeSlice) Less(i, j int) bool {
	enc := n.enc
	attri := enc.getAttr(n.nodes[i])
	attrj := enc.getAttr(n.nodes[j])
	cmp := strings.Compare(enc.attrForID(attri), enc.attrForID(attrj))
	return cmp < 0
}

func (n nodeSlice) Swap(i, j int) {
	n.nodes[i], n.nodes[j] = n.nodes[j], n.nodes[i]
}

func (enc *encoder) writeKey(fj fastJsonNode, out *bytes.Buffer) error {
	if _, err := out.WriteRune('"'); err != nil {
		return err
	}
	attrID := enc.getAttr(fj)
	if _, err := out.WriteString(enc.attrForID(attrID)); err != nil {
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

func (enc *encoder) attachFacets(fj fastJsonNode, fieldName string, isList bool,
	fList []*api.Facet, facetIdx int) error {

	idxFieldID := enc.idForAttr(strconv.Itoa(facetIdx))
	for _, f := range fList {
		fName := facetName(fieldName, f)
		fVal, err := facets.ValFor(f)
		if err != nil {
			return err
		}

		if !isList {
			if err := enc.AddValue(fj, enc.idForAttr(fName), fVal); err != nil {
				return err
			}
		} else {
			facetNode := enc.newNode(enc.idForAttr(fName))
			err := enc.AddValue(facetNode, idxFieldID, fVal)
			if err != nil {
				return err
			}
			// Mark this node as facetsParent.
			enc.setFacetsParent(facetNode)
			enc.AddMapChild(fj, facetNode)
		}
	}

	return nil
}

func (enc *encoder) encode(fj fastJsonNode, out *bytes.Buffer) error {
	fjAttrs := enc.getAttrs(fj)
	// This is a scalar value.
	if len(fjAttrs) == 0 {
		val, err := enc.getScalarVal(fj)
		if err != nil {
			return err
		}
		_, err = out.Write(val)
		return err
	}

	// This is an internal node.
	if _, err := out.WriteRune('{'); err != nil {
		return err
	}
	cnt := 0
	var cur, next fastJsonNode
	for i := 0; i < len(fjAttrs); i++ {
		cnt++
		validNext := false
		cur = fjAttrs[i]
		if i+1 < len(fjAttrs) {
			next = fjAttrs[i+1]
			validNext = true
		}

		if validNext && enc.getAttr(cur) == enc.getAttr(next) {
			if cnt == 1 {
				if err := enc.writeKey(cur, out); err != nil {
					return err
				}
				if _, err := out.WriteRune('['); err != nil {
					return err
				}
			}
			if err := enc.encode(cur, out); err != nil {
				return err
			}
		} else {
			if cnt == 1 {
				if err := enc.writeKey(cur, out); err != nil {
					return err
				}
				if enc.getList(cur) {
					if _, err := out.WriteRune('['); err != nil {
						return err
					}
				}
			}
			if err := enc.encode(cur, out); err != nil {
				return err
			}
			if cnt > 1 || enc.getList(cur) {
				if _, err := out.WriteRune(']'); err != nil {
					return err
				}
			}
			cnt = 0 // Reset the count.
		}
		// We need to print comma except for the last attribute.
		if i != len(fjAttrs)-1 {
			if _, err := out.WriteRune(','); err != nil {
				return err
			}
		}

		cur = next
	}
	if _, err := out.WriteRune('}'); err != nil {
		return err
	}

	return nil
}

func merge(parent, child [][]fastJsonNode) ([][]fastJsonNode, error) {
	if len(parent) == 0 {
		return child, nil
	}

	// Here we merge two slices of maps.
	mergedList := make([][]fastJsonNode, 0, len(parent)*len(child))
	cnt := 0
	for _, pa := range parent {
		for _, ca := range child {
			cnt += len(pa) + len(ca)
			if cnt > x.Config.NormalizeNodeLimit {
				return nil, errors.Errorf(
					"Couldn't evaluate @normalize directive - too many results")
			}
			list := make([]fastJsonNode, 0, len(pa)+len(ca))
			list = append(list, pa...)
			list = append(list, ca...)
			mergedList = append(mergedList, list)
		}
	}
	return mergedList, nil
}

// normalize returns all attributes of fj and its children (if any).
func (enc *encoder) normalize(fj fastJsonNode) ([][]fastJsonNode, error) {
	cnt := 0
	fjAttrs := enc.getAttrs(fj)
	for _, a := range fjAttrs {
		// Here we are counting all non-scalar attributes of fj. If there are any such
		// attributes, we will flatten it, otherwise we will return all attributes.
		// We should only consider those nodes for flattening which have children and are not
		// facetsParent.
		if len(enc.getAttrs(a)) > 0 && !enc.getFacetsParent(a) {
			cnt++
		}
	}

	if cnt == 0 {
		// Recursion base case
		// There are no children, we can just return slice with fj.attrs map.
		return [][]fastJsonNode{fjAttrs}, nil
	}

	parentSlice := make([][]fastJsonNode, 0, 5)
	// If the parents has attrs, lets add them to the slice so that it can be
	// merged with children later.
	attrs := make([]fastJsonNode, 0, len(fjAttrs)-cnt)
	for _, a := range fjAttrs {
		// Here, add all nodes which have either no children or they are facetsParent.
		if len(enc.getAttrs(a)) == 0 || enc.getFacetsParent(a) {
			attrs = append(attrs, a)
		}
	}
	parentSlice = append(parentSlice, attrs)

	for ci := 0; ci < len(fjAttrs); {
		childNode := fjAttrs[ci]
		// Here, exclude all nodes which have either no children or they are facetsParent.
		if len(enc.getAttrs(childNode)) == 0 || enc.getFacetsParent(childNode) {
			ci++
			continue
		}
		childSlice := make([][]fastJsonNode, 0, 5)
		for ci < len(fjAttrs) && enc.getAttr(childNode) == enc.getAttr(fjAttrs[ci]) {
			childSlice = append(childSlice, enc.getAttrs(fjAttrs[ci]))
			ci++
		}
		// Merging with parent.
		var err error
		parentSlice, err = merge(parentSlice, childSlice)
		if err != nil {
			return nil, err
		}
	}

	uidAttrID := enc.idForAttr("uid")
	for i, slice := range parentSlice {
		sort.Sort(nodeSlice{nodes: slice, enc: enc})

		first := -1
		last := 0
		for i := range slice {
			if enc.getAttr(slice[i]) == uidAttrID {
				if first == -1 {
					first = i
				}
				last = i
			}
		}
		if first != -1 && first != last {
			if first == 0 {
				parentSlice[i] = slice[last:]
			} else {
				parentSlice[i] = append(slice[:first], slice[last:]...)
			}
		}
	}

	return parentSlice, nil
}

func (sg *SubGraph) addGroupby(enc *encoder, fj fastJsonNode,
	res *groupResults, fname string) error {

	// Don't add empty groupby
	if len(res.group) == 0 {
		return nil
	}
	g := enc.newNode(enc.idForAttr(fname))
	for _, grp := range res.group {
		uc := enc.newNode(enc.idForAttr("@groupby"))
		for _, it := range grp.keys {
			if err := enc.AddValue(uc, enc.idForAttr(it.attr), it.key); err != nil {
				return err
			}
		}
		for _, it := range grp.aggregates {
			if err := enc.AddValue(uc, enc.idForAttr(it.attr), it.key); err != nil {
				return err
			}
		}
		enc.AddListChild(g, uc)
	}
	enc.AddListChild(fj, g)
	return nil
}

func (sg *SubGraph) addAggregations(enc *encoder, fj fastJsonNode) error {
	for _, child := range sg.Children {
		aggVal, ok := child.Params.UidToVal[0]
		if !ok {
			if len(child.Params.NeedsVar) == 0 {
				return errors.Errorf("Only aggregated variables allowed within empty block.")
			}
			// the aggregation didn't happen, most likely was called with unset vars.
			// See: query.go:fillVars
			aggVal = types.Val{Tid: types.FloatID, Value: float64(0)}
		}
		if child.Params.Normalize && child.Params.Alias == "" {
			continue
		}
		fieldName := child.aggWithVarFieldName()
		n1 := enc.newNode(enc.idForAttr(sg.Params.Alias))
		if err := enc.AddValue(n1, enc.idForAttr(fieldName), aggVal); err != nil {
			return err
		}
		enc.AddListChild(fj, n1)
	}
	if enc.IsEmpty(fj) {
		enc.AddListChild(fj, enc.newNode(enc.idForAttr(sg.Params.Alias)))
	}
	return nil
}

func (sg *SubGraph) handleCountUIDNodes(enc *encoder, n fastJsonNode, count int) (bool, error) {
	addedNewChild := false
	fieldName := sg.fieldName()
	sgFieldID := enc.idForAttr(fieldName)
	for _, child := range sg.Children {
		uidCount := child.Attr == "uid" && child.Params.DoCount && child.IsInternal()
		normWithoutAlias := child.Params.Alias == "" && child.Params.Normalize
		if uidCount && !normWithoutAlias {
			addedNewChild = true

			c := types.ValueForType(types.IntID)
			c.Value = int64(count)

			field := child.Params.Alias
			if field == "" {
				field = "count"
			}

			fjChild := enc.newNode(sgFieldID)
			if err := enc.AddValue(fjChild, enc.idForAttr(field), c); err != nil {
				return false, err
			}
			enc.AddListChild(n, fjChild)
		}
	}

	return addedNewChild, nil
}

func processNodeUids(fj fastJsonNode, enc *encoder, sg *SubGraph) error {
	if sg.Params.IsEmpty {
		return sg.addAggregations(enc, fj)
	}

	enc.curSize += uint64(len(sg.Params.Alias))

	attrID := enc.idForAttr(sg.Params.Alias)
	if sg.uidMatrix == nil {
		enc.AddListChild(fj, enc.newNode(attrID))
		return nil
	}

	hasChild, err := sg.handleCountUIDNodes(enc, fj, len(sg.DestUIDs.Uids))
	if err != nil {
		return err
	}
	if sg.Params.IsGroupBy {
		if len(sg.GroupbyRes) == 0 {
			return errors.Errorf("Expected GroupbyRes to have length > 0.")
		}
		return sg.addGroupby(enc, fj, sg.GroupbyRes[0], sg.Params.Alias)
	}

	lenList := len(sg.uidMatrix[0].Uids)
	for i := 0; i < lenList; i++ {
		uid := sg.uidMatrix[0].Uids[i]
		if algo.IndexOf(sg.DestUIDs, uid) < 0 {
			// This UID was filtered. So Ignore it.
			continue
		}

		n1 := enc.newNode(attrID)
		enc.setAttr(n1, enc.idForAttr(sg.Params.Alias))
		if err := sg.preTraverse(enc, uid, n1); err != nil {
			if err.Error() == "_INV_" {
				continue
			}
			return err
		}

		if enc.IsEmpty(n1) {
			continue
		}

		hasChild = true
		if !sg.Params.Normalize {
			enc.AddListChild(fj, n1)
			continue
		}

		// Lets normalize the response now.
		normalized, err := enc.normalize(n1)
		if err != nil {
			return err
		}
		for _, c := range normalized {
			node := enc.newNode(attrID)
			enc.appendAttrs(node, c...)
			enc.AddListChild(fj, node)
		}
	}

	if !hasChild {
		// So that we return an empty key if the root didn't have any children.
		enc.AddListChild(fj, enc.newNode(attrID))
	}
	return nil
}

// Extensions represents the extra information appended to query results.
type Extensions struct {
	Latency *api.Latency    `json:"server_latency,omitempty"`
	Txn     *api.TxnContext `json:"txn,omitempty"`
	Metrics *api.Metrics    `json:"metrics,omitempty"`
}

func (sg *SubGraph) toFastJSON(l *Latency) ([]byte, error) {
	encodingStart := time.Now()
	defer func() {
		l.Json = time.Since(encodingStart)
	}()

	enc := newEncoder()
	var err error
	n := enc.newNode(enc.idForAttr("_root_"))
	for _, sg := range sg.Children {
		err = processNodeUids(n, enc, sg)
		if err != nil {
			return nil, err
		}
	}

	// According to GraphQL spec response should only contain data, errors and extensions as top
	// level keys. Hence we send server_latency under extensions key.
	// https://facebook.github.io/graphql/#sec-Response-Format

	var bufw bytes.Buffer
	if len(enc.getAttrs(n)) == 0 {
		if _, err := bufw.WriteString(`{}`); err != nil {
			return nil, err
		}
	} else {
		if err := enc.encode(n, &bufw); err != nil {
			return nil, err
		}
	}

	// Return error if encoded buffer size exceeds than a threshold size.
	if uint64(bufw.Len()) > maxEncodedSize {
		return nil, fmt.Errorf("encoded response size: %d is bigger than threshold: %d",
			bufw.Len(), maxEncodedSize)
	}

	// Put encoder's arena back to arena pool.
	arenaPool.Put(enc.arena)
	return bufw.Bytes(), nil
}

func (sg *SubGraph) fieldName() string {
	fieldName := sg.Attr
	if sg.Params.Alias != "" {
		fieldName = sg.Params.Alias
	}
	return fieldName
}

func (sg *SubGraph) addCount(enc *encoder, count uint64, dst fastJsonNode) error {
	if sg.Params.Normalize && sg.Params.Alias == "" {
		return nil
	}
	c := types.ValueForType(types.IntID)
	c.Value = int64(count)
	fieldName := sg.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("count(%s)", sg.Attr)
	}
	return enc.AddValue(dst, enc.idForAttr(fieldName), c)
}

func (sg *SubGraph) aggWithVarFieldName() string {
	if sg.Params.Alias != "" {
		return sg.Params.Alias
	}
	fieldName := fmt.Sprintf("val(%v)", sg.Params.Var)
	if len(sg.Params.NeedsVar) > 0 {
		fieldName = fmt.Sprintf("val(%v)", sg.Params.NeedsVar[0].Name)
		if sg.SrcFunc != nil {
			fieldName = fmt.Sprintf("%s(%v)", sg.SrcFunc.Name, fieldName)
		}
	}
	return fieldName
}

func (sg *SubGraph) addInternalNode(enc *encoder, uid uint64, dst fastJsonNode) error {
	sv, ok := sg.Params.UidToVal[uid]
	if !ok || sv.Value == nil {
		return nil
	}
	fieldName := sg.aggWithVarFieldName()
	return enc.AddValue(dst, enc.idForAttr(fieldName), sv)
}

func (sg *SubGraph) addCheckPwd(enc *encoder, vals []*pb.TaskValue, dst fastJsonNode) error {
	c := types.ValueForType(types.BoolID)
	if len(vals) == 0 {
		c.Value = false
	} else {
		c.Value = task.ToBool(vals[0])
	}

	fieldName := sg.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("checkpwd(%s)", sg.Attr)
	}
	return enc.AddValue(dst, enc.idForAttr(fieldName), c)
}

func alreadySeen(parentIds []uint64, uid uint64) bool {
	for _, id := range parentIds {
		if id == uid {
			return true
		}
	}
	return false
}

func facetName(fieldName string, f *api.Facet) string {
	if f.Alias != "" {
		return f.Alias
	}
	return fieldName + x.FacetDelimeter + f.Key
}

// This method gets the values and children for a subprotos.
func (sg *SubGraph) preTraverse(enc *encoder, uid uint64, dst fastJsonNode) error {
	if sg.Params.IgnoreReflex {
		if alreadySeen(sg.Params.ParentIds, uid) {
			// A node can't have itself as the child at any level.
			return nil
		}
		// Push myself to stack before sending this to children.
		sg.Params.ParentIds = append(sg.Params.ParentIds, uid)
	}

	var invalidUids map[uint64]bool
	// We go through all predicate children of the subprotos.
	for _, pc := range sg.Children {
		if pc.Params.IgnoreResult {
			continue
		}
		if pc.IsInternal() {
			if pc.Params.Expand != "" {
				continue
			}
			if pc.Params.Normalize && pc.Params.Alias == "" {
				continue
			}
			if err := pc.addInternalNode(enc, uid, dst); err != nil {
				return err
			}
			continue
		}

		if len(pc.uidMatrix) == 0 {
			// Can happen in recurse query.
			continue
		}
		if len(pc.facetsMatrix) > 0 && len(pc.facetsMatrix) != len(pc.uidMatrix) {
			return errors.Errorf("Length of facetsMatrix and uidMatrix mismatch: %d vs %d",
				len(pc.facetsMatrix), len(pc.uidMatrix))
		}

		idx := algo.IndexOf(pc.SrcUIDs, uid)
		if idx < 0 {
			continue
		}
		if pc.Params.IsGroupBy {
			if len(pc.GroupbyRes) <= idx {
				return errors.Errorf("Unexpected length while adding Groupby. Idx: [%v], len: [%v]",
					idx, len(pc.GroupbyRes))
			}
			if err := pc.addGroupby(enc, dst, pc.GroupbyRes[idx], pc.fieldName()); err != nil {
				return err
			}
			continue
		}

		fieldName := pc.fieldName()
		switch {
		case len(pc.counts) > 0:
			if err := pc.addCount(enc, uint64(pc.counts[idx]), dst); err != nil {
				return err
			}

		case pc.SrcFunc != nil && pc.SrcFunc.Name == "checkpwd":
			if err := pc.addCheckPwd(enc, pc.valueMatrix[idx].Values, dst); err != nil {
				return err
			}

		case idx < len(pc.uidMatrix) && len(pc.uidMatrix[idx].Uids) > 0:
			var fcsList []*pb.Facets
			if pc.Params.Facet != nil {
				fcsList = pc.facetsMatrix[idx].FacetsList
			}

			if sg.Params.IgnoreReflex {
				pc.Params.ParentIds = sg.Params.ParentIds
			}

			// calculate it once to avoid mutliple call to idToAttr()
			fieldID := enc.idForAttr(fieldName)
			// Add len of fieldName to enc.curSize.
			enc.curSize += uint64(len(fieldName))

			// We create as many predicate entity children as the length of uids for
			// this predicate.
			ul := pc.uidMatrix[idx]
			for childIdx, childUID := range ul.Uids {
				if fieldName == "" || (invalidUids != nil && invalidUids[childUID]) {
					continue
				}
				uc := enc.newNode(fieldID)
				if rerr := pc.preTraverse(enc, childUID, uc); rerr != nil {
					if rerr.Error() == "_INV_" {
						if invalidUids == nil {
							invalidUids = make(map[uint64]bool)
						}

						invalidUids[childUID] = true
						continue // next UID.
					}
					// Some other error.
					glog.Errorf("Error while traversal: %v", rerr)
					return rerr
				}

				if !enc.IsEmpty(uc) {
					if sg.Params.GetUid {
						if err := enc.SetUID(uc, childUID, enc.idForAttr("uid")); err != nil {
							return err
						}
					}

					// Add facets nodes.
					if pc.Params.Facet != nil && len(fcsList) > childIdx {
						fs := fcsList[childIdx].Facets
						if err := enc.attachFacets(uc, fieldName, false, fs, childIdx); err != nil {
							return err
						}
					}

					if pc.Params.Normalize {
						// We will normalize at each level instead of
						// calling normalize after pretraverse.
						// Now normalize() only flattens one level,
						// the expectation is that its children have
						// already been normalized.
						normAttrs, err := enc.normalize(uc)
						if err != nil {
							return err
						}

						for _, c := range normAttrs {
							// Adding as list child irrespective of the type of pc
							// (list or non-list), otherwise result might be inconsistent or might
							// depend on children and grandchildren of pc. Consider the case:
							// 	boss: uid .
							// 	friend: [uid] .
							// 	name: string .
							// For query like:
							// {
							// 	me(func: uid(0x1)) {
							// 		boss @normalize {
							// 			name
							// 		}
							// 	}
							// }
							// boss will be non list type in response, but for query like:
							// {
							// 	me(func: uid(0x1)) {
							// 		boss @normalize {
							// 			friend {
							// 				name
							// 			}
							// 		}
							// 	}
							// }
							// boss should be of list type because there can be mutliple friends of
							// boss.
							node := enc.newNode(fieldID)
							enc.appendAttrs(node, c...)
							enc.AddListChild(dst, node)
						}
						continue
					}
					if pc.List {
						enc.AddListChild(dst, uc)
					} else {
						enc.AddMapChild(dst, uc)
					}
				}
			}

			// add value for count(uid) nodes if any.
			if _, err := pc.handleCountUIDNodes(enc, dst, len(ul.Uids)); err != nil {
				return err
			}
		default:
			if pc.Params.Alias == "" && len(pc.Params.Langs) > 0 && pc.Params.Langs[0] != "*" {
				fieldName += "@"
				fieldName += strings.Join(pc.Params.Langs, ":")
			}

			// calculate it once to avoid mutliple call to idToAttr()
			fieldID := enc.idForAttr(fieldName)
			// Add len of fieldName to enc.curSize.
			enc.curSize += uint64(len(fieldName))

			if pc.Attr == "uid" {
				if err := enc.SetUID(dst, uid, fieldID); err != nil {
					return err
				}
				continue
			}

			if len(pc.facetsMatrix) > idx && len(pc.facetsMatrix[idx].FacetsList) > 0 {
				// In case of Value we have only one Facets.
				for i, fcts := range pc.facetsMatrix[idx].FacetsList {
					if err := enc.attachFacets(dst, fieldName, pc.List, fcts.Facets, i); err != nil {
						return err
					}
				}
			}

			if len(pc.valueMatrix) <= idx {
				continue
			}

			for i, tv := range pc.valueMatrix[idx].Values {
				// if conversion not possible, we ignore it in the result.
				sv, convErr := convertWithBestEffort(tv, pc.Attr)
				if convErr != nil {
					return convErr
				}

				if pc.Params.ExpandAll && len(pc.LangTags[idx].Lang) != 0 {
					if i >= len(pc.LangTags[idx].Lang) {
						return errors.Errorf(
							"pb.error: all lang tags should be either present or absent")
					}
					fieldNameWithTag := fieldName
					lang := pc.LangTags[idx].Lang[i]
					if lang != "" && lang != "*" {
						fieldNameWithTag += "@" + lang
					}
					encodeAsList := pc.List && lang == ""
					if err := enc.AddListValue(dst, enc.idForAttr(fieldNameWithTag),
						sv, encodeAsList); err != nil {
						return err
					}
					continue
				}

				encodeAsList := pc.List && len(pc.Params.Langs) == 0
				if !pc.Params.Normalize {
					err := enc.AddListValue(dst, fieldID, sv, encodeAsList)
					if err != nil {
						return err
					}
					continue
				}
				// If the query had the normalize directive, then we only add nodes
				// with an Alias.
				if pc.Params.Alias != "" {
					err := enc.AddListValue(dst, fieldID, sv, encodeAsList)
					if err != nil {
						return err
					}
				}
			}
		}
	}

	if sg.Params.IgnoreReflex && len(sg.Params.ParentIds) > 0 {
		// Lets pop the stack.
		sg.Params.ParentIds = (sg.Params.ParentIds)[:len(sg.Params.ParentIds)-1]
	}

	// Only for shortest path query we wan't to return uid always if there is
	// nothing else at that level.
	if (sg.Params.GetUid && !enc.IsEmpty(dst)) || sg.Params.Shortest {
		if err := enc.SetUID(dst, uid, enc.idForAttr("uid")); err != nil {
			return err
		}
	}

	if sg.pathMeta != nil {
		totalWeight := types.Val{
			Tid:   types.FloatID,
			Value: sg.pathMeta.weight,
		}
		if err := enc.AddValue(dst, enc.idForAttr("_weight_"), totalWeight); err != nil {
			return err
		}
	}

	return nil
}
