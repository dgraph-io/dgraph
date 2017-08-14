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
	"log"
	"sort"
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

func mergeProto(parent [][]*protos.Property,
	child [][]*protos.Property) ([][]*protos.Property, error) {
	if len(parent) == 0 {
		return child, nil
	}

	mergedLists := make([][]*protos.Property, 0, len(parent)*len(child))
	cnt := 0
	for _, pa := range parent {
		for _, ca := range child {
			cnt += len(pa) + len(ca)
			if cnt > normalizeLimit {
				return nil, x.Errorf("Couldn't evaluate @normalize directive - to many results")
			}
			list := make([]*protos.Property, 0, len(pa)+len(ca))
			list = append(list, pa...)
			list = append(list, ca...)
			mergedLists = append(mergedLists, list)
		}
	}
	return mergedLists, nil
}

func (n *protoNode) normalize() ([][]*protos.Property, error) {
	if len(n.Children) == 0 {
		return [][]*protos.Property{n.Properties}, nil
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
			normalized, err := (&protoNode{child}).normalize()
			if err != nil {
				return nil, err
			}
			childSlice = append(childSlice, normalized...)
		}
		var err error
		parentSlice, err = mergeProto(parentSlice, childSlice)
		if err != nil {
			return nil, err
		}

	}
	return parentSlice, nil
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
	} else if sg.Params.isInternal {
		//n.addAggregations(sg)
	} else {
		for _, uid := range sg.uidMatrix[0].Uids {
			// For the root, the name is stored in Alias, not Attr.
			if algo.IndexOf(sg.DestUIDs, uid) < 0 {
				// This UID was filtered. So Ignore it.
				continue
			}
			n1 := seedNode.New(sg.Params.Alias)

			if rerr := sg.preTraverse(uid, n1); rerr != nil {
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
			normalized, err := n1.(*protoNode).normalize()
			if err != nil {
				return nil, err
			}
			for _, c := range normalized {
				n.AddListChild(sg.Params.Alias, &protoNode{&protos.Node{Properties: c}})
			}
		}
	}
	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n.(*protoNode).Node, nil
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

func (fj *fastJsonNode) writeKey(out *bufio.Writer) {
	out.WriteRune('"')
	out.WriteString(fj.attr)
	out.WriteRune('"')
	out.WriteRune(':')
}

func (fj *fastJsonNode) encode(out *bufio.Writer) {
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

func (n *fastJsonNode) addAggregations(sg *SubGraph) {
	for _, child := range sg.Children {
		aggVal, ok := child.Params.uidToVal[0]
		x.AssertTrue(ok)
		fieldName := aggWithVarFieldName(child)
		n1 := n.New(fieldName)
		n1.AddValue(fieldName, aggVal)
		n.AddMapChild(sg.Params.Alias, n1, true)
	}
}

func processNodeUids(n *fastJsonNode, sg *SubGraph) error {
	var seedNode *fastJsonNode
	if sg.Params.isInternal {
		n.addAggregations(sg)
		return nil
	}

	if sg.uidMatrix == nil {
		return nil
	}

	if sg.Params.uidCount != "" {
		n.addCountAtRoot(sg)
		return nil
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
		log.Fatal("here")
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
	if len(n.(*fastJsonNode).attrs) == 0 {
		bufw.WriteString(`{ "data": {} }`)
	} else {
		bufw.WriteString(`{"data": `)
		n.(*fastJsonNode).encode(bufw)
		bufw.WriteRune('}')
	}
	return bufw.Flush()
}
