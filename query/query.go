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
	Alias     string
	Count     int
	Offset    int
	AfterUID  uint64
	DoCount   bool
	GetUID    bool
	Order     string
	OrderDesc bool
	isDebug   bool
}

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr      string
	Params    params
	counts    []uint32
	values    []*task.Value
	uidMatrix []*task.List

	// SrcUIDs is a list of unique source UIDs. They are always copies of destUIDs
	// of parent nodes in GraphQL structure.
	SrcUIDs *task.List
	SrcFunc []string

	FilterOp string
	Filters  []*SubGraph
	Children []*SubGraph

	// destUIDs is a list of destination UIDs, after applying filters, pagination.
	DestUIDs *task.List
}

// DebugPrint prints out the SubGraph tree in a nice format for debugging purposes.
func (sg *SubGraph) DebugPrint(prefix string) {
	var src, dst int
	if sg.SrcUIDs != nil {
		src = len(sg.SrcUIDs.Uids)
	}
	if sg.DestUIDs != nil {
		dst = len(sg.DestUIDs.Uids)
	}
	x.Printf("%s[%q Alias:%q Func:%v SrcSz:%v Op:%q DestSz:%v Dest: %p ValueSz:%v]\n",
		prefix, sg.Attr, sg.Params.Alias, sg.SrcFunc, src, sg.FilterOp,
		dst, sg.DestUIDs, len(sg.values))
	for _, f := range sg.Filters {
		f.DebugPrint(prefix + "|-f->")
	}
	for _, c := range sg.Children {
		c.DebugPrint(prefix + "|->")
	}
}

// getValue gets the value from the task.
func getValue(tv *task.Value) (types.Val, error) {
	vID := types.TypeID(tv.ValType)
	val := types.ValueForType(vID)
	val.Value = tv.Val
	return val, nil
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
func (sg *SubGraph) preTraverse(uid uint64, dst outputNode) error {
	invalidUids := make(map[uint64]bool)
	// We go through all predicate children of the subgraph.
	for _, pc := range sg.Children {
		idx := algo.IndexOf(pc.SrcUIDs, uid)
		if idx < 0 {
			continue
		}
		ul := pc.uidMatrix[idx]

		fieldName := pc.Attr
		if pc.Params.Alias != "" {
			fieldName = pc.Params.Alias
		}
		if sg.Params.GetUID || sg.Params.isDebug {
			dst.SetUID(uid)
		}
		if len(pc.counts) > 0 {
			c := types.ValueForType(types.Int32ID)
			c.Value = int32(pc.counts[idx])
			uc := dst.New(pc.Attr)
			uc.AddValue("_count_", c)
			dst.AddChild(pc.Attr, uc)

		} else if len(ul.Uids) > 0 || len(pc.Children) > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			for _, childUID := range ul.Uids {
				if invalidUids[childUID] {
					continue
				}
				uc := dst.New(fieldName)

				// Doing check for UID here is no good because some of these might be
				// invalid nodes.
				// if pc.Params.GetUID || pc.Params.isDebug {
				//	dst.SetUID(uid)
				// }
				if rerr := pc.preTraverse(childUID, uc); rerr != nil {
					if rerr.Error() == "_INV_" {
						invalidUids[childUID] = true
						continue // next UID.
					}
					// Some other error.
					log.Printf("Error while traversal: %v", rerr)
					return rerr
				}
				if !uc.IsEmpty() {
					dst.AddChild(fieldName, uc)
				}
			}
		} else {
			tv := pc.values[idx]
			v, err := getValue(tv)
			if err != nil {
				return err
			}

			if pc.Attr == "_xid_" {
				txt := types.ValueForType(types.StringID)
				err := types.Convert(v, &txt)
				if err != nil {
					return err
				}
				dst.SetXID(txt.Value.(string))
			} else if pc.Attr == "_uid_" {
				dst.SetUID(uid)
			} else {
				// globalType is the best effort type to which we try converting
				// and if not possible, we ignore it in the result.
				globalType, hasType := schema.TypeOf(pc.Attr)
				sv := types.ValueForType(types.StringID)
				if hasType == nil {
					// Try to coerce types if this is an optional scalar outside an
					// object definition.
					if !globalType.IsScalar() {
						return x.Errorf("Leaf predicate:'%v' must be a scalar.", pc.Attr)
					}
					gtID := globalType
					sv = types.ValueForType(gtID)
					// Convert to schema type.
					err = types.Convert(v, &sv)
					if bytes.Equal(tv.Val, nil) || err != nil {
						continue
					}
				} else {
					x.Check(types.Convert(v, &sv))
				}
				if bytes.Equal(tv.Val, nil) {
					continue
				}
				// Only strings can have empty values.
				if sv.Tid == types.StringID && sv.Value.(string) == "_nil_" {
					sv.Value = ""
				}
				dst.AddValue(fieldName, sv)
			}
		}
	}
	return nil
}

func createProperty(prop string, v types.Val) *graph.Property {
	pval := toProtoValue(v)
	return &graph.Property{Prop: prop, Value: pval}
}

func isPresent(list []string, str string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func filterCopy(sg *SubGraph, ft *gql.FilterTree) {
	// Either we'll have an operation specified, or the function specified.
	if len(ft.Op) > 0 {
		sg.FilterOp = ft.Op
	} else {
		sg.Attr = ft.Func.Attr
		sg.SrcFunc = append(sg.SrcFunc, ft.Func.Name)
		sg.SrcFunc = append(sg.SrcFunc, ft.Func.Args...)
	}
	for _, ftc := range ft.Child {
		child := &SubGraph{}
		filterCopy(child, ftc)
		sg.Filters = append(sg.Filters, child)
	}
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
			sg.Params.DoCount = true
			break
		}
		if gchild.Attr == "_uid_" {
			sg.Params.GetUID = true
		}

		args := params{
			Alias:   gchild.Alias,
			isDebug: sg.Params.isDebug,
		}
		dst := &SubGraph{
			Attr:   gchild.Attr,
			Params: args,
		}
		if gchild.Filter != nil {
			dstf := &SubGraph{}
			filterCopy(dstf, gchild.Filter)
			dst.Filters = append(dst.Filters, dstf)
		}

		if v, ok := gchild.Args["offset"]; ok {
			offset, err := strconv.ParseInt(v, 0, 32)
			if err != nil {
				return err
			}
			dst.Params.Offset = int(offset)
		}
		if v, ok := gchild.Args["after"]; ok {
			after, err := strconv.ParseUint(v, 0, 64)
			if err != nil {
				return err
			}
			dst.Params.AfterUID = uint64(after)
		}
		if v, ok := gchild.Args["first"]; ok {
			first, err := strconv.ParseInt(v, 0, 32)
			if err != nil {
				return err
			}
			dst.Params.Count = int(first)
		}
		if v, ok := gchild.Args["order"]; ok {
			dst.Params.Order = v
		} else if v, ok := gchild.Args["orderdesc"]; ok {
			dst.Params.Order = v
			dst.Params.OrderDesc = true
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
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.
	if len(gq.UID) == 0 && gq.Func == nil {
		err := x.Errorf("Invalid query, query internal id is zero and generator is nil")
		x.TraceError(ctx, err)
		return nil, err
	}

	// For the root, the name to be used in result is stored in Alias, not Attr.
	// The attr at root (if present) would stand for the source functions attr.
	args := params{
		isDebug: gq.Alias == "debug",
		Alias:   gq.Alias,
	}

	sg := &SubGraph{
		Params: args,
	}
	if gq.Func != nil {
		sg.Attr = gq.Func.Attr
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Name)
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Args...)
	}
	if len(gq.UID) > 0 {
		sg.SrcUIDs = &task.List{Uids: gq.UID}
		sg.uidMatrix = []*task.List{&task.List{Uids: gq.UID}}
	}
	sg.values = createNilValuesList(1)
	// Copy roots filter.
	if gq.Filter != nil {
		sgf := &SubGraph{}
		filterCopy(sgf, gq.Filter)
		sg.Filters = append(sg.Filters, sgf)
	}
	return sg, nil
}

func createNilValuesList(count int) []*task.Value {
	out := make([]*task.Value, count)
	for i := 0; i < count; i++ {
		out[i] = &task.Value{
			Val: x.Nilbyte,
		}
	}
	return out
}

// createTaskQuery generates the query buffer.
func createTaskQuery(sg *SubGraph) *task.Query {
	attr := sg.Attr
	// Might be safer than just checking first byte due to i18n
	reverse := strings.HasPrefix(attr, "~")
	if reverse {
		attr = strings.TrimPrefix(attr, "~")
	}
	out := &task.Query{
		Attr:     attr,
		Reverse:  reverse,
		SrcFunc:  sg.SrcFunc,
		Count:    int32(sg.Params.Count),
		Offset:   int32(sg.Params.Offset),
		AfterUid: sg.Params.AfterUID,
		DoCount:  len(sg.Filters) == 0 && sg.Params.DoCount,
	}
	if sg.SrcUIDs != nil {
		out.Uids = sg.SrcUIDs.Uids
	}
	return out
}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances. Note: taskQuery is nil for root node.
func ProcessGraph(ctx context.Context, sg, parent *SubGraph, rch chan error) {
	var err error
	if len(sg.Attr) == 0 {
		// If we have a filter SubGraph which only contains an operator,
		// it won't have any attribute to work on.
		// This is to allow providing SrcUIDs to the filter children.
		sg.DestUIDs = sg.SrcUIDs

	} else if parent == nil && len(sg.SrcFunc) == 0 {
		// I am root. I don't have any function to execute, and my
		// result has been prepared for me already.
		sg.DestUIDs = algo.MergeSorted(sg.uidMatrix) // Could also be = sg.SrcUIDs

	} else {
		taskQuery := createTaskQuery(sg)
		result, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
			rch <- err
			return
		}

		sg.uidMatrix = result.UidMatrix
		sg.values = result.Values
		if len(sg.values) > 0 {
			v := sg.values[0]
			x.Trace(ctx, "Sample value for attr: %v Val: %v", sg.Attr, string(v.Val))
		}
		sg.counts = result.Counts

		if sg.Params.DoCount && len(sg.Filters) == 0 {
			// If there is a filter, we need to do more work to get the actual count.
			x.Trace(ctx, "Zero uids. Only count requested")
			rch <- nil
			return
		}

		if result.IntersectDest {
			sg.DestUIDs = algo.IntersectSorted(result.UidMatrix)
		} else {
			sg.DestUIDs = algo.MergeSorted(result.UidMatrix)
		}
	}

	if len(sg.DestUIDs.Uids) == 0 {
		// Looks like we're done here. Be careful with nil srcUIDs!
		x.Trace(ctx, "Zero uids for %q. Num attr children: %v", sg.Attr, len(sg.Children))
		rch <- nil
		return
	}

	// Apply filters if any.
	if len(sg.Filters) > 0 {
		// Run all filters in parallel.
		filterChan := make(chan error, len(sg.Filters))
		for _, filter := range sg.Filters {
			filter.SrcUIDs = sg.DestUIDs
			go ProcessGraph(ctx, filter, sg, filterChan)
		}

		for _ = range sg.Filters {
			select {
			case err = <-filterChan:
				if err != nil {
					x.TraceError(ctx, x.Wrapf(err, "Error while processing filter task"))
					rch <- err
					return
				}

			case <-ctx.Done():
				x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
				rch <- ctx.Err()
				return
			}
		}

		// Now apply the results from filter.
		var lists []*task.List
		for _, filter := range sg.Filters {
			lists = append(lists, filter.DestUIDs)
		}
		if sg.FilterOp == "|" {
			sg.DestUIDs = algo.MergeSorted(lists)
		} else {
			sg.DestUIDs = algo.IntersectSorted(lists)
		}
	}

	if len(sg.Params.Order) == 0 {
		// There is no ordering. Just apply pagination and return.
		if err = sg.applyPagination(ctx); err != nil {
			rch <- err
			return
		}
	} else {
		// If we are asked for count, we don't need to change the order of results.
		if !sg.Params.DoCount {
			// We need to sort first before pagination.
			if err = sg.applyOrderAndPagination(ctx); err != nil {
				rch <- err
				return
			}
		}
	}

	// Here we consider handling _count_ with filtering. We do this after
	// pagination because otherwise, we need to do the count with pagination
	// taken into account. For example, a PL might have only 50 entries but the
	// user wants to skip 100 entries and return 10 entries. In this case, you
	// should return a count of 0, not 10.
	if sg.Params.DoCount {
		x.AssertTrue(len(sg.Filters) > 0)
		sg.counts = make([]uint32, len(sg.uidMatrix))
		for i, ul := range sg.uidMatrix {
			// A possible optimization is to return the size of the intersection
			// without forming the intersection.

			algo.IntersectWith(ul, sg.DestUIDs)
			sg.counts[i] = uint32(len(ul.Uids))
		}
		rch <- nil
		return
	}

	childChan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.SrcUIDs = sg.DestUIDs // Make the connection.
		go ProcessGraph(ctx, child, sg, childChan)
	}

	// Now get all the results back.
	for _ = range sg.Children {
		select {
		case err = <-childChan:
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

// pageRange returns start and end indices given pagination params. Note that n
// is the size of the input list.
func pageRange(p *params, n int) (int, int) {
	if p.Count == 0 && p.Offset == 0 {
		return 0, n
	}
	if p.Count < 0 {
		// Items from the back of the array, like Python arrays. Do a postive mod n.
		return (((n + p.Count) % n) + n) % n, n
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
	x.AssertTrue(len(sg.SrcUIDs.Uids) == len(sg.uidMatrix))
	for _, l := range sg.uidMatrix {
		algo.IntersectWith(l, sg.DestUIDs)
		start, end := pageRange(&sg.Params, len(l.Uids))
		l.Uids = l.Uids[start:end]
	}
	// Re-merge the UID matrix.
	sg.DestUIDs = algo.MergeSorted(sg.uidMatrix)
	return nil
}

// applyOrderAndPagination orders each posting list by a given attribute
// before applying pagination.
func (sg *SubGraph) applyOrderAndPagination(ctx context.Context) error {
	if len(sg.Params.Order) == 0 {
		return nil
	}
	if sg.Params.Count == 0 {
		// Only retrieve up to 1000 results by default.
		sg.Params.Count = 1000
	}

	sort := &task.Sort{
		Attr:      sg.Params.Order,
		UidMatrix: sg.uidMatrix,
		Offset:    int32(sg.Params.Offset),
		Count:     int32(sg.Params.Count),
		Desc:      sg.Params.OrderDesc,
	}
	result, err := worker.SortOverNetwork(ctx, sort)
	if err != nil {
		return err
	}

	x.AssertTrue(len(result.UidMatrix) == len(sg.uidMatrix))
	sg.uidMatrix = result.GetUidMatrix()

	// Update sg.destUID. Iterate over the UID matrix (which is not sorted by
	// UID). For each element in UID matrix, we do a binary search in the
	// current destUID and mark it. Then we scan over this bool array and
	// rebuild destUIDs.
	included := make([]bool, len(sg.DestUIDs.Uids))
	for _, ul := range sg.uidMatrix {
		for _, uid := range ul.Uids {
			idx := algo.IndexOf(sg.DestUIDs, uid) // Binary search.
			if idx >= 0 {
				included[idx] = true
			}
		}
	}
	algo.ApplyFilter(sg.DestUIDs,
		func(uid uint64, idx int) bool { return included[idx] })
	return nil
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

func (sg *SubGraph) FastToJSON(l *Latency) ([]byte, error) {
	var seedNode *fastJsonNode
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
	return n.(*fastJsonNode).fastJsonNodeToJSON(), nil
}
