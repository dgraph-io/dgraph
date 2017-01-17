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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
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

// protoNode is the proto output for preTraverse.
type protoNode struct {
	*graph.Node
}

// AddValue adds an attribute value for protoOutputNode.
func (p *protoNode) AddValue(attr string, v types.Val) {
	p.Node.Properties = append(p.Node.Properties, createProperty(attr, v))
}

// AddChild adds a child for protoOutputNode.
func (p *protoNode) AddChild(attr string, child outputNode) {
	p.Node.Children = append(p.Node.Children, child.(*protoNode).Node)
}

// New creates a new node for protoOutputNode.
func (p *protoNode) New(attr string) outputNode {
	uc := nodePool.Get().(*graph.Node)
	uc.Attribute = attr
	return &protoNode{uc}
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

// ToProtocolBuffer does preorder traversal to build a proto buffer. We have
// used postorder traversal before, but preorder seems simpler and faster for
// most cases.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*graph.Node, error) {
	var seedNode *protoNode
	if sg.DestUIDs == nil {
		return seedNode.New(sg.Params.Alias).(*protoNode).Node, nil
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
			return n.(*protoNode).Node, rerr
		}
		if n1.IsEmpty() {
			continue
		}
		n.AddChild(sg.Params.Alias, n1)
	}
	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n.(*protoNode).Node, nil
}

// jsonNode is the JSON output for preTraverse.
type jsonNode struct {
	data map[string]interface{}
}

// AddValue adds an attribute value for jsonOutputNode.
func (p *jsonNode) AddValue(attr string, v types.Val) {
	p.data[attr] = v
}

// AddChild adds a child for jsonOutputNode.
func (p *jsonNode) AddChild(attr string, child outputNode) {
	a := p.data[attr]
	if a == nil {
		// Need to do this because we cannot cast nil interface to
		// []map[string]interface{}.
		a = make([]map[string]interface{}, 0, 5)
	}
	p.data[attr] = append(a.([]map[string]interface{}),
		child.(*jsonNode).data)
}

// New creates a new node for jsonOutputNode.
func (p *jsonNode) New(attr string) outputNode {
	return &jsonNode{make(map[string]interface{})}
}

// SetUID sets UID of a jsonOutputNode.
func (p *jsonNode) SetUID(uid uint64) {
	_, found := p.data["_uid_"]
	if !found {
		p.data["_uid_"] = fmt.Sprintf("%#x", uid)
	}
}

// SetXID sets XID of a jsonOutputNode.
func (p *jsonNode) SetXID(xid string) {
	p.data["_xid_"] = xid
}

func (p *jsonNode) IsEmpty() bool {
	return len(p.data) == 0
}

// ToJSON converts the internal subgraph object to JSON format which is then\
// sent to the HTTP client.
func (sg *SubGraph) ToJSON(l *Latency) ([]byte, error) {
	var seedNode *jsonNode
	n := seedNode.New("_root_")
	for _, uid := range sg.DestUIDs.Uids {
		// For the root, the name is stored in Alias, not Attr.
		n1 := seedNode.New(sg.Params.Alias)
		if sg.Params.GetUID || sg.Params.isDebug {
			n1.SetUID(uid)
		}

		if err := sg.preTraverse(uid, n1); err != nil {
			if err.Error() == "_INV_" {
				continue
			}
			return nil, err
		}
		if n1.IsEmpty() {
			continue
		}
		n.AddChild(sg.Params.Alias, n1)
	}
	res := n.(*jsonNode).data
	if sg.Params.isDebug {
		res["server_latency"] = l.ToMap()
	}
	return json.Marshal(res)
}

// Implementation of jsonNode : we do encoding to json ourselves.
// /query$ go test -v -run=Benchmark outputnode_test.go query.go outputnode.go proto.go
// tojson:     Benchmarks: 30000 times; 2.361178574s total time; 78705 ns/op
// fastToJson: Benchmarks: 50000 times; 2.181345374s total time; 43626 ns/op
// /query$ go test -run=XXX -bench=Mock -benchtime=4s outputnode_test.go query.go
//           outputnode.go proto.go
// BenchmarkMockSubGraphFastJson-4   	  200000	     57500 ns/op
// BenchmarkMockSubGraphToJSON-4     	  100000	     98768 ns/op

type fastJsonNode struct {
	children map[string][]*fastJsonNode
	attrs    map[string][]byte
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	if bs, err := valToBytes(v); err == nil {
		fj.attrs[attr] = bs
	}
}

func (fj *fastJsonNode) AddChild(attr string, child outputNode) {
	children, found := fj.children[attr]
	if !found {
		children = make([]*fastJsonNode, 0, 5)
	}
	fj.children[attr] = append(children, child.(*fastJsonNode))
}

func (fj *fastJsonNode) New(attr string) outputNode {
	return &fastJsonNode{
		children: make(map[string][]*fastJsonNode),
		attrs:    make(map[string][]byte),
	}
}

func (fj *fastJsonNode) SetUID(uid uint64) {
	uidBs, found := fj.attrs["_uid_"]
	if found {
		lUidBs := len(uidBs)
		currUid, err := strconv.ParseUint(string(uidBs[1:lUidBs-1]), 0, 64)
		x.AssertTruef(err == nil && currUid == uid, "Setting two different uids on same node.")
	} else {
		fj.attrs["_uid_"] = []byte(fmt.Sprintf("\"%#x\"", uid))
	}
}

func (fj *fastJsonNode) SetXID(xid string) {
	fj.attrs["_xid_"] = []byte(xid)
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
	case types.StringID:
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
	default:
		return nil, errors.New("unsupported types.Val.Tid")
	}
}

func (fj *fastJsonNode) encode(jsBuf *bytes.Buffer) {
	allKeys := make([]string, 0, len(fj.attrs))
	for k, _ := range fj.attrs {
		allKeys = append(allKeys, k)
	}
	for k, _ := range fj.children {
		allKeys = append(allKeys, k)
	}
	sort.Strings(allKeys)

	jsBuf.WriteRune('{')
	first := true
	for _, k := range allKeys {
		if !first {
			jsBuf.WriteRune(',')
		}
		first = false
		jsBuf.WriteRune('"')
		jsBuf.WriteString(k)
		jsBuf.WriteRune('"')
		jsBuf.WriteRune(':')

		if v, ok := fj.attrs[k]; ok {
			jsBuf.Write(v)
		} else {
			v := fj.children[k]
			first := true
			jsBuf.WriteRune('[')
			for _, vi := range v {
				if !first {
					jsBuf.WriteRune(',')
				}
				first = false
				vi.encode(jsBuf)
			}
			jsBuf.WriteRune(']')
		}
	}
	jsBuf.WriteRune('}')
}

func processNodeUids(n *fastJsonNode, sg *SubGraph) error {
	var seedNode *fastJsonNode
	for _, uid := range sg.DestUIDs.Uids {
		n1 := seedNode.New(sg.Params.Alias)
		if sg.Params.GetUID || sg.Params.isDebug {
			n1.SetUID(uid)
		}
		if err := sg.preTraverse(uid, n1); err != nil {
			if err.Error() == "_INV_" {
				continue
			}
			return err
		}
		if n1.IsEmpty() {
			continue
		}
		n.AddChild(sg.Params.Alias, n1)
	}
	return nil
}

func (sg *SubGraph) ToFastJSON(l *Latency) ([]byte, error) {
	var seedNode *fastJsonNode
	n := seedNode.New("_root_")
	if sg.Attr == "_root_" {
		for _, sg := range sg.Children {
			err := processNodeUids(n.(*fastJsonNode), sg)
			if err != nil {
				return nil, err
			}
		}
	} else {
		err := processNodeUids(n.(*fastJsonNode), sg)
		if err != nil {
			return nil, err
		}
	}

	buf := bufferPool.Get()
	defer bufferPool.Put(buf)

	jsBuf := buf.(bytes.Buffer)
	if sg.Params.isDebug {
		sl := seedNode.New("serverLatency").(*fastJsonNode)
		for k, v := range l.ToMap() {
			sl.attrs[k] = []byte(fmt.Sprintf("%q", v))
		}

		jsBuf.Reset()
		sl.encode(&jsBuf)
		slBs := make([]byte, jsBuf.Len())
		copy(slBs, jsBuf.Bytes())
		n.(*fastJsonNode).attrs["server_latency"] = slBs
	}

	jsBuf.Reset()
	n.(*fastJsonNode).encode(&jsBuf)
	jsBs := make([]byte, jsBuf.Len())
	copy(jsBs, jsBuf.Bytes())
	return jsBs, nil
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		var buf bytes.Buffer
		buf.Grow(4096)
		return buf
	},
}
