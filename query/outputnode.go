/*
 * Copyright 2017 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package query

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// ToProtocolBuf returns the list of graph.Node which would be returned to the go
// client.
func ToProtocolBuf(l *Latency, sgl []*SubGraph) ([]*graph.Node, error) {
	var resNode []*graph.Node
	for _, sg := range sgl {
		if sg.Params.Alias == "var" || sg.Params.Alias == "shortest" {
			continue
		}
		node, err := sg.ToProtocolBuffer(l)
		if err != nil {
			return nil, err
		}
		resNode = append(resNode, node)
	}
	return resNode, nil
}

// ToJson converts the list of subgraph into a JSON response by calling ToFastJSON.
func ToJson(l *Latency, sgl []*SubGraph, w io.Writer, allocIds map[string]string) error {
	sgr := &SubGraph{
		Attr: "__",
	}
	for _, sg := range sgl {
		if sg.Params.Alias == "var" || sg.Params.Alias == "shortest" {
			continue
		}
		if sg.Params.isDebug {
			sgr.Params.isDebug = true
		}
		sgr.Children = append(sgr.Children, sg)
	}
	return sgr.ToFastJSON(l, w, allocIds)
}

// outputNode is the generic output / writer for preTraverse.
type outputNode interface {
	AddValue(attr string, v types.Val)
	AddMapChild(attr string, node outputNode, isRoot bool)
	AddListChild(attr string, child outputNode)
	New(attr string) outputNode
	SetUID(uid uint64)
	SetXID(xid string)
	IsEmpty() bool
	Error() error
}

// protoNode is the proto output for preTraverse.
type protoNode struct {
	*graph.Node
	err error
}

// AddValue adds an attribute value for protoOutputNode.
func (p *protoNode) AddValue(attr string, v types.Val) {
	if p.err == nil {
		p.Node.Properties = append(p.Node.Properties, createProperty(attr, v))
	}
}

// AddMapChild adds a node value for protoOutputNode.
func (p *protoNode) AddMapChild(attr string, v outputNode, isRoot bool) {
	if p.err != nil {
		return
	}
	if v.Error() != nil {
		p.err = v.Error()
		return
	}
	var childNode *graph.Node
	var as []string
	for _, c := range p.Node.Children {
		as = append(as, c.Attribute)
		if c.Attribute == attr {
			childNode = c
			break
		}
	}
	if childNode != nil && isRoot {
		childNode.Children = append(childNode.Children, v.(*protoNode).Node)
	} else if childNode != nil {
		// merge outputNode into childNode
		vnode := v.(*protoNode).Node
		if !(vnode.Uid == childNode.Uid && vnode.Xid == childNode.Xid) {
			p.err = x.Errorf("Invalid nodes while merging.")
			return
		}
		for _, p := range vnode.Properties {
			childNode.Properties = append(childNode.Properties, p)
		}
		for _, c := range vnode.Children {
			childNode.Children = append(childNode.Children, c)
		}
	} else {
		vParent := v
		if isRoot {
			vParent = v.New(attr)
			vParent.AddListChild(attr, v)
			if vParent.Error() != nil {
				p.err = vParent.Error()
				return
			}
		}
		p.Node.Children = append(p.Node.Children, vParent.(*protoNode).Node)
	}
}

// AddListChild adds a child for protoOutputNode.
func (p *protoNode) AddListChild(attr string, child outputNode) {
	if p.err != nil {
		return
	}
	if child.Error() != nil {
		p.err = child.Error()
	} else {
		p.Node.Children = append(p.Node.Children, child.(*protoNode).Node)
	}
}

// New creates a new node for protoOutputNode.
func (p *protoNode) New(attr string) outputNode {
	uc := nodePool.Get().(*graph.Node)
	uc.Attribute = attr
	return &protoNode{uc, nil}
}

func (p *protoNode) Error() error {
	return p.err
}

// SetUID sets UID of a protoOutputNode.
func (p *protoNode) SetUID(uid uint64) { p.Node.Uid = uid }

// SetXID sets XID of a protoOutputNode.
func (p *protoNode) SetXID(xid string) { p.Node.Xid = xid }

func (p *protoNode) IsEmpty() bool {
	if p.Node.Uid > 0 {
		return false
	}
	if len(p.Node.Children) > 0 {
		return false
	}
	if len(p.Node.Properties) > 0 {
		return false
	}
	return true
}

func (n *protoNode) normalize(props []*graph.Property, out []protoNode) []protoNode {
	if len(n.Children) == 0 {
		props = append(props, n.Properties...)
		pn := protoNode{&graph.Node{Properties: props}, nil}
		out = append(out, pn)
		return out
	}

	for _, child := range n.Children {
		p := make([]*graph.Property, len(props))
		copy(p, props)
		p = append(p, n.Properties...)
		out = (&protoNode{child, nil}).normalize(p, out)
	}
	return out
}

// ToProtocolBuffer does preorder traversal to build a proto buffer. We have
// used postorder traversal before, but preorder seems simpler and faster for
// most cases.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*graph.Node, error) {
	var seedNode *protoNode
	if sg.uidMatrix == nil {
		return seedNode.New(sg.Params.Alias).(*protoNode).Node, nil
	}

	n := seedNode.New("_root_")
	it := algo.NewListIterator(sg.uidMatrix[0])
	for ; it.Valid(); it.Next() {
		uid := it.Val()
		// For the root, the name is stored in Alias, not Attr.
		n1 := seedNode.New(sg.Params.Alias)
		if sg.Params.GetUID || sg.Params.isDebug {
			n1.SetUID(uid)
		}

		if rerr := sg.preTraverse(uid, n1, n1); rerr != nil {
			if rerr.Error() == "_INV_" {
				continue
			}
			return n.(*protoNode).Node, rerr
		}
		if n1.Error() != nil {
			return nil, n1.Error()
		}
		if n1.IsEmpty() {
			continue
		}
		if !sg.Params.Normalize {
			n.AddListChild(sg.Params.Alias, n1)
			continue
		}

		// Lets normalize the response now.
		normalized := make([]protoNode, 0, 10)
		props := make([]*graph.Property, 0, 10)
		for _, c := range (n1.(*protoNode)).normalize(props, normalized) {
			n.AddListChild(sg.Params.Alias, &c)
		}
	}
	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n.(*protoNode).Node, n.Error()
}

type fastJsonAttr struct {
	isScalar  bool
	scalarVal []byte
	nodeVal   *fastJsonNode
}

func makeScalarAttr(val []byte) *fastJsonAttr {
	return &fastJsonAttr{true, val, nil}
}
func makeNodeAttr(val *fastJsonNode) *fastJsonAttr {
	return &fastJsonAttr{false, nil, val}
}

type fastJsonNode struct {
	children map[string][]*fastJsonNode
	attrs    map[string]*fastJsonAttr
	err      error
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	if fj.err != nil {
		return
	}
	if bs, err := valToBytes(v); err == nil {
		_, found := fj.attrs[attr]
		if found {
			fj.err = x.Errorf("Setting value %s twice for same attribute", attr)
			return
		}
		fj.attrs[attr] = makeScalarAttr(bs)
	} else {
		fj.err = err
	}
}

func (fj *fastJsonNode) AddMapChild(attr string, val outputNode, _ bool) {
	if fj.err != nil {
		return
	}
	if val.Error() != nil {
		fj.err = val.Error()
		return
	}
	nodeAttr, found := fj.attrs[attr]
	if found {
		if nodeAttr.isScalar {
			fj.err = x.Errorf("Can not merge scalar and node values for attr : %s.",
				attr)
		} else {
			// merge val and nodeAttr.nodeVal
			for k, v := range val.(*fastJsonNode).children {
				nodeAttr.nodeVal.children[k] = v
			}
			for k, v := range val.(*fastJsonNode).attrs {
				nodeAttr.nodeVal.attrs[k] = v
			}
		}
	} else {
		fj.attrs[attr] = makeNodeAttr(val.(*fastJsonNode))
	}
}

func (fj *fastJsonNode) AddListChild(attr string, child outputNode) {
	if fj.err != nil {
		return
	}
	if child.Error() != nil {
		fj.err = child.Error()
	}
	children, found := fj.children[attr]
	if !found {
		children = make([]*fastJsonNode, 0, 5)
	}
	fj.children[attr] = append(children, child.(*fastJsonNode))
}

func (fj *fastJsonNode) New(attr string) outputNode {
	return &fastJsonNode{
		children: make(map[string][]*fastJsonNode),
		attrs:    make(map[string]*fastJsonAttr),
		err:      nil,
	}
}

func (fj *fastJsonNode) Error() error {
	return fj.err
}

func (fj *fastJsonNode) SetUID(uid uint64) {
	if fj.err != nil {
		return
	}
	uidBs, found := fj.attrs["_uid_"]
	if found {
		if !uidBs.isScalar {
			fj.err = x.Errorf(
				"Found node value for _uid_ %d. Expected scalar value.", uid)
		} else {
			lUidBs := len(uidBs.scalarVal)
			currUid, err := strconv.ParseUint(string(uidBs.scalarVal[1:lUidBs-1]),
				0, 64)
			if err != nil || currUid != uid {
				fj.err = x.Errorf(
					"Setting two different uids %d and %d on same node.",
					currUid, uid)
			}
		}
	} else {
		fj.attrs["_uid_"] = makeScalarAttr([]byte(fmt.Sprintf("\"%#x\"", uid)))
	}
}

func (fj *fastJsonNode) SetXID(xid string) {
	fj.attrs["_xid_"] = makeScalarAttr([]byte(xid))
}

func (fj *fastJsonNode) IsEmpty() bool {
	return len(fj.attrs) == 0 && len(fj.children) == 0
}

func valToBytes(v types.Val) ([]byte, error) {
	switch v.Tid {
	case types.BinaryID:
		return v.Value.([]byte), nil
	case types.Int32ID:
		return []byte(fmt.Sprintf("%d", v.Value)), nil
	case types.FloatID:
		return []byte(fmt.Sprintf("%f", v.Value)), nil
	case types.BoolID:
		if v.Value.(bool) == true {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case types.StringID, types.DefaultID:
		return []byte(fmt.Sprintf("%q", v.Value.(string))), nil
	case types.DateID:
		s := v.Value.(time.Time).Format("2006-01-02")
		return json.Marshal(s)
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

func (fj *fastJsonNode) encode(bufw *bufio.Writer) {
	if fj.Error() != nil {
		return
	}
	allKeys := make([]string, 0, len(fj.attrs))
	for k := range fj.attrs {
		allKeys = append(allKeys, k)
	}
	for k := range fj.children {
		allKeys = append(allKeys, k)
	}
	sort.Strings(allKeys)

	bufw.WriteRune('{')
	first := true
	for _, k := range allKeys {
		if !first {
			bufw.WriteRune(',')
		}
		first = false
		bufw.WriteRune('"')
		bufw.WriteString(k)
		bufw.WriteRune('"')
		bufw.WriteRune(':')

		if v, ok := fj.attrs[k]; ok {
			if v.isScalar {
				bufw.Write(v.scalarVal)
			} else {
				v.nodeVal.encode(bufw)
			}
		} else {
			v := fj.children[k]
			first := true
			bufw.WriteRune('[')
			for _, vi := range v {
				if !first {
					bufw.WriteRune(',')
				}
				first = false
				vi.encode(bufw)
			}
			bufw.WriteRune(']')
		}
	}
	bufw.WriteRune('}')
}

func (n *fastJsonNode) normalize(av []attrVal, out []fastJsonNode) []fastJsonNode {
	if len(n.children) == 0 {
		// No more children nodes, lets copy the attrs to the slice and attach the
		// result to out.
		for k, v := range n.attrs {
			av = append(av, attrVal{k, v})
		}

		fn := fastJsonNode{
			attrs: make(map[string]*fastJsonAttr),
		}
		for _, pair := range av {
			fn.attrs[pair.attr] = pair.val
		}
		out = append(out, fn)
		return out
	}

	for _, child := range n.children {
		// n.children is a map of string -> []*fastJsonNode.
		for _, jn := range child {
			vals := make([]attrVal, len(av))
			copy(vals, av)
			// Create a copy of the attr-val slice, attach attrs and pass to children.
			for k, v := range n.attrs {
				vals = append(vals, attrVal{k, v})
			}
			out = jn.normalize(vals, out)
		}
	}
	return out
}

type attrVal struct {
	attr string
	val  *fastJsonAttr
}

func processNodeUids(n *fastJsonNode, sg *SubGraph) error {
	var seedNode *fastJsonNode
	if sg.uidMatrix == nil {
		return nil
	}
	it := algo.NewListIterator(sg.uidMatrix[0])
	for ; it.Valid(); it.Next() {
		uid := it.Val()
		n1 := seedNode.New(sg.Params.Alias)
		if sg.Params.GetUID || sg.Params.isDebug {
			n1.SetUID(uid)
		}
		if err := sg.preTraverse(uid, n1, n1); err != nil {
			if err.Error() == "_INV_" {
				continue
			}
			return err
		}
		if n1.Error() != nil {
			return n1.Error()
		}
		if n1.IsEmpty() {
			continue
		}

		if !sg.Params.Normalize {
			n.AddListChild(sg.Params.Alias, n1)
			continue
		}

		// Lets normalize the response now.
		normalized := make([]fastJsonNode, 0, 10)

		// This slice is used to mantain the leaf nodes along a path while traversing
		// the Subgraph.
		av := make([]attrVal, 0, 10)
		for _, c := range (n1.(*fastJsonNode)).normalize(av, normalized) {
			n.AddListChild(sg.Params.Alias, &fastJsonNode{attrs: c.attrs})
		}
	}
	return n.Error()
}

func (sg *SubGraph) ToFastJSON(l *Latency, w io.Writer, allocIds map[string]string) error {
	var seedNode *fastJsonNode
	n := seedNode.New("_root_")
	if sg.Attr == "__" {
		for _, sg := range sg.Children {
			err := processNodeUids(n.(*fastJsonNode), sg)
			if err != nil {
				return err
			}
		}
	} else {
		if err := processNodeUids(n.(*fastJsonNode), sg); err != nil {
			return err
		}
	}

	if sg.Params.isDebug {
		sl := seedNode.New("serverLatency").(*fastJsonNode)
		for k, v := range l.ToMap() {
			val := types.ValueForType(types.StringID)
			val.Value = v
			sl.AddValue(k, val)
		}
		if sl.Error() != nil {
			return sl.Error()
		}
		n.AddMapChild("server_latency", sl, false)
	}

	if allocIds != nil && len(allocIds) > 0 {
		sl := seedNode.New("uids").(*fastJsonNode)
		for k, v := range allocIds {
			val := types.ValueForType(types.StringID)
			val.Value = v
			sl.AddValue(k, val)
		}
		if sl.Error() != nil {
			return sl.Error()
		}
		n.AddMapChild("uids", sl, false)
	}

	bufw := bufio.NewWriter(w)
	n.(*fastJsonNode).encode(bufw)
	if n.Error() == nil {
		return bufw.Flush()
	}
	return n.Error()
}
