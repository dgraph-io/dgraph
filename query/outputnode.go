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
	"time"

	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"

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

// ToJSON converts the internal subgraph object to JSON format which is then\
// sent to the HTTP client.
func (sg *SubGraph) ToJSON(l *Latency) ([]byte, error) {
	var seedNode *jsonOutputNode
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
	res := n.(*jsonOutputNode).data
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
	attrsWithChildren map[string][]*fastJsonNode
	attrs             map[string][]byte
}

func (fj *fastJsonNode) AddValue(attr string, v types.Val) {
	if bs, err := valToBytes(v); err == nil {
		fj.attrs[attr] = bs
	}
}

func (fj *fastJsonNode) AddChild(attr string, child outputNode) {
	_, found := fj.attrsWithChildren[attr]
	if !found {
		fj.attrsWithChildren[attr] = make([]*fastJsonNode, 0, 5)
	}
	fj.attrsWithChildren[attr] = append(fj.attrsWithChildren[attr], child.(*fastJsonNode))
}

func (fj *fastJsonNode) New(attr string) outputNode {
	return &fastJsonNode{make(map[string][]*fastJsonNode), make(map[string][]byte)}
}

func (fj *fastJsonNode) SetUID(uid uint64) {
	_, found := fj.attrs["_uid_"]
	if !found {
		fj.attrs["_uid_"] = []byte(fmt.Sprintf("\"%#x\"", uid))
	}
}

func (fj *fastJsonNode) SetXID(xid string) {
	fj.attrs["_xid_"] = []byte(xid)
}

func (fj *fastJsonNode) IsEmpty() bool {
	return len(fj.attrs) == 0 && len(fj.attrsWithChildren) == 0
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
	for k, _ := range fj.attrsWithChildren {
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
			v := fj.attrsWithChildren[k]
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

func (sg *SubGraph) ToFastJSON(l *Latency) ([]byte, error) {
	var seedNode *fastJsonNode
	n := seedNode.New("_root_")
	if sg.Attr == "_root_" {
		for _, sg := range sg.Children {
			for _, uid := range sg.DestUIDs.Uids {
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
		}
	} else {
		for _, uid := range sg.DestUIDs.Uids {
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
	}
	if sg.Params.isDebug {
		var buf bytes.Buffer
		sl := seedNode.New("serverLatency").(*fastJsonNode)
		for k, v := range l.ToMap() {
			sl.attrs[k] = []byte(fmt.Sprintf("%q", v))
		}

		sl.encode(&buf)
		n.(*fastJsonNode).attrs["server_latency"] = buf.Bytes()
	}

	var jsBuf bytes.Buffer
	n.(*fastJsonNode).encode(&jsBuf)

	return jsBuf.Bytes(), nil
}
