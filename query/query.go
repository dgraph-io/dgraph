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
	Filter   *gql.FilterTree

	Query []byte // Contains list of source UIDs.
	//Result []byte // Contains UID matrix or list of values for child attributes.

	Count  *task.CountList
	Values task.ValueList
	Result algo.UIDLists

	// sorted is a list of destination UIDs, after applying filters.
	// One may ask: Doesn't children nodes contain sg.sorted? There is one
	// exception. Imagine a node with no children but we ask for its children's
	// UIDs. The node has a filter. In this case, it has no children to store the
	// filtered UIDs.
	sorted []uint64
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

	r := sg.Result
	x.Assertf(q.UidsLength() == r.Size(),
		"Result uidmatrixlength: %v. Query uidslength: %v", q.UidsLength(), r.Size())
	x.Assertf(q.UidsLength() == sg.Values.ValuesLength(),
		"Result valuelength: %v. Query uidslength: %v", q.UidsLength(), sg.Values.ValuesLength())

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

	if sg.Count != nil && sg.Count.CountLength() > 0 {
		log.Printf("~~~~postTraverse: [%s] len=%d", sg.Attr, sg.Count.CountLength())
		for i := 0; i < sg.Count.CountLength(); i++ {
			co := sg.Count.Count(i)
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
	}

	for i := 0; i < r.Size(); i++ {
		ul := r.Get(i)
		l := make([]interface{}, 0, ul.Size())

		// We want to intersect ul.Uids with this list. Since both are sorted, this
		// intersection is very cheap. We just need to maintain the variable sortedIdx
		// which indexes into sg.sorted (the sorted UID list).
		var sortedIdx int
		for j := 0; j < ul.Size(); j++ {
			uid := ul.Get(j)
			for ; sortedIdx < len(sg.sorted) && sg.sorted[sortedIdx] < uid; sortedIdx++ {
			}
			if sortedIdx >= len(sg.sorted) || sg.sorted[sortedIdx] > uid {
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
		if len(l) == 1 {
			m := make(map[string]interface{})
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = l[0]
			} else {
				m[sg.Attr] = l[0]
			}
			result[q.Uids(i)] = m
		} else if len(l) > 1 {
			m := make(map[string]interface{})
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = l
			} else {
				m[sg.Attr] = l
			}
			result[q.Uids(i)] = m
		}
	}

	values := sg.Values
	var tv task.Value
	for i := 0; i < values.ValuesLength(); i++ {
		if ok := values.Values(&tv, i); !ok {
			return result, fmt.Errorf("While parsing value")
		}
		val := tv.ValBytes()
		if bytes.Equal(val, nil) {
			// We do this, because we typically do set values, even though
			// they might be nil. This is to ensure that the index of the query uids
			// and the index of the results can remain in sync.
			continue
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
			// No type defined for attr in type system/schema, hence return string value
			if sg.Params.Alias != "" {
				m[sg.Params.Alias] = string(val)
			} else {
				m[sg.Attr] = string(val)
			}
		} else {
			// type assertion for scalar type values
			if !sg.Params.AttrType.IsScalar() {
				return result, fmt.Errorf("Unknown Scalar:%v. Leaf predicate:'%v' must be"+
					" one of the scalar types defined in the schema.", sg.Params.AttrType, sg.Attr)
			}
			stype := sg.Params.AttrType.(types.Scalar)
			lval, err := stype.Unmarshaler.FromText(val)
			if err != nil {
				return result, err
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
		r := pc.Result

		q := new(task.Query)
		x.ParseTaskQuery(q, pc.Query)
		idx := indexOf(uid, q)

		if idx == -1 {
			log.Fatal("Attribute with uid not found in child Query uids.")
			return fmt.Errorf("Attribute with uid not found")
		}

		ul := r.Get(idx)
		var tv task.Value

		if sg.Count != nil && sg.Count.CountLength() > 0 {
			count := strconv.Itoa(int(sg.Count.Count(idx)))
			p := &graph.Property{Prop: "_count_", Val: []byte(count)}
			uc := &graph.Node{
				Attribute:  pc.Attr,
				Properties: []*graph.Property{p},
			}
			children = append(children, uc)

		} else if ul.Size() > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			var sortedIdx int // Index into pc.sorted.
			for i := 0; i < ul.Size(); i++ {
				uid := ul.Get(i)
				for ; sortedIdx < len(pc.sorted) && pc.sorted[sortedIdx] < uid; sortedIdx++ {
				}
				if sortedIdx >= len(pc.sorted) || pc.sorted[sortedIdx] > uid {
					continue
				}
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
			if ok := pc.Values.Values(&tv, idx); !ok {
				return x.Errorf("While parsing value")
			}
			v := tv.ValBytes()

			//do type checking on response values
			if pc.Params.AttrType != nil {
				// type assertion for scalar type values
				if !pc.Params.AttrType.IsScalar() {
					return fmt.Errorf("Unknown Scalar:%v. Leaf predicate:'%v' must be"+
						" one of the scalar types defined in the schema.", pc.Params.AttrType, pc.Attr)
				}
				stype := pc.Params.AttrType.(types.Scalar)
				if _, err := stype.Unmarshaler.FromText(v); err != nil {
					return err
				}
			}

			if pc.Attr == "_xid_" {
				dst.Xid = string(v)
				// We don't want to add _uid_ to properties map.
			} else if pc.Attr == "_uid_" {
				continue
			} else {
				p := &graph.Property{Prop: pc.Attr, Val: v}
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

	x.Assert(sg.Result.Size() == 1)
	ul := sg.Result.Get(0)
	if sg.Params.GetUid || sg.Params.isDebug {
		n.Uid = ul.Get(0)
	}

	if rerr := sg.preTraverse(ul.Get(0), n); rerr != nil {
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

	// sg is to be returned.
	args := params{
		AttrType: gql.SchemaType(gq.Attr),
		IsRoot:   true,
		isDebug:  gq.Attr == "debug",
	}
	sg := &SubGraph{
		Attr:   gq.Attr,
		Params: args,
		Filter: gq.Filter,
	}

	log.Printf("~~~~populating [%s]", sg.Attr)

	{
		// Encode uid into result flatbuffer.
		b := flatbuffers.NewBuilder(0)
		b.Finish(x.UidlistOffset(b, []uint64{euid}))
		buf := b.FinishedBytes()
		ul := new(algo.UIDList)
		x.ParseUidList(&ul.UidList, buf)
		sg.Result = algo.UIDLists{ul}
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

		log.Printf("~~~~~~~[%s] %d", sg.Attr, sg.Values.ValuesLength())
	}

	// Also add query for consistency and to allow for ToJSON() later.
	sg.Query = createTaskQuery(sg, []uint64{euid}, nil, nil)
	return sg, nil
}

// createTaskQuery generates the query buffer.
func createTaskQuery(sg *SubGraph, uids []uint64, terms []string, intersect []uint64) []byte {
	x.Assert(uids == nil || terms == nil)

	b := flatbuffers.NewBuilder(0)
	var vend flatbuffers.UOffsetT
	if uids != nil {
		task.QueryStartUidsVector(b, len(uids))
		for i := len(uids) - 1; i >= 0; i-- {
			b.PrependUint64(uids[i])
		}
		vend = b.EndVector(len(uids))
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
		intersectOffset = x.UidlistOffset(b, intersect)
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
// from different instances.
func ProcessGraph(ctx context.Context, sg *SubGraph, rch chan error) {
	var err error
	if len(sg.Query) > 0 && !sg.Params.IsRoot {
		resultBuf, err := worker.ProcessTaskOverNetwork(ctx, sg.Query)
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
			ul := new(algo.UIDList)
			x.Assert(r.Uidmatrix(&ul.UidList, i))
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
		sg.Count = r.Count(nil)
	}

	if sg.Params.GetCount == 1 {
		x.Trace(ctx, "Zero uids. Only count requested")
		rch <- nil
		return
	}

	sg.sorted = algo.MergeSorted(sg.Result)
	log.Printf("~~~[%s] %v", sg.Attr, sg.sorted)
	if sg.Values.ValuesLength() > 0 {
		log.Printf("~~~values [%s] len=%d", sg.Attr, sg.Values.ValuesLength())
		for i := 0; i < sg.Values.ValuesLength(); i++ {
			var tv task.Value
			if sg.Values.Values(&tv, i) {
				log.Printf("~~~value [%s][%d] = %s", sg.Attr, i, string(tv.ValBytes()))
			}
		}
	}
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
		rch <- err
		return
	}

	if len(sg.sorted) == 0 {
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

	// Let's execute it in a tree fashion. Each SubGraph would break off
	// as many goroutines as it's children; which would then recursively
	// do the same thing.
	// Buffered channel to ensure no-blockage.
	childchan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.Query = createTaskQuery(child, sg.sorted, nil, nil)
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

// applyFilter applies filters to sg.sorted.
func (sg *SubGraph) applyFilter(ctx context.Context) error {
	if sg.Filter == nil { // No filter.
		return nil
	}
	newSorted, err := runFilter(ctx, sg.sorted, sg.Filter)
	if err != nil {
		return err
	}
	sg.sorted = newSorted
	return nil
}

// runFilter traverses filter tree and produce a filtered list of UIDs.
// Input "sorted" is the very original list of destination UIDs of a SubGraph.
func runFilter(ctx context.Context, sorted []uint64, filter *gql.FilterTree) ([]uint64, error) {
	if len(filter.FuncName) > 0 { // Leaf node.
		x.Assertf(filter.FuncName == "eq", "Only exact match is supported now")
		x.Assertf(len(filter.FuncArgs) == 2,
			"Expect exactly two arguments: pred and predValue")

		attr := filter.FuncArgs[0]
		sg := &SubGraph{Attr: attr}
		sg.Query = createTaskQuery(sg, nil, []string{filter.FuncArgs[1]}, sorted)
		sgChan := make(chan error, 1)
		go ProcessGraph(ctx, sg, sgChan)
		err := <-sgChan
		if err != nil {
			return nil, err
		}

		x.Assert(sg.Result.Size() == 1)
		ul := sg.Result.Get(0)
		result := make([]uint64, ul.Size())
		for i := 0; i < ul.Size(); i++ {
			result[i] = ul.Get(i)
		}
		return result, nil
	}

	// For now, we only handle AND and OR.
	if filter.Op != gql.FilterOpAnd && filter.Op != gql.FilterOpOr {
		return sorted, x.Errorf("Unknown operator %v", filter.Op)
	}

	// Get UIDs for child filters.
	type resultPair struct {
		uid []uint64
		err error
	}
	resultChan := make(chan resultPair)
	for _, c := range filter.Child {
		go func(c *gql.FilterTree) {
			r, err := runFilter(ctx, sorted, c)
			resultChan <- resultPair{r, err}
		}(c)
	}
	uidList := make(algo.PlainUintLists, 0, len(filter.Child))
	// Collect the results from above goroutines.
	for i := 0; i < len(filter.Child); i++ {
		r := <-resultChan
		if r.err != nil {
			return sorted, r.err
		}
		uidList = append(uidList, r.uid)
	}

	// Either merge or intersect the UID lists.
	if filter.Op == gql.FilterOpOr {
		return algo.MergeSorted(uidList), nil
	}
	x.Assert(filter.Op == gql.FilterOpAnd)
	return algo.IntersectSorted(uidList), nil
}

// window returns start and end indices given windowing params.
func window(p *params, l *task.UidList) (int, int) {
	if p.Count == 0 && p.Offset == 0 { // No windowing.
		return 0, l.UidsLength()
	}
	if p.Count < 0 {
		// Items from the back of the array, like Python arrays.
		return l.UidsLength() - p.Count, l.UidsLength()
	}
	start := p.Offset
	if start < 0 {
		start = 0
	}
	if p.Count == 0 {
		return start, l.UidsLength()
	}
	end := start + p.Count
	if end > l.UidsLength() {
		end = l.UidsLength()
	}
	return start, end
}

// applyWindow applies windowing to sg.sorted.
func (sg *SubGraph) applyPagination(ctx context.Context) error {
	return nil
	//	params := sg.Params
	//	if params.Count == 0 && params.Offset == 0 { // No windowing.
	//		return nil
	//	}
	//	// For each row in UID matrix, we want to apply windowing. After that, we need
	//	// to rebuild sg.sorted.
	//	var result task.Result
	//	x.ParseTaskResult(&result, sg.Result)

	//	// We do not modify sg.Result. In postTraverse and preTraverse, we will take
	//	// into count windowing params.
	//	n := result.UidmatrixLength()
	//	var results algo.GenericLists
	//	results.Data = make([]algo.Uint64List, n)
	//	for i := 0; i < n; i++ {
	//		l := new(algo.UIDList)
	//		x.Assert(result.Uidmatrix(&l.UidList, i))
	//		start, end := window(&sg.Params, &l.UidList)
	//		results.Data[i] = algo.NewUint64ListSlice(l, start, end)
	//	}
	//	sg.sorted = algo.MergeSorted(results)
	//	return nil
}
