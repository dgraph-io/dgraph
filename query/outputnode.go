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
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	//"strconv"
	"time"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	//"github.com/dgraph-io/dgraph/x"
)

// ToProtocolBuf returns the list of protos.Node which would be returned to the go
// client.
func ToProtocolBuf(l *Latency, sgl []*SubGraph) ([]*protos.Node, error) {
	var resNode []*protos.Node
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
func ToJson(l *Latency, sgl []*SubGraph, w io.Writer, allocIds map[string]string,
	addLatency bool) error {
	sgr := &SubGraph{
		Attr: "__",
	}
	for _, sg := range sgl {
		if sg.Params.Alias == "var" || sg.Params.Alias == "shortest" {
			continue
		}
		if sg.Params.GetUid {
			sgr.Params.GetUid = true
		}
		sgr.Children = append(sgr.Children, sg)
	}
	return sgr.ToFastJSON(l, w, allocIds, addLatency)
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
}

// protoNode is the proto output for preTraverse.
type protoNode struct {
	*protos.Node
}

// AddValue adds an attribute value for protoOutputNode.
func (p *protoNode) AddValue(attr string, v types.Val) {
	p.Node.Properties = append(p.Node.Properties, createProperty(attr, v))
}

// AddMapChild adds a node value for protoOutputNode.
func (p *protoNode) AddMapChild(attr string, v outputNode, isRoot bool) {
	// Assert that attr == v.Node.Attribute
	var childNode *protos.Node
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
		}
		p.Node.Children = append(p.Node.Children, vParent.(*protoNode).Node)
	}
}

// AddListChild adds a child for protoOutputNode.
func (p *protoNode) AddListChild(attr string, child outputNode) {
	p.Node.Children = append(p.Node.Children, child.(*protoNode).Node)
}

// New creates a new node for protoOutputNode.
func (p *protoNode) New(attr string) outputNode {
	uc := nodePool.Get().(*protos.Node)
	uc.Attribute = attr
	return &protoNode{uc}
}

// SetUID sets UID of a protoOutputNode.
func (p *protoNode) SetUID(uid uint64, attr string) {
	p.AddValue(attr, types.Val{
		Tid:   types.UidID,
		Value: uid,
	})
}

func (p *protoNode) IsEmpty() bool {
	if len(p.Node.Children) > 0 {
		return false
	}
	if len(p.Node.Properties) > 0 {
		return false
	}
	return true
}

func mergeProto(parent [][]*protos.Property, child [][]*protos.Property) [][]*protos.Property {
	if len(parent) == 0 {
		return child
	}

	mergedLists := make([][]*protos.Property, 0, len(parent)*len(child))
	for _, pa := range parent {
		for _, ca := range child {
			mergeList := make([]*protos.Property, 0, len(parent))
			mergeList = append(mergeList, pa...)
			mergeList = append(mergeList, ca...)
			mergedLists = append(mergedLists, mergeList)
		}
	}
	return mergedLists
}

func (n *protoNode) normalize() [][]*protos.Property {
	if len(n.Children) == 0 {
		return [][]*protos.Property{n.Properties}
	}

	parentSlice := make([][]*protos.Property, 0, len(n.Properties))
	if len(n.Properties) > 0 {
		parentSlice = append(parentSlice, n.Properties)
	}

	// Temporary map, so that we can group children by attribute, similar to how
	// we have in JSON. Then we can call normalize on all children with same attribute,
	// aggregate results and merge them with the results of children with some other attribute.
	attrChildrenMap := make(map[string][]*protos.Node)
	for _, child := range n.Children {
		attrChildrenMap[child.Attribute] = append(attrChildrenMap[child.Attribute], child)
	}

	// A temporary slice in which we store the attrs and then sort them. We need this so that
	// the order of results is deterministic which wont be the case if we directly iterated over
	// the map.
	attrSlice := make([]string, 0, len(n.Children))
	for attr, _ := range attrChildrenMap {
		attrSlice = append(attrSlice, attr)
	}
	sort.Strings(attrSlice)

	for _, attr := range attrSlice {
		attrChildren := attrChildrenMap[attr]
		childSlice := make([][]*protos.Property, 0, 5)

		for _, child := range attrChildren {
			childSlice = append(childSlice, (&protoNode{child}).normalize()...)
		}
		parentSlice = mergeProto(parentSlice, childSlice)

	}
	return parentSlice
}

func (n *protoNode) addCountAtRoot(sg *SubGraph) {
	c := types.ValueForType(types.IntID)
	// This is count() without any attribute.
	c.Value = int64(len(sg.DestUIDs.Uids))
	n1 := n.New(sg.Params.Alias)
	n1.AddValue(sg.Params.uidCount, c)
	n.AddListChild(sg.Params.Alias, n1)
}

func (n *protoNode) addGroupby(sg *SubGraph, fname string) {
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

// ToProtocolBuffer does preorder traversal to build a proto buffer. We have
// used postorder traversal before, but preorder seems simpler and faster for
// most cases.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*protos.Node, error) {
	var seedNode *protoNode
	if sg.uidMatrix == nil {
		return seedNode.New(sg.Params.Alias).(*protoNode).Node, nil
	}

	n := seedNode.New("_root_")
	if sg.Params.uidCount != "" {
		n.addCountAtRoot(sg)
	}

	if sg.Params.isGroupBy {
		n.addGroupby(sg, sg.Params.Alias)
	} else {
		for _, uid := range sg.uidMatrix[0].Uids {
			// For the root, the name is stored in Alias, not Attr.
			if algo.IndexOf(sg.DestUIDs, uid) < 0 {
				// This UID was filtered. So Ignore it.
				continue
			}
			n1 := seedNode.New(sg.Params.Alias)

			if rerr := sg.preTraverse(uid, n1, n1); rerr != nil {
				if rerr.Error() == "_INV_" {
					continue
				}
				return n.(*protoNode).Node, rerr
			}
			if n1.IsEmpty() {
				continue
			}
			if !sg.Params.Normalize {
				n.AddListChild(sg.Params.Alias, n1)
				continue
			}

			// Lets normalize the response now.
			for _, c := range n1.(*protoNode).normalize() {
				n.AddListChild(sg.Params.Alias, &protoNode{&protos.Node{Properties: c}})
			}
		}
	}
	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n.(*protoNode).Node, nil
}

type fastJsonAttr struct {
	attr      string
	isScalar  bool
	scalarVal []byte
	nodeVal   *fastJsonNode
}

func makeScalarAttr(attr string, val []byte) *fastJsonAttr {
	return &fastJsonAttr{attr, true, val, nil}
}
func makeNodeAttr(attr string, val *fastJsonNode) *fastJsonAttr {
	return &fastJsonAttr{attr, false, nil, val}
}

type fastJsonNode struct {
	children map[string][]*fastJsonNode
	attrs    []*fastJsonAttr
}

func (fj *fastJsonNode) ensureChildrenMap() {
	if fj.children == nil {
		fj.children = make(map[string][]*fastJsonNode)
	}
}

func (fj *fastJsonNode) ensureAttrsMap() {
	if fj.attrs == nil {
		fj.attrs = []*fastJsonAttr{} // TODO(tzdybal) - we need to know size upfront
	}
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	if bs, err := valToBytes(v); err == nil {
		fj.ensureAttrsMap()
		fj.attrs = append(fj.attrs, makeScalarAttr(attr, bs))
	}
}

func (fj *fastJsonNode) AddMapChild(attr string, val outputNode, _ bool) {
	fj.ensureAttrsMap()
	fj.attrs = append(fj.attrs, makeNodeAttr(attr, val.(*fastJsonNode)))
}

func (fj *fastJsonNode) AddListChild(attr string, child outputNode) {
	fj.ensureChildrenMap()
	children, found := fj.children[attr]
	if !found {
		children = make([]*fastJsonNode, 0, 5)
	}
	fj.children[attr] = append(children, child.(*fastJsonNode))
}

func (fj *fastJsonNode) New(attr string) outputNode {
	return &fastJsonNode{}
}

func (fj *fastJsonNode) SetUID(uid uint64, attr string) {
	fj.ensureAttrsMap()
	found := false // TODO(tzdybal) - merge later!
	if found {
		/*
			x.AssertTruef(uidBs.isScalar, "Found node value for _uid_. Expected scalar value.")
			lUidBs := len(uidBs.scalarVal)
			currUid, err := strconv.ParseUint(string(uidBs.scalarVal[1:lUidBs-1]), 0, 64)
			x.AssertTruef(err == nil && currUid == uid, "Setting two different uids on same node.")
		*/
	} else {
		fj.attrs = append(fj.attrs, makeScalarAttr(attr, []byte(fmt.Sprintf("\"%#x\"", uid))))
	}
}

func (fj *fastJsonNode) IsEmpty() bool {
	return len(fj.attrs) == 0 && len(fj.children) == 0
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
		return []byte(fmt.Sprintf(`"%s"`, v.Value.(string))), nil
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
	fj.ensureChildrenMap()
	allKeys := make([]string, 0, len(fj.attrs)+len(fj.children))
	for _, a := range fj.attrs {
		allKeys = append(allKeys, a.attr)
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

		if v, ok := fj.children[k]; ok {
			first := true
			bufw.WriteRune('[')
			for _, vi := range v {
				if !first { // TODO(tzdybal) - optimize
					bufw.WriteRune(',')
				}
				first = false
				vi.encode(bufw)
			}
			bufw.WriteRune(']')
		} else {
			v := &fastJsonAttr{nodeVal: &fastJsonNode{}}
			scalar := false
			for _, a := range fj.attrs {
				if a.attr == k {
					if a.isScalar {
						bufw.Write(a.scalarVal)
						scalar = true
						break
					} else {
						v.nodeVal.ensureAttrsMap()
						v.nodeVal.ensureChildrenMap()

						for kk, vv := range a.nodeVal.children {
							v.nodeVal.children[kk] = vv
						}
						v.nodeVal.attrs = append(v.nodeVal.attrs, a.nodeVal.attrs...)
					}
				}
			}
			if !scalar {
				v.nodeVal.encode(bufw)
			}

		}
	}
	bufw.WriteRune('}')
}

func merge(parent [][]*fastJsonAttr,
	child [][]*fastJsonAttr) [][]*fastJsonAttr {
	if len(parent) == 0 {
		return child
	}

	// Here we merge two slices of maps.
	mergedList := make([][]*fastJsonAttr, 0, len(parent)*len(child))
	for _, pa := range parent {
		for _, ca := range child {
			mergeMap := make(map[string]*fastJsonAttr)
			for _, v := range pa {
				mergeMap[v.attr] = v
			}
			// Copy over child map entries to mergeMap created above.
			for _, v := range ca {
				mergeMap[v.attr] = v
			}
			// Add the map to the list.
			merged := make([]*fastJsonAttr, 0, len(mergeMap))
			for _, v := range mergeMap {
				merged = append(merged, v)
			}
			mergedList = append(mergedList, merged)
		}
	}
	return mergedList
}

func (n *fastJsonNode) normalize() [][]*fastJsonAttr {
	if len(n.children) == 0 {
		// Recursion base case
		// There are no children, we can just return slice with n.attrs map.
		return [][]*fastJsonAttr{n.attrs}
	}

	parentSlice := make([][]*fastJsonAttr, 0, 5)
	// If the parents has attrs, lets add them to the slice so that it can be
	// merged with children later.
	parentSlice = append(parentSlice, n.attrs)

	for _, childNodes := range n.children {
		childSlice := make([][]*fastJsonAttr, 0, 5)
		// Normalizing children.
		for _, childNode := range childNodes {
			childSlice = append(childSlice, childNode.normalize()...)
		}
		// Merging with parent.
		parentSlice = merge(parentSlice, childSlice)
	}
	return parentSlice
}

type attrVal struct {
	attr string
	val  *fastJsonAttr
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

func processNodeUids(n *fastJsonNode, sg *SubGraph) error {
	var seedNode *fastJsonNode
	if sg.uidMatrix == nil {
		return nil
	}

	if sg.Params.uidCount != "" {
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
		if err := sg.preTraverse(uid, n1, n1); err != nil {
			if err.Error() == "_INV_" {
				continue
			}
			return err
		}

		if n1.IsEmpty() {
			continue
		}

		if !sg.Params.Normalize {
			n.AddListChild(sg.Params.Alias, n1)
			continue
		}

		// Lets normalize the response now.
		for _, c := range n1.(*fastJsonNode).normalize() {
			n.AddListChild(sg.Params.Alias, &fastJsonNode{attrs: c})
		}
	}
	return nil
}

func (sg *SubGraph) ToFastJSON(l *Latency, w io.Writer, allocIds map[string]string, addLatency bool) error {
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
		err := processNodeUids(n.(*fastJsonNode), sg)
		if err != nil {
			return err
		}
	}

	if addLatency {
		sl := seedNode.New("serverLatency").(*fastJsonNode)
		for k, v := range l.ToMap() {
			val := types.ValueForType(types.StringID)
			val.Value = v
			sl.AddValue(k, val)
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
		n.AddMapChild("uids", sl, false)
	}

	bufw := bufio.NewWriter(w)
	n.(*fastJsonNode).encode(bufw)
	return bufw.Flush()
}
