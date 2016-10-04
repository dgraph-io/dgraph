/*
 * Copyright 2015 DGraph Labs, Inc.
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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
	"github.com/google/flatbuffers/go"
)

/*
 * QUERY:
 * Let's take this query from GraphQL as example:
 * {
 *   me {
 *     id
 *     firstName
 *     lastName
 *     birthday {
 *       month
 *       day
 *     }
 *     friends {
 *       name
 *     }
 *   }
 * }
 *
 * REPRESENTATION:
 * This would be represented in SubGraph format internally, as such:
 * SubGraph [result uid = me]
 *    |
 *  Children
 *    |
 *    --> SubGraph [Attr = "xid"]
 *    --> SubGraph [Attr = "firstName"]
 *    --> SubGraph [Attr = "lastName"]
 *    --> SubGraph [Attr = "birthday"]
 *           |
 *         Children
 *           |
 *           --> SubGraph [Attr = "month"]
 *           --> SubGraph [Attr = "day"]
 *    --> SubGraph [Attr = "friends"]
 *           |
 *         Children
 *           |
 *           --> SubGraph [Attr = "name"]
 *
 * ALGORITHM:
 * This is a rough and simple algorithm of how to process this SubGraph query
 * and populate the results:
 *
 * For a given entity, a new SubGraph can be started off with NewGraph(id).
 * Given a SubGraph, is the Query field empty? [Step a]
 *   - If no, run (or send it to server serving the attribute) query
 *     and populate result.
 * Iterate over children and copy Result Uids to child Query Uids.
 *     Set Attr. Then for each child, use goroutine to run Step:a.
 * Wait for goroutines to finish.
 * Return errors, if any.
 */

// Latency is used to keep track of the latency involved in parsing and processing
// the query. It also contains information about the time it took to convert the
// result into a format(JSON/Protocol Buffer) that the client expects.
type Latency struct {
	Start          time.Time     `json:"-"`
	Parsing        time.Duration `json:"query_parsing"`
	Processing     time.Duration `json:"processing"`
	Json           time.Duration `json:"json_conversion"`
	ProtocolBuffer time.Duration `json:"pb_conversion"`
}

// ToMap converts the latency object to a map.
func (l *Latency) ToMap() map[string]string {
	m := make(map[string]string)
	j := time.Since(l.Start) - l.Processing - l.Parsing
	m["parsing"] = l.Parsing.String()
	m["processing"] = l.Processing.String()
	m["json"] = j.String()
	m["total"] = time.Since(l.Start).String()
	return m
}

type params struct {
	AttrType types.Type
	Alias    string
	Count    int
	Offset   int
	AfterUid uint64
	GetCount uint16
	GetUid   bool
	isDebug  bool
}

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr     string
	Children []*SubGraph
	Params   params
	Filter   *gql.FilterTree

	Counts *task.CountList
	Values task.ValueList
	Result []*algo.UIDList

	// srcUIDs is a list of unique source UIDs. They are always copies of destUIDs
	// of parent nodes in GraphQL structure.
	srcUIDs *algo.UIDList

	// destUIDs is a list of destination UIDs, after applying filters, pagination.
	destUIDs *algo.UIDList
}

func mergeInterfaces(i1 interface{}, i2 interface{}) interface{} {
	switch i1.(type) {
	case map[string]interface{}:
		m1 := i1.(map[string]interface{})
		if m2, ok := i2.(map[string]interface{}); ok {
			for k1, v1 := range m1 {
				m2[k1] = v1
			}
			return m2
		}
		break
	}
	return []interface{}{i1, i2}
}

// postTraverse traverses the subgraph recursively and returns final result for the query.
func postTraverse(sg *SubGraph) (map[uint64]interface{}, error) {
	if sg.srcUIDs.Size() == 0 {
		return nil, nil
	}
	result := make(map[uint64]interface{})
	// Get results from all children first.
	cResult := make(map[uint64]interface{})

	// ignoreUids would contain those UIDs whose scalar childlren dont obey the
	// types specified in the schema.
	ignoreUids := make(map[uint64]bool)
	for _, child := range sg.Children {
		m, err := postTraverse(child)
		if err != nil {
			return result, err
		}

		// condFlag is used to ensure that the child is a required part of the
		// current node according to schema. Only then should we check for its
		// validity.
		var condFlag bool
		if obj, ok := schema.TypeOf(sg.Attr).(types.Object); ok {
			if _, ok := obj.Fields[child.Attr]; ok {
				condFlag = true
			}
		}

		// Merge results from all children, one by one.
		for k, v := range m {
			if _, ok := v.(map[string]interface{})["_inv_"]; ok {
				if condFlag {
					ignoreUids[k] = true
					delete(cResult, k)
				} else {
					continue
				}
			}
			if ignoreUids[k] {
				continue
			}

			if val, present := cResult[k]; !present {
				cResult[k] = v
			} else {
				cResult[k] = mergeInterfaces(val, v)
			}
		}
	}

	r := sg.Result
	x.Assertf(sg.srcUIDs.Size() == len(r),
		"Result uidmatrixlength: %v. Query uidslength: %v", sg.srcUIDs.Size(), len(r))
	x.Assertf(sg.srcUIDs.Size() == sg.Values.ValuesLength(),
		"Result valuelength: %v. Query uidslength: %v", sg.srcUIDs.Size(), sg.Values.ValuesLength())

	// Generate a matrix of maps
	// Row -> .....
	// Col
	//  |
	//  v
	//  map{_uid_ = uid}
	// If some result is present from children results, then merge.
	// Otherwise, this would only contain the _uid_ property.
	// result[uid in row] = map[cur attribute ->
	//                          list of maps of {uid, uid + children result}]
	//

	if sg.Counts != nil && sg.Counts.CountLength() > 0 {
		for i := 0; i < sg.Counts.CountLength(); i++ {
			co := sg.Counts.Count(i)
			m := make(map[string]interface{})
			m["_count_"] = co
			mp := make(map[string]interface{})
			if sg.Params.Alias != "" {
				mp[sg.Params.Alias] = m
			} else {
				mp[sg.Attr] = m
			}
			result[sg.srcUIDs.Get(i)] = mp
		}
	}

	for i, ul := range r {
		l := make([]interface{}, 0, ul.Size())

		// We want to intersect ul.Uids with sg.destUIDs. Both are sorted.
		// We need to maintain an index into sg.destUIDs, to do the intersection.
		var destIdx int
		destSize := sg.destUIDs.Size()
		for j := 0; j < ul.Size(); j++ {
			uid := ul.Get(j)
			for ; destIdx < destSize && sg.destUIDs.Get(destIdx) < uid; destIdx++ {
			}
			if destIdx >= destSize || sg.destUIDs.Get(destIdx) > uid {
				continue
			}

			m := make(map[string]interface{})
			if sg.Params.GetUid || sg.Params.isDebug {
				m["_uid_"] = fmt.Sprintf("%#x", uid)
			}
			if ival, present := cResult[uid]; !present {
				l = append(l, m)
			} else {
				l = append(l, mergeInterfaces(m, ival))
			}
		}
		if len(l) > 0 {
			m := make(map[string]interface{})
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = l
			} else {
				m[sg.Attr] = l
			}
			result[sg.srcUIDs.Get(i)] = m
		}
	}

	values := sg.Values
	var tv task.Value
	for i := 0; i < values.ValuesLength(); i++ {
		if ok := values.Values(&tv, i); !ok {
			return result, x.Errorf("While parsing value")
		}
		valBytes := tv.ValBytes()
		m := make(map[string]interface{})
		if bytes.Equal(valBytes, nil) {

			// We do this, because we typically do set values, even though
			// they might be nil. This is to ensure that the index of the query uids
			// and the index of the results can remain in sync.
			if sg.Params.AttrType != nil && sg.Params.AttrType.IsScalar() {
				m["_inv_"] = true
				result[sg.srcUIDs.Get(i)] = m
			}
			continue
		}
		val, storageType, err := getValue(tv)
		if err != nil {
			return result, err
		}

		if pval, present := result[sg.srcUIDs.Get(i)]; present {
			log.Fatalf("prev: %v _uid_: %v new: %v"+
				" Previous value detected. A uid -> list of uids / value. Not both",
				pval, sg.srcUIDs.Get(i), val)
		}
		if sg.Params.GetUid || sg.Params.isDebug {
			m["_uid_"] = fmt.Sprintf("%#x", sg.srcUIDs.Get(i))
		}
		if sg.Params.AttrType == nil {
			// No type defined for attr in type system/schema, hence return the original value
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = val
			} else {
				m[sg.Attr] = val
			}
		} else {
			// type assertion for scalar type values
			if !sg.Params.AttrType.IsScalar() {
				return result, x.Errorf("Unknown Scalar:%v. Leaf predicate:'%v' must be"+
					" one of the scalar types defined in the schema.", sg.Params.AttrType, sg.Attr)
			}
			schemaType := sg.Params.AttrType.(types.Scalar)
			lval := val
			if schemaType != storageType {
				// The schema and storage types do not match, so we do a type conversion.
				var err error
				lval, err = schemaType.Convert(val)
				if err != nil {
					// We ignore schema conversion errors and not include the values in the result
					m["_inv_"] = true
					result[sg.srcUIDs.Get(i)] = m
					continue
				}
			}
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = lval
			} else {
				m[sg.Attr] = lval
			}
		}
		result[sg.srcUIDs.Get(i)] = m
	}
	return result, nil
}

// gets the value from the task.
func getValue(tv task.Value) (types.TypeValue, types.Type, error) {
	vType := tv.ValType()
	valBytes := tv.ValBytes()
	stype, _ := types.TypeForID(types.TypeID(vType))
	if stype == nil {
		return nil, nil, x.Errorf("Invalid type: %v", vType)
	}
	if !stype.IsScalar() {
		return nil, nil, x.Errorf("Unknown scalar type :%v", vType)
	}
	val, err := stype.(types.Scalar).Unmarshaler.FromBinary(valBytes)
	if err != nil {
		return nil, nil, err
	}
	return val, stype, nil
}

// ToJSON converts the internal subgraph object to JSON format which is then sent
// to the HTTP client.
func (sg *SubGraph) ToJSON(l *Latency) ([]byte, error) {
	r, err := postTraverse(sg)
	if err != nil {
		return nil, err
	}
	l.Json = time.Since(l.Start) - l.Parsing - l.Processing
	if len(r) != 1 {
		log.Fatal("We don't currently support more than 1 uid at root.")
	}

	// r is a map, and we don't know it's key. So iterate over it, even though it only has 1 result.
	for _, ival := range r {
		var m map[string]interface{}
		if ival != nil {
			m = ival.(map[string]interface{})
		} else {
			m = make(map[string]interface{})
		}
		if sg.Params.isDebug {
			m["server_latency"] = l.ToMap()
		}
		return json.Marshal(m)
	}
	log.Fatal("Runtime should never reach here.")
	return nil, x.Errorf("Runtime should never reach here.")
}

var nodePool = sync.Pool{
	New: func() interface{} {
		return &graph.Node{}
	},
}

var nodeCh chan *graph.Node

func release() {
	for n := range nodeCh {
		// In case of mutations, n is nil
		if n == nil {
			continue
		}
		for i := 0; i < len(n.Children); i++ {
			nodeCh <- n.Children[i]
		}
		*n = graph.Node{}
		nodePool.Put(n)
	}
}

func init() {
	nodeCh = make(chan *graph.Node, 1000)
	go release()
}

// This method gets the values and children for a subgraph.
func (sg *SubGraph) preTraverse(uid uint64, dst *graph.Node) error {
	var properties []*graph.Property
	var children []*graph.Node

	invalidUids := make(map[uint64]bool)
	// We go through all predicate children of the subgraph.
	for _, pc := range sg.Children {
		idx := pc.srcUIDs.IndexOf(uid)

		if idx == -1 {
			log.Fatal("Attribute with uid not found in child Query uids.")
			return x.Errorf("Attribute with uid not found")
		}

		ul := pc.Result[idx]
		var tv task.Value

		if sg.Counts != nil && sg.Counts.CountLength() > 0 {
			p := createProperty("_count_", types.Int32(sg.Counts.Count(idx)))
			uc := &graph.Node{
				Attribute:  pc.Attr,
				Properties: []*graph.Property{p},
			}
			children = append(children, uc)

		} else if ul.Size() > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			var destIdx int // Index into pc.destUIDs.
			destSize := pc.destUIDs.Size()
			for i := 0; i < ul.Size(); i++ {
				uid := ul.Get(i)
				for ; destIdx < destSize && pc.destUIDs.Get(destIdx) < uid; destIdx++ {
				}
				if destIdx >= destSize || pc.destUIDs.Get(destIdx) > uid {
					continue
				}
				uc := nodePool.Get().(*graph.Node)
				uc.Attribute = pc.Attr
				if sg.Params.GetUid || sg.Params.isDebug {
					uc.Uid = uid
				}
				if rerr := pc.preTraverse(uid, uc); rerr != nil {
					if rerr.Error() == "_INV_" {
						invalidUids[uid] = true
					} else {
						log.Printf("Error while traversal: %v", rerr)
						return rerr
					}
				}
				if !invalidUids[uid] {
					children = append(children, uc)
				}
			}
		} else {
			if ok := pc.Values.Values(&tv, idx); !ok {
				return x.Errorf("While parsing value")
			}

			valBytes := tv.ValBytes()
			v, storageType, err := getValue(tv)
			if err != nil {
				return err
			}

			if pc.Attr == "_xid_" {
				txt, err := v.MarshalText()
				if err != nil {
					return err
				}
				dst.Xid = string(txt)
				// We don't want to add _uid_ to properties map.
			} else if pc.Attr == "_uid_" {
				continue
			} else if pc.Params.AttrType != nil {
				//do type checking on response values
				// type assertion for scalar type values
				if !pc.Params.AttrType.IsScalar() {
					return x.Errorf("Unknown Scalar:%v. Leaf predicate:'%v' must be"+
						" one of the scalar types defined in the schema.", pc.Params.AttrType, pc.Attr)
				}
				schemaType := pc.Params.AttrType.(types.Scalar)
				sv := v
				if schemaType != storageType {
					// schema types don't match so we convert
					var err error
					sv, err = schemaType.Convert(v)
					if bytes.Equal(valBytes, nil) || err != nil {
						// skip values that don't convert.
						return x.Errorf("_INV_")
					}
				}
				p := createProperty(pc.Attr, sv)
				properties = append(properties, p)
			} else {
				p := createProperty(pc.Attr, v)
				properties = append(properties, p)
			}
		}
	}

	dst.Properties, dst.Children = properties, children
	return nil
}

func createProperty(prop string, v types.TypeValue) *graph.Property {
	pval := toProtoValue(v)
	return &graph.Property{Prop: prop, Value: pval}
}

// ToProtocolBuffer method transforms the predicate based subgraph to an
// predicate-entity based protocol buffer subgraph.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*graph.Node, error) {
	n := &graph.Node{
		Attribute: sg.Attr,
	}
	if sg.srcUIDs == nil {
		return n, nil
	}

	x.Assert(len(sg.Result) == 1)
	ul := sg.Result[0]
	if sg.Params.GetUid || sg.Params.isDebug {
		n.Uid = ul.Get(0)
	}

	if rerr := sg.preTraverse(ul.Get(0), n); rerr != nil {
		return n, rerr
	}

	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n, nil
}

func isPresent(list []string, str string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func treeCopy(ctx context.Context, gq *gql.GraphQuery, sg *SubGraph) error {
	// Typically you act on the current node, and leave recursion to deal with
	// children. But, in this case, we don't want to muck with the current
	// node, because of the way we're dealing with the root node.
	// So, we work on the children, and then recurse for grand children.

	var scalars []string
	// Add scalar children nodes based on schema
	if obj, ok := sg.Params.AttrType.(types.Object); ok {
		// Add scalar fields in the level to children
		list := schema.ScalarList(obj.Name)
		for _, it := range list {
			args := params{
				AttrType: it.Typ,
				isDebug:  sg.Params.isDebug,
			}
			dst := &SubGraph{
				Attr:   it.Field,
				Params: args,
			}
			sg.Children = append(sg.Children, dst)
			scalars = append(scalars, it.Field)
		}
	}

	for _, gchild := range gq.Children {
		if isPresent(scalars, gchild.Attr) {
			continue
		}
		if gchild.Attr == "_count_" {
			if len(gq.Children) > 1 {
				return errors.New("Cannot have other attributes with count")
			}
			if gchild.Children != nil {
				return errors.New("Count cannot have other attributes")
			}
			sg.Params.GetCount = 1
			break
		}
		if gchild.Attr == "_uid_" {
			sg.Params.GetUid = true
		}

		// Determine the type of current node.
		var attrType types.Type
		if sg.Params.AttrType != nil {
			if objType, ok := sg.Params.AttrType.(types.Object); ok {
				attrType = schema.TypeOf(objType.Fields[gchild.Attr])
			}
		} else {
			// Child is explicitly specified as some type.
			if objType := schema.TypeOf(gchild.Attr); objType != nil {
				if o, ok := objType.(types.Object); ok && o.Name == gchild.Attr {
					attrType = objType
				}
			}
		}
		args := params{
			AttrType: attrType,
			Alias:    gchild.Alias,
			isDebug:  sg.Params.isDebug,
		}
		dst := &SubGraph{
			Attr:   gchild.Attr,
			Params: args,
			Filter: gchild.Filter,
		}
		if v, ok := gchild.Args["offset"]; ok {
			offset, err := strconv.ParseInt(v, 0, 32)
			if err != nil {
				return err
			}
			dst.Params.Offset = int(offset)
		}
		if v, ok := gchild.Args["after"]; ok {
			after, err := strconv.ParseInt(v, 0, 64)
			if err != nil {
				return err
			}
			dst.Params.AfterUid = uint64(after)
		}
		if v, ok := gchild.Args["first"]; ok {
			first, err := strconv.ParseInt(v, 0, 32)
			if err != nil {
				return err
			}
			dst.Params.Count = int(first)
		}
		sg.Children = append(sg.Children, dst)
		err := treeCopy(ctx, gchild, dst)
		if err != nil {
			return err
		}
	}
	return nil
}

// ToSubGraph converts the GraphQuery into the internal SubGraph instance type.
func ToSubGraph(ctx context.Context, gq *gql.GraphQuery) (*SubGraph, error) {
	sg, err := newGraph(ctx, gq)
	if err != nil {
		return nil, err
	}
	err = treeCopy(ctx, gq, sg)
	return sg, err
}

// newGraph returns the SubGraph and its task query.
func newGraph(ctx context.Context, gq *gql.GraphQuery) (*SubGraph, error) {
	euid, exid := gq.UID, gq.XID
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.
	if len(exid) > 0 {
		x.Assertf(!strings.HasPrefix(exid, "_new_:"), "Query shouldn't contain _new_")
		euid = farm.Fingerprint64([]byte(exid))
		x.Trace(ctx, "Xid: %v Uid: %v", exid, euid)
	}

	if euid == 0 {
		err := x.Errorf("Invalid query, query internal id is zero")
		x.TraceError(ctx, err)
		return nil, err
	}

	// sg is to be returned.
	args := params{
		AttrType: schema.TypeOf(gq.Attr),
		isDebug:  gq.Attr == "debug",
	}
	sg := &SubGraph{
		Attr:    gq.Attr,
		Params:  args,
		Filter:  gq.Filter,
		srcUIDs: algo.NewUIDList([]uint64{euid}),
	}

	{
		// Encode uid into result flatbuffer.
		b := flatbuffers.NewBuilder(0)
		b.Finish(x.UidlistOffset(b, []uint64{euid}))
		buf := b.FinishedBytes()
		tl := new(task.UidList)
		x.ParseUidList(tl, buf)
		ul := new(algo.UIDList)
		ul.FromTask(tl)
		sg.Result = []*algo.UIDList{ul}
	}

	{
		// Also need to add nil value to keep this consistent.
		b := flatbuffers.NewBuilder(0)
		bvo := b.CreateByteVector(x.Nilbyte)
		task.ValueStart(b)
		task.ValueAddVal(b, bvo)
		voffset := task.ValueEnd(b)

		task.ValueListStartValuesVector(b, 1)
		b.PrependUOffsetT(voffset)
		voffset = b.EndVector(1)

		task.ValueListStart(b)
		task.ValueListAddValues(b, voffset)
		b.Finish(task.ValueListEnd(b))
		buf := b.FinishedBytes()
		x.ParseValueList(&sg.Values, buf)
	}
	return sg, nil
}

// createTaskQuery generates the query buffer.
func createTaskQuery(sg *SubGraph, uids *algo.UIDList, terms []string, intersect *algo.UIDList) []byte {
	x.Assert(uids == nil || terms == nil)

	b := flatbuffers.NewBuilder(0)
	var vend flatbuffers.UOffsetT
	if uids != nil {
		task.QueryStartUidsVector(b, uids.Size())
		for i := uids.Size() - 1; i >= 0; i-- {
			b.PrependUint64(uids.Get(i))
		}
		vend = b.EndVector(uids.Size())
	} else {
		offsets := make([]flatbuffers.UOffsetT, 0, len(terms))
		for _, term := range terms {
			offsets = append(offsets, b.CreateString(term))
		}
		task.QueryStartTermsVector(b, len(terms))
		for i := len(terms) - 1; i >= 0; i-- {
			b.PrependUOffsetT(offsets[i])
		}
		vend = b.EndVector(len(terms))
	}

	var intersectOffset flatbuffers.UOffsetT
	if intersect != nil {
		x.Assert(uids == nil)
		x.Assert(len(terms) > 0)
		intersectOffset = intersect.UidlistOffset(b)
	}

	ao := b.CreateString(sg.Attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	if uids != nil {
		task.QueryAddUids(b, vend)
	} else {
		task.QueryAddTerms(b, vend)
	}
	if intersect != nil {
		task.QueryAddIntersect(b, intersectOffset)
	}
	task.QueryAddCount(b, int32(sg.Params.Count))
	task.QueryAddOffset(b, int32(sg.Params.Offset))
	task.QueryAddAfterUid(b, sg.Params.AfterUid)
	task.QueryAddGetCount(b, sg.Params.GetCount)

	b.Finish(task.QueryEnd(b))
	return b.FinishedBytes()
}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances. Note: taskQuery is nil for root node.
func ProcessGraph(ctx context.Context, sg *SubGraph, taskQuery []byte, rch chan error) {
	var err error
	if taskQuery != nil {
		resultBuf, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
			rch <- err
			return
		}

		r := new(task.Result)
		x.ParseTaskResult(r, resultBuf)

		// Extract UIDLists from task.Result.
		sg.Result = make([]*algo.UIDList, r.UidmatrixLength())
		for i := 0; i < r.UidmatrixLength(); i++ {
			tl := new(task.UidList)
			x.Assert(r.Uidmatrix(tl, i))
			ul := new(algo.UIDList)
			ul.FromTask(tl)
			sg.Result[i] = ul
		}

		// Extract values from task.Result.
		x.Assert(r.Values(&sg.Values) != nil)
		if sg.Values.ValuesLength() > 0 {
			var v task.Value
			if sg.Values.Values(&v, 0) {
				x.Trace(ctx, "Sample value for attr: %v Val: %v", sg.Attr, string(v.ValBytes()))
			}
		}

		// Extract counts from task.Result.
		sg.Counts = r.Count(nil)
	}

	if sg.Params.GetCount == 1 {
		x.Trace(ctx, "Zero uids. Only count requested")
		rch <- nil
		return
	}

	sg.destUIDs = algo.MergeLists(sg.Result)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
		rch <- err
		return
	}

	if sg.destUIDs.Size() == 0 {
		// Looks like we're done here.
		x.Trace(ctx, "Zero uids. Num attr children: %v", len(sg.Children))
		rch <- nil
		return
	}

	// Apply filters if any.
	if err = sg.applyFilter(ctx); err != nil {
		rch <- err
		return
	}

	// Apply offset and count (for pagination).
	if err = sg.applyPagination(ctx); err != nil {
		rch <- err
		return
	}

	var processed, leftToProcess []*SubGraph
	// First iterate over the value nodes and check for validity.
	childChan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.srcUIDs = sg.destUIDs // Make the connection.
		if child.Params.AttrType == nil || child.Params.AttrType.IsScalar() {
			processed = append(processed, child)
			taskQuery := createTaskQuery(child, sg.destUIDs, nil, nil)
			go ProcessGraph(ctx, child, taskQuery, childChan)
		} else {
			leftToProcess = append(leftToProcess, child)
			childChan <- nil
		}
	}

	// Now get all the results back.
	for i := 0; i < len(sg.Children); i++ {
		select {
		case err = <-childChan:
			x.Trace(ctx, "Reply from child. Index: %v Attr: %v", i, sg.Children[i].Attr)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error while processing child task"))
				rch <- err
				return
			}
		case <-ctx.Done():
			x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
			rch <- ctx.Err()
			return
		}
	}

	// If all the child nodes are processed, return.
	if len(leftToProcess) == 0 {
		rch <- nil
		return
	}

	sgObj, ok := schema.TypeOf(sg.Attr).(types.Object)
	invalidUids := make(map[uint64]bool)
	// Check the results of the child and eliminate uids without all results.
	if ok {
		for _, node := range processed {
			if _, present := sgObj.Fields[node.Attr]; !present {
				continue
			}
			var tv task.Value
			for i := 0; i < node.Values.ValuesLength(); i++ {
				uid := sg.destUIDs.Get(i)
				if ok := node.Values.Values(&tv, i); !ok {
					invalidUids[uid] = true
				}

				valBytes := tv.ValBytes()
				v, storageType, err := getValue(tv)
				if err != nil || bytes.Equal(valBytes, nil) {
					// The value is not as requested in schema.
					invalidUids[uid] = true
					continue
				}

				// type assertion for scalar type values
				if !node.Params.AttrType.IsScalar() {
					rch <- x.Errorf("Fatal mistakes in type.")
				}

				schemaType := node.Params.AttrType.(types.Scalar)
				if schemaType != storageType {
					if _, err = schemaType.Convert(v); err != nil {
						invalidUids[uid] = true
					}
				}
			}
		}
	}

	// Filter out the invalid UIDs.
	sg.destUIDs.ApplyFilterFn(func(uid uint64) bool {
		return !invalidUids[uid]
	})

	// Now process next level with valid UIDs.
	childChan = make(chan error, len(leftToProcess))
	for _, child := range leftToProcess {
		taskQuery := createTaskQuery(child, sg.destUIDs, nil, nil)
		go ProcessGraph(ctx, child, taskQuery, childChan)
	}

	// Now get the results back.
	for i := 0; i < len(leftToProcess); i++ {
		select {
		case err = <-childChan:
			x.Trace(ctx, "Reply from child. Index: %v Attr: %v", i, sg.Children[i].Attr)
			if err != nil {
				x.Trace(ctx, "Error while processing child task: %v", err)
				rch <- err
				return
			}
		case <-ctx.Done():
			x.Trace(ctx, "Context done before full execution: %v", ctx.Err())
			rch <- ctx.Err()
			return
		}
	}

	rch <- nil
}

// applyFilter applies filters to sg.sorted.
func (sg *SubGraph) applyFilter(ctx context.Context) error {
	if sg.Filter == nil { // No filter.
		return nil
	}
	newSorted, err := runFilter(ctx, sg.destUIDs, sg.Filter)
	if err != nil {
		return err
	}
	sg.destUIDs = newSorted
	return nil
}

// runFilter traverses filter tree and produce a filtered list of UIDs.
// Input "destUIDs" is the very original list of destination UIDs of a SubGraph.
func runFilter(ctx context.Context, destUIDs *algo.UIDList, filter *gql.FilterTree) (*algo.UIDList, error) {
	if len(filter.FuncName) > 0 { // Leaf node.
		x.Assertf(filter.FuncName == "eq", "Only exact match is supported now")
		x.Assertf(len(filter.FuncArgs) == 2,
			"Expect exactly two arguments: pred and predValue")

		attr := filter.FuncArgs[0]
		sg := &SubGraph{Attr: attr}
		sgChan := make(chan error, 1)
		taskQuery := createTaskQuery(sg, nil, []string{filter.FuncArgs[1]}, destUIDs)
		go ProcessGraph(ctx, sg, taskQuery, sgChan)
		err := <-sgChan
		if err != nil {
			return nil, err
		}

		x.Assert(len(sg.Result) == 1)
		return sg.Result[0], nil
	}

	// For now, we only handle AND and OR.
	if filter.Op != gql.FilterOpAnd && filter.Op != gql.FilterOpOr {
		return destUIDs, x.Errorf("Unknown operator %v", filter.Op)
	}

	// Get UIDs for child filters.
	type resultPair struct {
		uid *algo.UIDList
		err error
	}
	resultChan := make(chan resultPair)
	for _, c := range filter.Child {
		go func(c *gql.FilterTree) {
			r, err := runFilter(ctx, destUIDs, c)
			resultChan <- resultPair{r, err}
		}(c)
	}
	lists := make([]*algo.UIDList, 0, len(filter.Child))
	// Collect the results from above goroutines.
	for i := 0; i < len(filter.Child); i++ {
		r := <-resultChan
		if r.err != nil {
			return destUIDs, r.err
		}
		lists = append(lists, r.uid)
	}

	// Either merge or intersect the UID lists.
	if filter.Op == gql.FilterOpOr {
		return algo.MergeLists(lists), nil
	}
	x.Assert(filter.Op == gql.FilterOpAnd)
	return algo.IntersectLists(lists), nil
}

// pageRange returns start and end indices given pagination params. Note that n
// is the size of the input list.
func pageRange(p *params, n int) (int, int) {
	if p.Count == 0 && p.Offset == 0 {
		return 0, n
	}
	if p.Count < 0 {
		// Items from the back of the array, like Python arrays.
		return n + p.Count, n
	}
	start := p.Offset
	if start < 0 {
		start = 0
	}
	if p.Count == 0 { // No count specified. Just take the offset parameter.
		return start, n
	}
	end := start + p.Count
	if end > n {
		end = n
	}
	return start, end
}

// applyWindow applies windowing to sg.sorted.
func (sg *SubGraph) applyPagination(ctx context.Context) error {
	params := sg.Params
	if params.Count == 0 && params.Offset == 0 { // No pagination.
		return nil
	}
	x.Assert(sg.srcUIDs.Size() == len(sg.Result))
	for _, l := range sg.Result {
		l.Intersect(sg.destUIDs)
		start, end := pageRange(&sg.Params, l.Size())
		l.Slice(start, end)
	}
	// Re-merge the UID matrix.
	sg.destUIDs = algo.MergeLists(sg.Result)
	return nil
}
