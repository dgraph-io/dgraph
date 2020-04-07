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
	"encoding/binary"
	"encoding/json"
	"fmt"
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

	"github.com/dgraph-io/dgo/v2/protos/api"
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

func (enc *encoder) makeScalarNode(attr uint16, isChild bool, val []byte, list bool) fastJsonNode {
	fj := enc.newFastJsonNode()
	fj.setAttr(enc, attr)
	fj.setIsChild(enc, isChild)
	fj.setScalarVal(enc, val)
	fj.setList(enc, list)

	return fj
}

type encoder struct {
	attrMap map[string]uint16
	idMap   map[uint16]string
	seqNo   uint16
	arena   *arena

	root          fastJsonNode
	compositSlice []uint64
	childrenMap   map[fastJsonNode][]fastJsonNode
}

func newEncoder() *encoder {
	compositSlice := make([]uint64, 0)
	compositSlice = append(compositSlice, 0) // Append dummy entry.
	return &encoder{
		attrMap: make(map[string]uint16),
		idMap:   make(map[uint16]string),
		seqNo:   uint16(0),
		arena:   newArena(1 * 1024 /*1 KB*/),

		compositSlice: compositSlice,
		childrenMap:   make(map[fastJsonNode][]fastJsonNode),
	}
}

var (
// enc = newEncoder()
)

func (e *encoder) idForAttr(attr string) uint16 {
	if id, ok := e.attrMap[attr]; ok {
		return id
	}

	e.seqNo++
	e.attrMap[attr] = e.seqNo
	e.idMap[e.seqNo] = attr
	return e.seqNo
}

func (e *encoder) attrForID(id uint16) string {
	if id == 0 {
		return ""
	}

	x.AssertTrue(id <= e.seqNo)
	if attr, ok := e.idMap[id]; ok {
		return attr
	}

	// Panic for now.
	panic(fmt.Sprintf("id not found in map: %d", id))
}

const (
	msbBit       = uint64(1) << 63
	secondMsbBit = uint64(1) << 62
	setByte56    = uint64(0xFFFF) << 32
	setBytes1234 = uint64(0xFFFFFFFF)
)

type fastJsonNode int

func (enc *encoder) newFastJsonNode() fastJsonNode {
	enc.compositSlice = append(enc.compositSlice, 0)
	return fastJsonNode(len(enc.compositSlice) - 1)
}

func (fj fastJsonNode) setAttr(enc *encoder, attr uint16) {
	meta := enc.compositSlice[fj]
	// First clear.
	meta &= 0xFFFF0000FFFFFFFF
	meta |= (uint64(attr) << 32)

	enc.compositSlice[fj] = meta
}

func (fj fastJsonNode) setScalarVal(enc *encoder, sv []byte) {
	meta := enc.compositSlice[fj]
	meta |= uint64(enc.arena.put(sv))
	enc.compositSlice[fj] = meta
}

func (fj fastJsonNode) setIsChild(enc *encoder, isChild bool) {
	if isChild {
		meta := enc.compositSlice[fj]
		meta |= msbBit
		enc.compositSlice[fj] = meta
	}
}

func (fj fastJsonNode) setList(enc *encoder, list bool) {
	if list {
		meta := enc.compositSlice[fj]
		meta |= secondMsbBit
		enc.compositSlice[fj] = meta
	}
}

func (fj fastJsonNode) appendAttrs(enc *encoder, attrs []fastJsonNode) {
	cs, ok := enc.childrenMap[fj]
	if ok {
		cs = append(cs, attrs...)
	} else {
		cs = make([]fastJsonNode, 0)
		cs = append(cs, attrs...)
	}
	enc.childrenMap[fj] = cs
}

func (fj fastJsonNode) getAttr(enc *encoder) uint16 {
	meta := enc.compositSlice[fj]
	return uint16((meta & setByte56) >> 32)
}

func (fj fastJsonNode) getScalarVal(enc *encoder) []byte {
	meta := enc.compositSlice[fj]
	offset := uint32(meta & setBytes1234)

	return enc.arena.get(offset)
}

func (fj fastJsonNode) getIsChild(enc *encoder) bool {
	meta := enc.compositSlice[fj]
	return ((meta & msbBit) > 0)
}

func (fj fastJsonNode) getList(enc *encoder) bool {
	meta := enc.compositSlice[fj]
	return ((meta & secondMsbBit) > 0)
}

func (fj fastJsonNode) getAttrs(enc *encoder) []fastJsonNode {
	if attrs, ok := enc.childrenMap[fj]; ok {
		return attrs // Not copying it for now.
	}

	// // Returning nil if no attrs are found.
	return nil
}

// For debugging.
func (fj fastJsonNode) printMeta(enc *encoder) {
	meta := enc.compositSlice[fj]
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], meta)
}

func (enc *encoder) AddValue(fj fastJsonNode, attr uint16, v types.Val) {
	enc.AddListValue(fj, attr, v, false)
}

func (enc *encoder) AddListValue(fj fastJsonNode, attr uint16, v types.Val, list bool) {
	if bs, err := valToBytes(v); err == nil {
		fj.appendAttrs(enc, []fastJsonNode{enc.makeScalarNode(attr, false, bs, list)})
	}
}

func (enc *encoder) AddMapChild(fj fastJsonNode, attr uint16, val fastJsonNode, isRoot bool) {
	var childNode fastJsonNode
	for _, c := range fj.getAttrs(enc) {
		if c.getAttr(enc) == attr {
			childNode = c
			break
		}
	}

	if childNode > 0 {
		val.setIsChild(enc, true)
		val.setAttr(enc, attr)
		childNode.appendAttrs(enc, val.getAttrs(enc))
	} else {
		val.setIsChild(enc, false)
		val.setAttr(enc, attr)
		fj.appendAttrs(enc, []fastJsonNode{val})
	}
}

func (enc *encoder) AddListChild(fj fastJsonNode, attr uint16, child fastJsonNode) {
	child.setAttr(enc, attr)
	child.setIsChild(enc, true)
	fj.appendAttrs(enc, []fastJsonNode{child})
}

func (enc *encoder) newFastJsonNodeWithAttr(attr uint16) fastJsonNode {
	nn := enc.newFastJsonNode()
	nn.setAttr(enc, attr)
	nn.setIsChild(enc, false)
	return nn
}

func (enc *encoder) SetUID(fj fastJsonNode, uid uint64, attr uint16) {
	// if we're in debug mode, uid may be added second time, skip this
	if attr == enc.idForAttr("uid") {
		for _, a := range fj.getAttrs(enc) {
			if a.getAttr(enc) == attr {
				return
			}
		}
	}

	fj.appendAttrs(enc, []fastJsonNode{enc.makeScalarNode(attr, false, []byte(fmt.Sprintf("\"%#x\"", uid)),
		false)})
}

func (enc *encoder) IsEmpty(fj fastJsonNode) bool {
	return len(fj.getAttrs(enc)) == 0
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
		return []byte(fmt.Sprintf("%f", v.Value)), nil
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

type nodeSlice struct {
	nodes []fastJsonNode
	enc   *encoder
}

func (n nodeSlice) Len() int {
	return len(n.nodes)
}

func (n nodeSlice) Less(i, j int) bool {
	cmp := strings.Compare(n.enc.attrForID(n.nodes[i].getAttr(n.enc)), n.enc.attrForID(n.nodes[j].getAttr(n.enc)))
	return cmp < 0
}

func (n nodeSlice) Swap(i, j int) {
	n.nodes[i], n.nodes[j] = n.nodes[j], n.nodes[i]
}

func (enc *encoder) writeKey(fj fastJsonNode, out *bytes.Buffer) error {
	if _, err := out.WriteRune('"'); err != nil {
		return err
	}
	attrId := fj.getAttr(enc)
	if _, err := out.WriteString(enc.attrForID(attrId)); err != nil {
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

	for _, f := range fList {
		fName := facetName(fieldName, f)
		fVal, err := facets.ValFor(f)
		if err != nil {
			return err
		}

		if !isList {
			enc.AddValue(fj, enc.idForAttr(fName), fVal)
		} else {
			facetNode := enc.newFastJsonNode()
			facetNode.setAttr(enc, enc.idForAttr(fName))
			enc.AddValue(facetNode, enc.idForAttr(strconv.Itoa(facetIdx)), fVal)
			enc.AddMapChild(fj, enc.idForAttr(fName), facetNode, false)
		}
	}

	return nil
}

func (enc *encoder) encode(fj fastJsonNode, out *bytes.Buffer) error {
	i := 0
	fjAttrs := fj.getAttrs(enc)
	if i < len(fjAttrs) {
		if _, err := out.WriteRune('{'); err != nil {
			return err
		}
		cur := fjAttrs[i]
		i++
		cnt := 1
		last := false
		inArray := false
		for {
			var next fastJsonNode
			if i < len(fjAttrs) {
				next = fjAttrs[i]
				i++
			} else {
				last = true
			}

			if !last {
				if cur.getAttr(enc) == next.getAttr(enc) {
					if cnt == 1 {
						if err := enc.writeKey(cur, out); err != nil {
							return err
						}
						if _, err := out.WriteRune('['); err != nil {
							return err
						}
						inArray = true
					}
					if err := enc.encode(cur, out); err != nil {
						return err
					}
					cnt++
				} else {
					if cnt == 1 {
						if err := enc.writeKey(cur, out); err != nil {
							return err
						}
						if cur.getIsChild(enc) || cur.getList(enc) {
							if _, err := out.WriteRune('['); err != nil {
								return err
							}
							inArray = true
						}
					}
					if err := enc.encode(cur, out); err != nil {
						return err
					}
					if cnt != 1 || (cur.getIsChild(enc) || cur.getList(enc)) {
						if _, err := out.WriteRune(']'); err != nil {
							return err
						}
						inArray = false
					}
					cnt = 1
				}
				if _, err := out.WriteRune(','); err != nil {
					return err
				}

				cur = next
			} else {
				if cnt == 1 {
					if err := enc.writeKey(cur, out); err != nil {
						return err
					}
				}
				if (cur.getIsChild(enc) || cur.getList(enc)) && !inArray {
					if _, err := out.WriteRune('['); err != nil {
						return err
					}
				}
				if err := enc.encode(cur, out); err != nil {
					return err
				}
				if cnt != 1 || (cur.getIsChild(enc) || cur.getList(enc)) {
					if _, err := out.WriteRune(']'); err != nil {
						return err
					}
				}
				break
			}
		}
		if _, err := out.WriteRune('}'); err != nil {
			return err
		}
	} else {
		if _, err := out.Write(fj.getScalarVal(enc)); err != nil {
			return err
		}
	}

	return nil
}

func merge(parent [][]fastJsonNode, child [][]fastJsonNode) ([][]fastJsonNode, error) {
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
	for _, a := range fj.getAttrs(enc) {
		// Here we are counting all non-scalar attributes of fj. If there are any such
		// attributes, we will flatten it, otherwise we will return all attributes.

		// When we call addMapChild it tries to find whether there is already an attribute
		// with attr field same as attribute argument of addMapChild. If it doesn't find any
		// such attribute, it creates an attribute with isChild = false. In those cases
		// sometimes cnt remains zero  and normalize returns attributes without flattening.
		// So we are using len(a.attrs) > 0 instead of a.isChild
		if len(a.getAttrs(enc)) > 0 {
			cnt++
		}
	}

	if cnt == 0 {
		// Recursion base case
		// There are no children, we can just return slice with fj.attrs map.
		return [][]fastJsonNode{fj.getAttrs(enc)}, nil
	}

	parentSlice := make([][]fastJsonNode, 0, 5)
	// If the parents has attrs, lets add them to the slice so that it can be
	// merged with children later.
	attrs := make([]fastJsonNode, 0, len(fj.getAttrs(enc))-cnt)
	for _, a := range fj.getAttrs(enc) {
		// Check comment at previous occurrence of len(a.attrs) > 0
		if len(a.getAttrs(enc)) == 0 {
			attrs = append(attrs, a)
		}
	}
	parentSlice = append(parentSlice, attrs)

	fjAttrs := fj.getAttrs(enc)
	for ci := 0; ci < len(fjAttrs); {
		childNode := fjAttrs[ci]
		// Check comment at previous occurrence of len(a.attrs) > 0
		if len(childNode.getAttrs(enc)) == 0 {
			ci++
			continue
		}
		childSlice := make([][]fastJsonNode, 0, 5)
		for ci < len(fjAttrs) && childNode.getAttr(enc) == fjAttrs[ci].getAttr(enc) {
			childSlice = append(childSlice, fjAttrs[ci].getAttrs(enc))
			ci++
		}
		// Merging with parent.
		var err error
		parentSlice, err = merge(parentSlice, childSlice)
		if err != nil {
			return nil, err
		}
	}
	for i, slice := range parentSlice {
		sort.Sort(nodeSlice{nodes: slice, enc: enc})

		first := -1
		last := 0
		for i := range slice {
			if slice[i].getAttr(enc) == enc.idForAttr("uid") {
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

func (enc *encoder) addGroupby(fj fastJsonNode, sg *SubGraph, res *groupResults, fname string) {
	// Don't add empty groupby
	if len(res.group) == 0 {
		return
	}
	g := enc.newFastJsonNodeWithAttr(enc.idForAttr(fname))
	for _, grp := range res.group {
		uc := enc.newFastJsonNodeWithAttr(enc.idForAttr("@groupby"))
		for _, it := range grp.keys {
			enc.AddValue(uc, enc.idForAttr(it.attr), it.key)
		}
		for _, it := range grp.aggregates {
			enc.AddValue(uc, enc.idForAttr(it.attr), it.key)
		}
		enc.AddListChild(g, enc.idForAttr("@groupby"), uc)
	}
	enc.AddListChild(fj, enc.idForAttr(fname), g)
}

func (enc *encoder) addAggregations(fj fastJsonNode, sg *SubGraph) error {
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
		fieldName := aggWithVarFieldName(child)
		n1 := enc.newFastJsonNodeWithAttr(enc.idForAttr(fieldName))
		enc.AddValue(n1, enc.idForAttr(fieldName), aggVal)
		enc.AddListChild(fj, enc.idForAttr(sg.Params.Alias), n1)
	}
	if enc.IsEmpty(fj) {
		enc.AddListChild(fj, enc.idForAttr(sg.Params.Alias), enc.newFastJsonNode())
	}
	return nil
}

func handleCountUIDNodes(sg *SubGraph, enc *encoder, n fastJsonNode, count int) bool {
	addedNewChild := false
	fieldName := sg.fieldName()
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

			fjChild := enc.newFastJsonNodeWithAttr(enc.idForAttr(fieldName))
			enc.AddValue(fjChild, enc.idForAttr(field), c)
			enc.AddListChild(n, enc.idForAttr(fieldName), fjChild)
		}
	}

	return addedNewChild
}

func processNodeUids(fj fastJsonNode, enc *encoder, sg *SubGraph) error {
	if sg.Params.IsEmpty {
		return enc.addAggregations(fj, sg)
	}

	if sg.uidMatrix == nil {
		enc.AddListChild(fj, enc.idForAttr(sg.Params.Alias), enc.newFastJsonNode())
		return nil
	}

	hasChild := handleCountUIDNodes(sg, enc, fj, len(sg.DestUIDs.Uids))
	if sg.Params.IsGroupBy {
		if len(sg.GroupbyRes) == 0 {
			return errors.Errorf("Expected GroupbyRes to have length > 0.")
		}
		enc.addGroupby(fj, sg, sg.GroupbyRes[0], sg.Params.Alias)
		return nil
	}

	lenList := len(sg.uidMatrix[0].Uids)
	for i := 0; i < lenList; i++ {
		uid := sg.uidMatrix[0].Uids[i]
		if algo.IndexOf(sg.DestUIDs, uid) < 0 {
			// This UID was filtered. So Ignore it.
			continue
		}

		n1 := enc.newFastJsonNodeWithAttr(enc.idForAttr(sg.Params.Alias))
		n1.setAttr(enc, enc.idForAttr(sg.Params.Alias))
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
			enc.AddListChild(fj, enc.idForAttr(sg.Params.Alias), n1)
			continue
		}

		// Lets normalize the response now.
		normalized, err := enc.normalize(n1)
		if err != nil {
			return err
		}
		for _, c := range normalized {
			node := enc.newFastJsonNode()
			node.appendAttrs(enc, c)
			enc.AddListChild(fj, enc.idForAttr(sg.Params.Alias), node)
		}
	}

	if !hasChild {
		// So that we return an empty key if the root didn't have any children.
		enc.AddListChild(fj, enc.idForAttr(sg.Params.Alias), enc.newFastJsonNode())
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
	n := enc.newFastJsonNodeWithAttr(enc.idForAttr("_root_"))
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
	if len(n.getAttrs(enc)) == 0 {
		if _, err := bufw.WriteString(`{}`); err != nil {
			return nil, err
		}
	} else {
		if err := enc.encode(n, &bufw); err != nil {
			return nil, err
		}
	}
	return bufw.Bytes(), nil
}

func (sg *SubGraph) fieldName() string {
	fieldName := sg.Attr
	if sg.Params.Alias != "" {
		fieldName = sg.Params.Alias
	}
	return fieldName
}

func addCount(pc *SubGraph, enc *encoder, count uint64, dst fastJsonNode) {
	if pc.Params.Normalize && pc.Params.Alias == "" {
		return
	}
	c := types.ValueForType(types.IntID)
	c.Value = int64(count)
	fieldName := pc.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("count(%s)", pc.Attr)
	}
	enc.AddValue(dst, enc.idForAttr(fieldName), c)
}

func aggWithVarFieldName(pc *SubGraph) string {
	if pc.Params.Alias != "" {
		return pc.Params.Alias
	}
	fieldName := fmt.Sprintf("val(%v)", pc.Params.Var)
	if len(pc.Params.NeedsVar) > 0 {
		fieldName = fmt.Sprintf("val(%v)", pc.Params.NeedsVar[0].Name)
		if pc.SrcFunc != nil {
			fieldName = fmt.Sprintf("%s(%v)", pc.SrcFunc.Name, fieldName)
		}
	}
	return fieldName
}

func addInternalNode(pc *SubGraph, enc *encoder, uid uint64, dst fastJsonNode) error {
	sv, ok := pc.Params.UidToVal[uid]
	if !ok || sv.Value == nil {
		return nil
	}
	fieldName := aggWithVarFieldName(pc)
	enc.AddValue(dst, enc.idForAttr(fieldName), sv)
	return nil
}

func addCheckPwd(pc *SubGraph, enc *encoder, vals []*pb.TaskValue, dst fastJsonNode) {
	c := types.ValueForType(types.BoolID)
	if len(vals) == 0 {
		c.Value = false
	} else {
		c.Value = task.ToBool(vals[0])
	}

	fieldName := pc.Params.Alias
	if fieldName == "" {
		fieldName = fmt.Sprintf("checkpwd(%s)", pc.Attr)
	}
	enc.AddValue(dst, enc.idForAttr(fieldName), c)
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
			if err := addInternalNode(pc, enc, uid, dst); err != nil {
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
			enc.addGroupby(dst, pc, pc.GroupbyRes[idx], pc.fieldName())
			continue
		}

		fieldName := pc.fieldName()
		switch {
		case len(pc.counts) > 0:
			addCount(pc, enc, uint64(pc.counts[idx]), dst)

		case pc.SrcFunc != nil && pc.SrcFunc.Name == "checkpwd":
			addCheckPwd(pc, enc, pc.valueMatrix[idx].Values, dst)

		case idx < len(pc.uidMatrix) && len(pc.uidMatrix[idx].Uids) > 0:
			var fcsList []*pb.Facets
			if pc.Params.Facet != nil {
				fcsList = pc.facetsMatrix[idx].FacetsList
			}

			if sg.Params.IgnoreReflex {
				pc.Params.ParentIds = sg.Params.ParentIds
			}

			// We create as many predicate entity children as the length of uids for
			// this predicate.
			ul := pc.uidMatrix[idx]
			// noneEmptyUID will store indexes of non empty UIDs.
			// This will be used for indexing facet response
			var nonEmptyUID []int
			for childIdx, childUID := range ul.Uids {
				if fieldName == "" || (invalidUids != nil && invalidUids[childUID]) {
					continue
				}
				uc := enc.newFastJsonNodeWithAttr(enc.idForAttr(fieldName))
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
						enc.SetUID(uc, childUID, enc.idForAttr("uid"))
					}
					nonEmptyUID = append(nonEmptyUID, childIdx) // append index to nonEmptyUID.

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
							node := enc.newFastJsonNode()
							node.appendAttrs(enc, c)
							enc.AddListChild(dst, enc.idForAttr(fieldName), node)
						}
						continue
					}
					if pc.List {
						enc.AddListChild(dst, enc.idForAttr(fieldName), uc)
					} else {
						enc.AddMapChild(dst, enc.idForAttr(fieldName), uc, false)
					}
				}
			}

			// Now fill facets for non empty UIDs.
			facetIdx := 0
			for _, uidIdx := range nonEmptyUID {
				if pc.Params.Facet != nil && len(fcsList) > uidIdx {
					fs := fcsList[uidIdx]
					err := enc.attachFacets(dst, fieldName, pc.List, fs.Facets, facetIdx)
					if err != nil {
						return err
					}
					facetIdx++
				}
			}

			// add value for count(uid) nodes if any.
			_ = handleCountUIDNodes(pc, enc, dst, len(ul.Uids))
		default:
			if pc.Params.Alias == "" && len(pc.Params.Langs) > 0 && pc.Params.Langs[0] != "*" {
				fieldName += "@"
				fieldName += strings.Join(pc.Params.Langs, ":")
			}

			if pc.Attr == "uid" {
				enc.SetUID(dst, uid, enc.idForAttr(pc.fieldName()))
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
					encodeAsList := pc.List && len(lang) == 0
					enc.AddListValue(dst, enc.idForAttr(fieldNameWithTag), sv, encodeAsList)
					continue
				}

				encodeAsList := pc.List && len(pc.Params.Langs) == 0
				if !pc.Params.Normalize {
					enc.AddListValue(dst, enc.idForAttr(fieldName), sv, encodeAsList)
					continue
				}
				// If the query had the normalize directive, then we only add nodes
				// with an Alias.
				if pc.Params.Alias != "" {
					enc.AddListValue(dst, enc.idForAttr(fieldName), sv, encodeAsList)
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
		enc.SetUID(dst, uid, enc.idForAttr("uid"))
	}

	if sg.pathMeta != nil {
		totalWeight := types.Val{
			Tid:   types.FloatID,
			Value: sg.pathMeta.weight,
		}
		enc.AddValue(dst, enc.idForAttr("_weight_"), totalWeight)
	}

	return nil
}
