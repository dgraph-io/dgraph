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
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
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
	IsRoot   bool
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

	Query  []byte // Contains list of source UIDs.
	Result []byte // Contains UID matrix or list of values for child attributes.
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
	if len(sg.Query) == 0 {
		return nil, nil
	}
	result := make(map[uint64]interface{})
	// Get results from all children first.
	cResult := make(map[uint64]interface{})

	for _, child := range sg.Children {
		m, err := postTraverse(child)
		if err != nil {
			return result, err
		}
		// Merge results from all children, one by one.
		for k, v := range m {
			if val, present := cResult[k]; !present {
				cResult[k] = v
			} else {
				cResult[k] = mergeInterfaces(val, v)
			}
		}
	}

	// Now read the query and results at current node.
	q := new(task.Query)
	x.ParseTaskQuery(q, sg.Query)

	r := new(task.Result)
	x.ParseTaskResult(r, sg.Result)

	if q.UidsLength() != r.UidmatrixLength() {
		log.Fatalf("Result uidmatrixlength: %v. Query uidslength: %v",
			r.UidmatrixLength(), q.UidsLength())
	}
	if q.UidsLength() != r.ValuesLength() {
		log.Fatalf("Result valuelength: %v. Query uidslength: %v",
			r.ValuesLength(), q.UidsLength())
	}

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

	for i := 0; i < r.CountLength(); i++ {
		co := r.Count(i)
		m := make(map[string]interface{})
		m["_count_"] = co
		mp := make(map[string]interface{})
		if sg.Params.Alias != "" {
			mp[sg.Params.Alias] = m
		} else {
			mp[sg.Attr] = m
		}
		result[q.Uids(i)] = mp
	}

	var ul task.UidList
	for i := 0; i < r.UidmatrixLength(); i++ {
		if ok := r.Uidmatrix(&ul, i); !ok {
			return result, fmt.Errorf("While parsing UidList")
		}
		l := make([]interface{}, ul.UidsLength())
		for j := 0; j < ul.UidsLength(); j++ {
			uid := ul.Uids(j)
			m := make(map[string]interface{})
			if sg.Params.GetUid || sg.Params.isDebug {
				m["_uid_"] = fmt.Sprintf("%#x", uid)
			}
			if ival, present := cResult[uid]; !present {
				l[j] = m
			} else {
				l[j] = mergeInterfaces(m, ival)
			}
		}
		if len(l) > 0 {
			m := make(map[string]interface{})
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = l
			} else {
				m[sg.Attr] = l
			}
			result[q.Uids(i)] = m
		}
	}
	var tv task.Value
	for i := 0; i < r.ValuesLength(); i++ {
		if ok := r.Values(&tv, i); !ok {
			return result, fmt.Errorf("While parsing value")
		}
		valBytes := tv.ValBytes()
		if bytes.Equal(valBytes, nil) {
			// We do this, because we typically do set values, even though
			// they might be nil. This is to ensure that the index of the query uids
			// and the index of the results can remain in sync.
			continue
		}
		val, storageType, err := getValue(tv)
		if err != nil {
			return result, err
		}

		if pval, present := result[q.Uids(i)]; present {
			log.Fatalf("prev: %v _uid_: %v new: %v"+
				" Previous value detected. A uid -> list of uids / value. Not both",
				pval, q.Uids(i), val)
		}
		m := make(map[string]interface{})
		if sg.Params.GetUid || sg.Params.isDebug {
			m["_uid_"] = fmt.Sprintf("%#x", q.Uids(i))
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
				return result, fmt.Errorf("Unknown Scalar:%v. Leaf predicate:'%v' must be"+
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
					continue
				}
			}
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = lval
			} else {
				m[sg.Attr] = lval
			}
		}
		result[q.Uids(i)] = m
	}
	return result, nil
}

// gets the value from the task.
func getValue(tv task.Value) (types.TypeValue, types.Type, error) {
	vType := tv.ValType()
	valBytes := tv.ValBytes()
	stype := types.TypeForID(types.TypeID(vType))
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
	return nil, fmt.Errorf("Runtime should never reach here.")
}

// This function performs a binary search on the uids slice and returns the
// index at which it finds the uid, else returns -1
func indexOf(uid uint64, q *task.Query) int {
	low, mid, high := 0, 0, q.UidsLength()-1
	for low <= high {
		mid = (low + high) / 2
		if q.Uids(mid) == uid {
			return mid
		} else if q.Uids(mid) > uid {
			high = mid - 1
		} else {
			low = mid + 1
		}
	}
	return -1
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

	// We go through all predicate children of the subgraph.
	for _, pc := range sg.Children {
		r := new(task.Result)
		x.ParseTaskResult(r, pc.Result)

		q := new(task.Query)
		x.ParseTaskQuery(q, pc.Query)
		idx := indexOf(uid, q)

		if idx == -1 {
			log.Fatal("Attribute with uid not found in child Query uids.")
			return fmt.Errorf("Attribute with uid not found")
		}

		var ul task.UidList
		var tv task.Value
		if ok := r.Uidmatrix(&ul, idx); !ok {
			return fmt.Errorf("While parsing UidList")
		}

		if r.CountLength() > 0 {
			count := strconv.Itoa(int(r.Count(idx)))
			p := &graph.Property{Prop: "_count_", Val: []byte(count)}
			uc := &graph.Node{
				Attribute:  pc.Attr,
				Properties: []*graph.Property{p},
			}
			children = append(children, uc)

		} else if ul.UidsLength() > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			for i := 0; i < ul.UidsLength(); i++ {
				uid := ul.Uids(i)
				uc := nodePool.Get().(*graph.Node)
				uc.Attribute = pc.Attr
				if sg.Params.GetUid || sg.Params.isDebug {
					uc.Uid = uid
				}
				if rerr := pc.preTraverse(uid, uc); rerr != nil {
					log.Printf("Error while traversal: %v", rerr)
					return rerr
				}
				children = append(children, uc)
			}
		} else {
			if ok := r.Values(&tv, idx); !ok {
				return fmt.Errorf("While parsing value")
			}

			v, storageType, err := getValue(tv)
			if err != nil {
				return err
			}

			//do type checking on response values
			if pc.Params.AttrType != nil {
				// type assertion for scalar type values
				if !pc.Params.AttrType.IsScalar() {
					return fmt.Errorf("Unknown Scalar:%v. Leaf predicate:'%v' must be"+
						" one of the scalar types defined in the schema.", pc.Params.AttrType, pc.Attr)
				}
				schemaType := pc.Params.AttrType.(types.Scalar)
				if schemaType != storageType {
					if _, err := schemaType.Convert(v); err != nil {
						// skip values that don't convert.
						continue
					}
				}
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
			} else {
				val, err := v.MarshalBinary()
				if err != nil {
					return err
				}
				// TODO: Figure out how the client parses this value
				p := &graph.Property{Prop: pc.Attr, Val: val}
				properties = append(properties, p)
			}
		}
	}

	dst.Properties, dst.Children = properties, children
	return nil
}

// ToProtocolBuffer method transforms the predicate based subgraph to an
// predicate-entity based protocol buffer subgraph.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*graph.Node, error) {
	n := &graph.Node{
		Attribute: sg.Attr,
	}
	if len(sg.Query) == 0 {
		return n, nil
	}

	r := new(task.Result)
	x.ParseTaskResult(r, sg.Result)

	var ul task.UidList
	r.Uidmatrix(&ul, 0)
	if sg.Params.GetUid || sg.Params.isDebug {
		n.Uid = ul.Uids(0)
	}

	if rerr := sg.preTraverse(ul.Uids(0), n); rerr != nil {
		return n, rerr
	}

	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n, nil
}

func treeCopy(ctx context.Context, gq *gql.GraphQuery, sg *SubGraph) error {
	// Typically you act on the current node, and leave recursion to deal with
	// children. But, in this case, we don't want to muck with the current
	// node, because of the way we're dealing with the root node.
	// So, we work on the children, and then recurse for grand children.
	for _, gchild := range gq.Children {
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

		args := params{
			AttrType: gql.SchemaType(gchild.Attr),
			Alias:    gchild.Alias,
			isDebug:  sg.Params.isDebug,
		}
		dst := &SubGraph{
			Attr:   gchild.Attr,
			Params: args,
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

func newGraph(ctx context.Context, gq *gql.GraphQuery) (*SubGraph, error) {
	euid, exid := gq.UID, gq.XID
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.
	if len(exid) > 0 {
		xidToUid := make(map[string]uint64)
		xidToUid[exid] = 0
		if err := worker.GetOrAssignUidsOverNetwork(ctx, xidToUid); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while getting uids over network"))
			return nil, err
		}

		euid = xidToUid[exid]
		x.Trace(ctx, "Xid: %v Uid: %v", exid, euid)
	}

	if euid == 0 {
		err := x.Errorf("Invalid query, query internal id is zero")
		x.TraceError(ctx, err)
		return nil, err
	}

	// Encode uid into result flatbuffer.
	b := flatbuffers.NewBuilder(0)
	omatrix := x.UidlistOffset(b, []uint64{euid})

	// Also need to add nil value to keep this consistent.
	var voffset flatbuffers.UOffsetT
	{
		bvo := b.CreateByteVector(x.Nilbyte)
		task.ValueStart(b)
		task.ValueAddVal(b, bvo)
		voffset = task.ValueEnd(b)
	}

	task.ResultStartUidmatrixVector(b, 1)
	b.PrependUOffsetT(omatrix)
	mend := b.EndVector(1)

	task.ResultStartValuesVector(b, 1)
	b.PrependUOffsetT(voffset)
	vend := b.EndVector(1)

	task.ResultStart(b)
	task.ResultAddUidmatrix(b, mend)
	task.ResultAddValues(b, vend)
	rend := task.ResultEnd(b)
	b.Finish(rend)

	args := params{
		AttrType: gql.SchemaType(gq.Attr),
		IsRoot:   true,
		isDebug:  gq.Attr == "debug",
	}
	sg := &SubGraph{
		Attr:   gq.Attr,
		Result: b.Bytes[b.Head():],
		Params: args,
	}
	// Also add query for consistency and to allow for ToJSON() later.
	sg.Query = createTaskQuery(sg, []uint64{euid})
	return sg, nil
}

// createTaskQuery generates the query buffer.
func createTaskQuery(sg *SubGraph, sorted []uint64) []byte {
	b := flatbuffers.NewBuilder(0)
	ao := b.CreateString(sg.Attr)

	task.QueryStartUidsVector(b, len(sorted))
	for i := len(sorted) - 1; i >= 0; i-- {
		b.PrependUint64(sorted[i])
	}
	vend := b.EndVector(len(sorted))

	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	task.QueryAddUids(b, vend)
	task.QueryAddCount(b, int32(sg.Params.Count))
	task.QueryAddOffset(b, int32(sg.Params.Offset))
	task.QueryAddAfterUid(b, sg.Params.AfterUid)
	task.QueryAddGetCount(b, sg.Params.GetCount)

	qend := task.QueryEnd(b)
	b.Finish(qend)
	return b.Bytes[b.Head():]
}

func sortedUniqueUids(r *task.Result) ([]uint64, error) {
	uidLists := make(algo.UIDLists, r.UidmatrixLength())
	for i := 0; i < r.UidmatrixLength(); i++ {
		ul := new(algo.UIDList)
		if ok := r.Uidmatrix(&ul.UidList, i); !ok {
			return nil, x.Errorf("While parsing UID matrix")
		}
		uidLists[i] = ul
	}
	return algo.MergeSorted(uidLists), nil
}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances.
func ProcessGraph(ctx context.Context, sg *SubGraph, rch chan error) {
	var err error
	if len(sg.Query) > 0 && !sg.Params.IsRoot {
		sg.Result, err = worker.ProcessTaskOverNetwork(ctx, sg.Query)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
			rch <- err
			return
		}
	}

	r := new(task.Result)
	x.ParseTaskResult(r, sg.Result)
	if r.ValuesLength() > 0 {
		var v task.Value
		if r.Values(&v, 0) {
			x.Trace(ctx, "Sample value for attr: %v Val: %v", sg.Attr, string(v.ValBytes()))
		}
	}

	if sg.Params.GetCount == 1 {
		x.Trace(ctx, "Zero uids. Only count requested")
		rch <- nil
		return
	}

	sorted, err := sortedUniqueUids(r)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
		rch <- err
		return
	}

	if len(sorted) == 0 {
		// Looks like we're done here.
		x.Trace(ctx, "Zero uids. Num attr children: %v", len(sg.Children))
		rch <- nil
		return
	}

	// Let's execute it in a tree fashion. Each SubGraph would break off
	// as many goroutines as it's children; which would then recursively
	// do the same thing.
	// Buffered channel to ensure no-blockage.
	childchan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.Query = createTaskQuery(child, sorted)
		go ProcessGraph(ctx, child, childchan)
	}

	// Now get all the results back.
	for i := 0; i < len(sg.Children); i++ {
		select {
		case err = <-childchan:
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
	rch <- nil
}
