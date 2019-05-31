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
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/types"
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

// outputNode is the generic output / writer for preTraverse.
type outputNode interface {
	AddValue(attr string, v types.Val)
	AddListValue(attr string, v types.Val, list bool)
	AddMapChild(attr string, node outputNode, isRoot bool)
	AddListChild(attr string, child outputNode)
	New(attr string) outputNode
	SetUID(uid uint64, attr string)
	IsEmpty() bool

	addCountAtRoot(*SubGraph)
	addGroupby(*SubGraph, *groupResults, string)
	addAggregations(*SubGraph) error
}

func makeScalarNode(attr string, isChild bool, val []byte, list bool) *fastJsonNode {
	return &fastJsonNode{
		attr:      attr,
		isChild:   isChild,
		scalarVal: val,
		list:      list,
	}
}

type fastJsonNode struct {
	attr      string
	order     int // relative ordering (for sorted results)
	isChild   bool
	scalarVal []byte
	attrs     []*fastJsonNode
	list      bool
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	fj.AddListValue(attr, v, false)
}

func (fj *fastJsonNode) AddListValue(attr string, v types.Val, list bool) {
	if bs, err := valToBytes(v); err == nil {
		fj.attrs = append(fj.attrs, makeScalarNode(attr, false, bs, list))
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
	// if we're in debug mode, uid may be added second time, skip this
	if attr == "uid" {
		for _, a := range fj.attrs {
			if a.attr == attr {
				return
			}
		}
	}
	fj.attrs = append(fj.attrs, makeScalarNode(attr, false, []byte(fmt.Sprintf("\"%#x\"", uid)),
		false))
}

func (fj *fastJsonNode) IsEmpty() bool {
	return len(fj.attrs) == 0
}

func valToBytes(v types.Val) ([]byte, error) {
	switch v.Tid {
	case types.StringID, types.DefaultID:
		return json.Marshal(v.Value)
	case types.BinaryID:
		return []byte(fmt.Sprintf("%q", v.Value)), nil
	case types.IntID:
		return []byte(fmt.Sprintf("%d", v.Value)), nil
	case types.FloatID:
		return []byte(fmt.Sprintf("%f", v.Value)), nil
	case types.BoolID:
		if v.Value.(bool) {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case types.DateTimeID:
		// Return empty string instead of zero-time value string - issue#3166
		t := v.Value.(time.Time)
		if t.IsZero() {
			return []byte(`""`), nil
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

type nodeSlice []*fastJsonNode

func (n nodeSlice) Len() int {
	return len(n)
}

func (n nodeSlice) Less(i, j int) bool {
	cmp := strings.Compare(n[i].attr, n[j].attr)
	if cmp == 0 {
		return n[i].order < n[j].order
	}
	return cmp < 0
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
						if cur.isChild || cur.list {
							out.WriteRune('[')
							inArray = true
						}
					}
					cur.encode(out)
					if cnt != 1 || (cur.isChild || cur.list) {
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
				if (cur.isChild || cur.list) && !inArray {
					out.WriteRune('[')
				}
				cur.encode(out)
				if cnt != 1 || (cur.isChild || cur.list) {
					out.WriteRune(']')
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
			if cnt > x.Config.NormalizeNodeLimit {
				return nil, errors.Errorf(
					"Couldn't evaluate @normalize directive - too many results")
			}
			list := make([]*fastJsonNode, 0, len(pa)+len(ca))
			list = append(list, pa...)
			list = append(list, ca...)
			mergedList = append(mergedList, list)
		}
	}
	return mergedList, nil
}

func (fj *fastJsonNode) normalize() ([][]*fastJsonNode, error) {
	cnt := 0
	for _, a := range fj.attrs {
		if a.isChild {
			cnt++
		}
	}

	if cnt == 0 {
		// Recursion base case
		// There are no children, we can just return slice with fj.attrs map.
		return [][]*fastJsonNode{fj.attrs}, nil
	}

	parentSlice := make([][]*fastJsonNode, 0, 5)
	// If the parents has attrs, lets add them to the slice so that it can be
	// merged with children later.
	attrs := make([]*fastJsonNode, 0, len(fj.attrs)-cnt)
	for _, a := range fj.attrs {
		if !a.isChild {
			attrs = append(attrs, a)
		}
	}
	parentSlice = append(parentSlice, attrs)

	for ci := 0; ci < len(fj.attrs); {
		childNode := fj.attrs[ci]
		if !childNode.isChild {
			ci++
			continue
		}
		childSlice := make([][]*fastJsonNode, 0, 5)
		for ci < len(fj.attrs) && childNode.attr == fj.attrs[ci].attr {
			normalized, err := fj.attrs[ci].normalize()
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
			if slice[i].attr == "uid" {
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

func (fj *fastJsonNode) addGroupby(sg *SubGraph, res *groupResults, fname string) {
	// Don't add empty groupby
	if len(res.group) == 0 {
		return
	}
	g := fj.New(fname)
	for _, grp := range res.group {
		uc := g.New("@groupby")
		for _, it := range grp.keys {
			uc.AddValue(it.attr, it.key)
		}
		for _, it := range grp.aggregates {
			uc.AddValue(it.attr, it.key)
		}
		g.AddListChild("@groupby", uc)
	}
	fj.AddListChild(fname, g)
}

func (fj *fastJsonNode) addCountAtRoot(sg *SubGraph) {
	c := types.ValueForType(types.IntID)
	c.Value = int64(len(sg.DestUIDs.Uids))
	n1 := fj.New(sg.Params.Alias)
	field := sg.Params.uidCountAlias
	if field == "" {
		field = "count"
	}
	n1.AddValue(field, c)
	fj.AddListChild(sg.Params.Alias, n1)
}

func (fj *fastJsonNode) addAggregations(sg *SubGraph) error {
	for _, child := range sg.Children {
		aggVal, ok := child.Params.uidToVal[0]
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
		n1 := fj.New(fieldName)
		n1.AddValue(fieldName, aggVal)
		fj.AddListChild(sg.Params.Alias, n1)
	}
	if fj.IsEmpty() {
		fj.AddListChild(sg.Params.Alias, &fastJsonNode{})
	}
	return nil
}

func processNodeUids(fj *fastJsonNode, sg *SubGraph) error {
	var seedNode *fastJsonNode
	if sg.Params.IsEmpty {
		return fj.addAggregations(sg)
	}

	if sg.uidMatrix == nil {
		fj.AddListChild(sg.Params.Alias, &fastJsonNode{})
		return nil
	}

	hasChild := false
	if sg.Params.uidCount && !(sg.Params.uidCountAlias == "" && sg.Params.Normalize) {
		hasChild = true
		fj.addCountAtRoot(sg)
	}

	if sg.Params.isGroupBy {
		if len(sg.GroupbyRes) == 0 {
			return errors.Errorf("Expected GroupbyRes to have length > 0.")
		}
		fj.addGroupby(sg, sg.GroupbyRes[0], sg.Params.Alias)
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
			fj.AddListChild(sg.Params.Alias, n1)
			continue
		}

		// Lets normalize the response now.
		normalized, err := n1.(*fastJsonNode).normalize()
		if err != nil {
			return err
		}
		for _, c := range normalized {
			fj.AddListChild(sg.Params.Alias, &fastJsonNode{attrs: c})
		}
	}

	if !hasChild {
		// So that we return an empty key if the root didn't have any children.
		fj.AddListChild(sg.Params.Alias, &fastJsonNode{})
	}
	return nil
}

// Extensions represents the extra information appended to query results.
type Extensions struct {
	Latency *api.Latency    `json:"server_latency,omitempty"`
	Txn     *api.TxnContext `json:"txn,omitempty"`
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
	if len(n.(*fastJsonNode).attrs) == 0 {
		bufw.WriteString(`{}`)
	} else {
		n.(*fastJsonNode).encode(&bufw)
	}
	return bufw.Bytes(), nil
}
