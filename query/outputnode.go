/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	normalizeLimit = 10000
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

// outputNode is the generic output / writer for preTraverse.
type outputNode interface {
	AddValue(attr string, v types.Val)
	AddMapChild(attr string, node outputNode, isRoot bool)
	AddListChild(attr string, child outputNode)
	New(attr string) outputNode
	SetUID(uid uint64, attr string)
	IsEmpty() bool

	addCountAtRoot(*SubGraph)
	addGroupby(*SubGraph, string)
	addAggregations(*SubGraph) error
}

func makeScalarNode(attr string, isChild bool, val []byte) *fastJsonNode {
	return &fastJsonNode{
		attr:      attr,
		isChild:   isChild,
		scalarVal: val,
	}
}

func makeNestedNode(attr string, isChild bool, val *fastJsonNode) *fastJsonNode {
	return &fastJsonNode{
		attr:    attr,
		isChild: isChild,
		attrs:   []*fastJsonNode{val},
	}
}

type fastJsonNode struct {
	attr      string
	order     int // relative ordering (for sorted results)
	isChild   bool
	scalarVal []byte
	attrs     []*fastJsonNode
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	if bs, err := valToBytes(v); err == nil {
		fj.attrs = append(fj.attrs, makeScalarNode(attr, false, bs))
	}
}

func (fj *fastJsonNode) AddMapChild(attr string, val outputNode, isRoot bool) {
	var childNode *fastJsonNode
	for _, c := range fj.attrs {
		if c.attr == attr {
			childNode = c
			break
		}
	}

	if childNode != nil {
		val.(*fastJsonNode).isChild = true
		val.(*fastJsonNode).attr = attr
		childNode.attrs = append(childNode.attrs, val.(*fastJsonNode).attrs...)
	} else {
		val.(*fastJsonNode).isChild = false
		val.(*fastJsonNode).attr = attr
		fj.attrs = append(fj.attrs, val.(*fastJsonNode))
	}
}

func (fj *fastJsonNode) AddListChild(attr string, child outputNode) {
	child.(*fastJsonNode).attr = attr
	child.(*fastJsonNode).isChild = true
	fj.attrs = append(fj.attrs, child.(*fastJsonNode))
}

func (fj *fastJsonNode) New(attr string) outputNode {
	return &fastJsonNode{attr: attr, isChild: false}
}

func (fj *fastJsonNode) SetUID(uid uint64, attr string) {
	// if we're in debug mode, _uid_ may be added second time, skip this
	if attr == "_uid_" {
		for _, a := range fj.attrs {
			if a.attr == attr {
				return
			}
		}
	}
	fj.attrs = append(fj.attrs, makeScalarNode(attr, false, []byte(fmt.Sprintf("\"%#x\"", uid))))
}

func (fj *fastJsonNode) IsEmpty() bool {
	return len(fj.attrs) == 0
}

func valToBytes(v types.Val) ([]byte, error) {
	switch v.Tid {
	case types.BinaryID:
		// Encode to base64 and add "" around the value.
		b := fmt.Sprintf("%q", v.Value.([]byte))
		return []byte(b), nil
	case types.IntID:
		return []byte(fmt.Sprintf("%d", v.Value)), nil
	case types.FloatID:
		return []byte(fmt.Sprintf("%f", v.Value)), nil
	case types.BoolID:
		if v.Value.(bool) == true {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case types.StringID, types.DefaultID:
		return []byte(strconv.Quote(v.Value.(string))), nil
	case types.DateTimeID:
		return v.Value.(time.Time).MarshalJSON()
	case types.GeoID:
		return geojson.Marshal(v.Value.(geom.T))
	case types.UidID:
		return []byte(fmt.Sprintf("\"%#x\"", v.Value)), nil
	case types.PasswordID:
		return []byte(fmt.Sprintf("%q", v.Value.(string))), nil
	default:
		return nil, errors.New("unsupported types.Val.Tid")
	}
}

type nodeSlice []*fastJsonNode

func (n nodeSlice) Len() int {
	return len(n)
}

func (n nodeSlice) Less(i, j int) bool {
	cmp := strings.Compare(n[i].attr, n[j].attr)
	if cmp == 0 {
		return n[i].order < n[j].order
	} else {
		return cmp < 0
	}
}
func (n nodeSlice) Swap(i, j int) {
	n[i], n[j] = n[j], n[i]
}

func (fj *fastJsonNode) writeKey(out *bytes.Buffer) {
	out.WriteRune('"')
	out.WriteString(fj.attr)
	out.WriteRune('"')
	out.WriteRune(':')
}

func (fj *fastJsonNode) encode(out *bytes.Buffer) {
	// set relative ordering
	for i, a := range fj.attrs {
		a.order = i
	}

	i := 0
	if i < len(fj.attrs) {
		out.WriteRune('{')
		cur := fj.attrs[i]
		i++
		cnt := 1
		last := false
		inArray := false
		for {
			var next *fastJsonNode
			if i < len(fj.attrs) {
				next = fj.attrs[i]
				i++
			} else {
				last = true
			}

			if !last {
				if cur.attr == next.attr {
					if cnt == 1 {
						cur.writeKey(out)
						out.WriteRune('[')
						inArray = true
					}
					cur.encode(out)
					cnt++
				} else {
					if cnt == 1 {
						cur.writeKey(out)
						if cur.isChild {
							out.WriteRune('[')
							inArray = true
						}
					}
					cur.encode(out)
					if cnt != 1 || cur.isChild {
						out.WriteRune(']')
						inArray = false
					}
					cnt = 1
				}
				out.WriteRune(',')

				cur = next
			} else {
				if cnt == 1 {
					cur.writeKey(out)
				}
				if cur.isChild && !inArray {
					out.WriteRune('[')
				}
				cur.encode(out)
				if cnt != 1 || cur.isChild {
					out.WriteRune(']')
					inArray = false
				}
				break
			}
		}
		out.WriteRune('}')
	} else {
		out.Write(fj.scalarVal)
	}
}

func merge(parent [][]*fastJsonNode, child [][]*fastJsonNode) ([][]*fastJsonNode, error) {
	if len(parent) == 0 {
		return child, nil
	}

	// Here we merge two slices of maps.
	mergedList := make([][]*fastJsonNode, 0, len(parent)*len(child))
	cnt := 0
	for _, pa := range parent {
		for _, ca := range child {
			cnt += len(pa) + len(ca)
			if cnt > normalizeLimit {
				return nil, x.Errorf("Couldn't evaluate @normalize directive - to many results")
			}
			list := make([]*fastJsonNode, 0, len(pa)+len(ca))
			list = append(list, pa...)
			list = append(list, ca...)
			mergedList = append(mergedList, list)
		}
	}
	return mergedList, nil
}

func (n *fastJsonNode) normalize() ([][]*fastJsonNode, error) {
	cnt := 0
	for _, a := range n.attrs {
		if a.isChild {
			cnt++
		}
	}

	if cnt == 0 {
		// Recursion base case
		// There are no children, we can just return slice with n.attrs map.
		return [][]*fastJsonNode{n.attrs}, nil
	}

	parentSlice := make([][]*fastJsonNode, 0, 5)
	// If the parents has attrs, lets add them to the slice so that it can be
	// merged with children later.
	attrs := make([]*fastJsonNode, 0, len(n.attrs)-cnt)
	for _, a := range n.attrs {
		if !a.isChild {
			attrs = append(attrs, a)
		}
	}
	parentSlice = append(parentSlice, attrs)

	for ci := 0; ci < len(n.attrs); {
		childNode := n.attrs[ci]
		if !childNode.isChild {
			ci++
			continue
		}
		childSlice := make([][]*fastJsonNode, 0, 5)
		for ci < len(n.attrs) && childNode.attr == n.attrs[ci].attr {
			normalized, err := n.attrs[ci].normalize()
			if err != nil {
				return nil, err
			}
			childSlice = append(childSlice, normalized...)
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
		sort.Sort(nodeSlice(slice))

		first := -1
		last := 0
		for i := range slice {
			if slice[i].attr == "_uid_" {
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

type attrVal struct {
	attr string
	val  *fastJsonNode
}

func (n *fastJsonNode) addGroupby(sg *SubGraph, fname string) {
	g := n.New(fname)
	for _, grp := range sg.GroupbyRes.group {
		uc := g.New("@groupby")
		for _, it := range grp.keys {
			uc.AddValue(it.attr, it.key)
		}
		for _, it := range grp.aggregates {
			uc.AddValue(it.attr, it.key)
		}
		g.AddListChild("@groupby", uc)
	}
	n.AddListChild(fname, g)
}

func (n *fastJsonNode) addCountAtRoot(sg *SubGraph) {
	c := types.ValueForType(types.IntID)
	// This is count() without any attribute.
	c.Value = int64(len(sg.DestUIDs.Uids))
	n1 := n.New(sg.Params.Alias)
	n1.AddValue(sg.Params.uidCount, c)
	n.AddListChild(sg.Params.Alias, n1)
}

func (n *fastJsonNode) addAggregations(sg *SubGraph) error {
	for _, child := range sg.Children {
		aggVal, ok := child.Params.uidToVal[0]
		if !ok {
			return x.Errorf("Only aggregated variables allowed within empty block.")
		}
		fieldName := aggWithVarFieldName(child)
		n1 := n.New(fieldName)
		n1.AddValue(fieldName, aggVal)
		n.AddListChild(sg.Params.Alias, n1)
	}
	return nil
}

func processNodeUids(n *fastJsonNode, sg *SubGraph) error {
	var seedNode *fastJsonNode
	if sg.Params.IsEmpty {
		return n.addAggregations(sg)
	}

	if sg.uidMatrix == nil {
		n.AddListChild(sg.Params.Alias, &fastJsonNode{})
		return nil
	}

	hasChild := false
	if sg.Params.uidCount != "" {
		hasChild = true
		n.addCountAtRoot(sg)
	}

	if sg.Params.isGroupBy {
		n.addGroupby(sg, sg.Params.Alias)
		return nil
	}

	lenList := len(sg.uidMatrix[0].Uids)
	for i := 0; i < lenList; i++ {
		uid := sg.uidMatrix[0].Uids[i]
		if algo.IndexOf(sg.DestUIDs, uid) < 0 {
			// This UID was filtered. So Ignore it.
			continue
		}

		n1 := seedNode.New(sg.Params.Alias)
		if err := sg.preTraverse(uid, n1); err != nil {
			if err.Error() == "_INV_" {
				continue
			}
			return err
		}

		if n1.IsEmpty() {
			continue
		}

		hasChild = true
		if !sg.Params.Normalize {
			n.AddListChild(sg.Params.Alias, n1)
			continue
		}

		// Lets normalize the response now.
		normalized, err := n1.(*fastJsonNode).normalize()
		if err != nil {
			return err
		}
		for _, c := range normalized {
			n.AddListChild(sg.Params.Alias, &fastJsonNode{attrs: c})
		}
	}

	if !hasChild {
		// So that we return an empty key if the root didn't have any children.
		n.AddListChild(sg.Params.Alias, &fastJsonNode{})
	}
	return nil
}

type Extensions struct {
	Latency *protos.Latency    `json:"server_latency,omitempty"`
	Txn     *protos.TxnContext `json:"txn,omitempty"`
}

func (sg *SubGraph) toFastJSON(l *Latency) ([]byte, error) {
	defer func() {
		l.Json = time.Since(l.Start) - l.Parsing - l.Processing
	}()

	var seedNode *fastJsonNode
	var err error
	n := seedNode.New("_root_")
	for _, sg := range sg.Children {
		err = processNodeUids(n.(*fastJsonNode), sg)
		if err != nil {
			return nil, err
		}
	}

	// According to GraphQL spec response should only contain data, errors and extensions as top
	// level keys. Hence we send server_latency under extensions key.
	// https://facebook.github.io/graphql/#sec-Response-Format

	var bufw bytes.Buffer
	bufw.WriteString(`{`)
	bufw.WriteString(`"data": `)
	if len(n.(*fastJsonNode).attrs) == 0 {
		bufw.WriteString(`{}`)
	} else {
		n.(*fastJsonNode).encode(&bufw)
	}
	bufw.WriteString(`}`)
	return bufw.Bytes(), nil
}
