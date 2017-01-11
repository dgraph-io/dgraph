/*
 * Copyright 2017 DGraph Labs, Inc.
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
	//	"encoding/json"
	"bytes"
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/types"
)

// outputNode is the generic output / writer for preTraverse.
type outputNode interface {
	AddValue(attr string, v types.Val)
	AddChild(attr string, child outputNode)
	New(attr string) outputNode
	SetUID(uid uint64)
	SetXID(xid string)
	IsEmpty() bool
}

// protoOutputNode is the proto output for preTraverse.
type protoOutputNode struct {
	*graph.Node
}

// AddValue adds an attribute value for protoOutputNode.
func (p *protoOutputNode) AddValue(attr string, v types.Val) {
	p.Node.Properties = append(p.Node.Properties, createProperty(attr, v))
}

// AddChild adds a child for protoOutputNode.
func (p *protoOutputNode) AddChild(attr string, child outputNode) {
	p.Node.Children = append(p.Node.Children, child.(*protoOutputNode).Node)
}

// New creates a new node for protoOutputNode.
func (p *protoOutputNode) New(attr string) outputNode {
	uc := nodePool.Get().(*graph.Node)
	uc.Attribute = attr
	return &protoOutputNode{uc}
}

// SetUID sets UID of a protoOutputNode.
func (p *protoOutputNode) SetUID(uid uint64) { p.Node.Uid = uid }

// SetXID sets XID of a protoOutputNode.
func (p *protoOutputNode) SetXID(xid string) { p.Node.Xid = xid }

func (p *protoOutputNode) IsEmpty() bool {
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

// ToProtocolBuffer does preorder traversal to build a proto buffer. We have
// used postorder traversal before, but preorder seems simpler and faster for
// most cases.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*graph.Node, error) {
	var seedNode *protoOutputNode
	if sg.DestUIDs == nil {
		return seedNode.New(sg.Params.Alias).(*protoOutputNode).Node, nil
	}

	n := seedNode.New("_root_")
	for _, uid := range sg.DestUIDs.Uids {
		// For the root, the name is stored in Alias, not Attr.
		n1 := seedNode.New(sg.Params.Alias)
		if sg.Params.GetUID || sg.Params.isDebug {
			n1.SetUID(uid)
		}

		if rerr := sg.preTraverse(uid, n1); rerr != nil {
			if rerr.Error() == "_INV_" {
				continue
			}
			return n.(*protoOutputNode).Node, rerr
		}
		if n1.IsEmpty() {
			continue
		}
		n.AddChild(sg.Params.Alias, n1)
	}
	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n.(*protoOutputNode).Node, nil
}

// jsonOutputNode is the JSON output for preTraverse.
type jsonOutputNode struct {
	data map[string]interface{}
}

// AddValue adds an attribute value for jsonOutputNode.
func (p *jsonOutputNode) AddValue(attr string, v types.Val) {
	p.data[attr] = v
}

// AddChild adds a child for jsonOutputNode.
func (p *jsonOutputNode) AddChild(attr string, child outputNode) {
	a := p.data[attr]
	if a == nil {
		// Need to do this because we cannot cast nil interface to
		// []map[string]interface{}.
		a = make([]map[string]interface{}, 0, 5)
	}
	p.data[attr] = append(a.([]map[string]interface{}),
		child.(*jsonOutputNode).data)
}

// New creates a new node for jsonOutputNode.
func (p *jsonOutputNode) New(attr string) outputNode {
	return &jsonOutputNode{make(map[string]interface{})}
}

// SetUID sets UID of a jsonOutputNode.
func (p *jsonOutputNode) SetUID(uid uint64) {
	_, found := p.data["_uid_"]
	if !found {
		p.data["_uid_"] = fmt.Sprintf("%#x", uid)
	}
}

// SetXID sets XID of a jsonOutputNode.
func (p *jsonOutputNode) SetXID(xid string) {
	p.data["_xid_"] = xid
}

func (p *jsonOutputNode) IsEmpty() bool {
	return len(p.data) == 0
}

type fastJsonNode struct {
	data map[string][]string
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	fj.data[attr] = []string{valToString(v)}
}

func (fj *fastJsonNode) AddChild(attr string, child outputNode) {
	_, found := fj.data[attr]
	if !found {
		fj.data[attr] = make([]string, 0, 5)
	}
	bs := child.(*fastJsonNode).fastJsonNodeToJSON()
	// if err == nil {
	fj.data[attr] = append(fj.data[attr], string(bs))
	//}
}

func (fj *fastJsonNode) New(attr string) outputNode {
	return &fastJsonNode{make(map[string][]string)}
}

func (fj *fastJsonNode) SetUID(uid uint64) {
	_, found := fj.data["_uid_"]
	if !found {
		fj.data["_uid_"] = []string{fmt.Sprintf("%#x", uid)}
	}
}

func (fj *fastJsonNode) SetXID(xid string) {
	fj.data["_xid_"] = []string{xid}
}

func (fj *fastJsonNode) IsEmpty() bool {
	return len(fj.data) == 0
}

func valToString(v types.Val) string {
	switch v.Tid {
	case types.Int32ID:
		return fmt.Sprintf("%d", v.Value)
	case types.BoolID:
		if v.Value.(bool) == true {
			return "true"
		}
		return "false"
	case types.StringID:
		return v.Value.(string)
	default:
		return "not yet implemented"

	}
}

func (fj *fastJsonNode) fastJsonNodeToJSON() []byte {
	res := fj.data
	finalRes := make(map[string]string)
	for k, v := range res {
		if len(v) == 1 {
			finalRes[k] = "\"" + v[0] + "\""
		} else {
			var buf bytes.Buffer
			first := true
			for _, vi := range v {
				if !first {
					buf.WriteString(",")
				}
				first = false
				buf.WriteString(vi)
			}
			finalRes[k] = "[" + string(buf.Bytes()) + "]"
		}
	}

	var buf bytes.Buffer
	buf.WriteString("{")
	first := true
	for k, v := range finalRes {
		if !first {
			buf.WriteString(",")
		}
		first = false
		buf.WriteString("\"" + k + "\"")
		buf.WriteString(":")
		buf.WriteString(v)
	}
	buf.WriteString("}")

	return buf.Bytes()
}
