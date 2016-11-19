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

	farm "github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/geo"
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
	AttrType types.Type
	Alias    string
	Count    int
	Offset   int
	AfterUID uint64
	DoCount  bool
	GetUID   bool
	Order    string
	isDebug  bool
}

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr      string
	Children  []*SubGraph
	Params    params
	Filter    *gql.FilterTree
	counts    []uint32
	values    []*task.Value
	uidMatrix []*task.List // TODO: This will be replaced with task.UIDList.

	// SrcUIDs is a list of unique source UIDs. They are always copies of destUIDs
	// of parent nodes in GraphQL structure.
	SrcUIDs *task.List

	// destUIDs is a list of destination UIDs, after applying filters, pagination.
	DestUIDs *task.List
}

// getValue gets the value from the task.
func getValue(tv *task.Value) (types.Value, error) {
	vType := tv.ValType
	valBytes := tv.Val
	val := types.ValueForType(types.TypeID(vType))
	if val == nil {
		return nil, x.Errorf("Invalid type: %v", vType)
	}
	if err := val.UnmarshalBinary(valBytes); err != nil {
		return nil, err
	}
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
		x.AssertTruef(idx >= 0, "Attribute with uid not found in child Query uids")
		ul := pc.uidMatrix[idx]

		if len(pc.counts) > 0 {
			c := types.Int32(pc.counts[idx])
			uc := dst.New(pc.Attr)
			uc.AddValue("_count_", &c)
			dst.AddChild(pc.Attr, uc)
		} else if len(ul.Uids) > 0 || len(pc.Children) > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			for _, uid := range ul.Uids {
				if invalidUids[uid] {
					continue
				}
				uc := dst.New(pc.Attr)
				if sg.Params.GetUID || sg.Params.isDebug {
					uc.SetUID(uid)
				}
				if rerr := pc.preTraverse(uid, uc); rerr != nil {
					if rerr.Error() == "_INV_" {
						invalidUids[uid] = true
						continue // next UID.
					}
					// Some other error.
					log.Printf("Error while traversal: %v", rerr)
					return rerr
				}
				dst.AddChild(pc.Attr, uc)
			}
		} else {
			tv := pc.values[idx]
			v, err := getValue(tv)
			if err != nil {
				return err
			}

			if pc.Attr == "_xid_" {
				txt, err := v.MarshalText()
				if err != nil {
					return err
				}
				dst.SetXID(string(txt))
			} else if pc.Attr == "_uid_" {
				dst.SetUID(uid)
			} else {
				globalType := schema.TypeOf(pc.Attr)
				schemaType := pc.Params.AttrType
				sv := v
				if schemaType != nil {
					// Do type checking on response values
					if !schemaType.IsScalar() {
						return x.Errorf("Unknown Scalar:%v. Leaf predicate:'%v' must be"+
							" one of the scalar types defined in the schema.", pc.Params.AttrType, pc.Attr)
					}
					st := schemaType.(types.Scalar)
					// Convert to schema type.
					sv, err = st.Convert(v)
					if bytes.Equal(tv.Val, nil) || err != nil {
						// skip values that don't convert.
						return x.Errorf("_INV_")
					}
				} else if globalType != nil {
					// Try to coerce types if this is an optional scalar outside an
					// object definition.
					if !globalType.IsScalar() {
						return x.Errorf("Leaf predicate:'%v' must be a scalar.", pc.Attr)
					}
					gt := globalType.(types.Scalar)
					// Convert to schema type.
					sv, err = gt.Convert(v)
					if bytes.Equal(tv.Val, nil) || err != nil {
						continue
					}
				}
				dst.AddValue(pc.Attr, sv)
			}
		}
	}
	return nil
}

func createProperty(prop string, v types.Value) *graph.Property {
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
			sg.Params.DoCount = true
			break
		}
		if gchild.Attr == "_uid_" {
			sg.Params.GetUID = true
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
		x.AssertTruef(!strings.HasPrefix(exid, "_new_:"), "Query shouldn't contain _new_")
		euid = farm.Fingerprint64([]byte(exid))
		x.Trace(ctx, "Xid: %v Uid: %v", exid, euid)
	}

	if euid == 0 && gq.Gen == nil {
		err := x.Errorf("Invalid query, query internal id is zero and generator is nil")
		x.TraceError(ctx, err)
		return nil, err
	}

	// sg is to be returned.
	args := params{
		AttrType: schema.TypeOf(gq.Attr),
		isDebug:  gq.Attr == "debug",
	}

	sg := &SubGraph{
		Attr:   gq.Attr,
		Params: args,
		Filter: gq.Filter,
	}
	if gq.Gen != nil {
		err := sg.applyGenerator(ctx, gq.Gen)
		if err != nil {
			return nil, err
		}
	} else {
		// euid is the root UID.
		sg.SrcUIDs = &task.List{Uids: []uint64{euid}}
		sg.uidMatrix = []*task.List{&task.List{Uids: []uint64{euid}}}
	}
	sg.values = createNilValuesList(1)
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
func createTaskQuery(sg *SubGraph, uids *task.List, tokens []string,
	intersect *task.List) *task.Query {
	x.AssertTrue(uids == nil || tokens == nil)
	out := &task.Query{
		Attr:     sg.Attr,
		Tokens:   tokens,
		Count:    int32(sg.Params.Count),
		Offset:   int32(sg.Params.Offset),
		AfterUid: sg.Params.AfterUID,
		DoCount:  sg.Params.DoCount,
	}
	if uids != nil {
		out.Uids = uids.Uids
	}
	if intersect != nil {
		out.ToIntersect = intersect.Uids
	}
	return out
}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances. Note: taskQuery is nil for root node.
func ProcessGraph(ctx context.Context, sg *SubGraph, taskQuery *task.Query, rch chan error) {
	var err error
	if taskQuery != nil {
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
	}

	if sg.Params.DoCount {
		x.Trace(ctx, "Zero uids. Only count requested")
		rch <- nil
		return
	}

	sg.DestUIDs = algo.MergeSortedLists(sg.uidMatrix)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
		rch <- err
		return
	}

	if len(sg.DestUIDs.Uids) == 0 {
		// Looks like we're done here. Be careful with nil srcUIDs!
		x.Trace(ctx, "Zero uids. Num attr children: %v", len(sg.Children))
		rch <- nil
		return
	}

	// Apply filters if any.
	if err = sg.applyFilter(ctx); err != nil {
		rch <- err
		return
	}

	if len(sg.Params.Order) == 0 {
		// There is no ordering. Just apply pagination and return.
		if err = sg.applyPagination(ctx); err != nil {
			rch <- err
			return
		}
	} else {
		// We need to sort first before pagination.
		if err = sg.applyOrderAndPagination(ctx); err != nil {
			rch <- err
			return
		}
	}

	var processed, leftToProcess []*SubGraph
	// First iterate over the value nodes and check for validity.
	childChan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.SrcUIDs = sg.DestUIDs // Make the connection.
		if child.Params.AttrType == nil || child.Params.AttrType.IsScalar() {
			processed = append(processed, child)
			taskQuery := createTaskQuery(child, sg.DestUIDs, nil, nil)
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
			for i, uid := range sg.DestUIDs.Uids {
				tv := node.values[i]
				valBytes := tv.Val
				v, err := getValue(tv)
				if err != nil || bytes.Equal(valBytes, nil) {
					// The value is not as requested in schema.
					invalidUids[uid] = true
					continue
				}

				// type assertion for scalar type values.
				if !node.Params.AttrType.IsScalar() {
					rch <- x.Errorf("Fatal mistakes in type.")
				}

				// Check if compatible with schema type.
				schemaType := node.Params.AttrType.(types.Scalar)
				if _, err = schemaType.Convert(v); err != nil {
					invalidUids[uid] = true
				}
			}
		}
	}

	// Filter out the invalid UIDs.
	algo.ApplyFilter(sg.DestUIDs, func(uid uint64, idx int) bool {
		return !invalidUids[uid]
	})

	// Now process next level with valid UIDs.
	childChan = make(chan error, len(leftToProcess))
	for _, child := range leftToProcess {
		taskQuery := createTaskQuery(child, sg.DestUIDs, nil, nil)
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

func (sg *SubGraph) applyGenerator(ctx context.Context, gen *gql.Generator) error {
	if gen == nil { // No Generator.
		return nil
	}
	newSorted, err := runGenerator(ctx, gen)
	if err != nil {
		return err
	}
	sg.SrcUIDs = newSorted
	for _, uid := range newSorted.Uids {
		sg.uidMatrix = append(sg.uidMatrix, &task.List{Uids: []uint64{uid}})
	}
	return nil
}

func runGenerator(ctx context.Context, gen *gql.Generator) (*task.List, error) {
	if len(gen.FuncName) == 0 {
		return nil, nil
	}
	gen.FuncName = strings.ToLower(gen.FuncName)

	switch gen.FuncName {
	case "anyof":
		return anyOf(ctx, nil, gen.FuncArgs[0], gen.FuncArgs[1])
	case "allof":
		return allOf(ctx, nil, gen.FuncArgs[0], gen.FuncArgs[1])
	case "near":
		maxD, err := strconv.ParseFloat(gen.FuncArgs[2], 64)
		if err != nil {
			return nil, err
		}
		var g types.Geo
		geoD := strings.Replace(gen.FuncArgs[1], "'", "\"", -1)
		if err = g.UnmarshalText([]byte(geoD)); err != nil {
			return nil, err
		}
		gb, err := g.MarshalBinary()
		if err != nil {
			return nil, err
		}
		return generateGeo(ctx, gen.FuncArgs[0], geo.QueryTypeNear, gb, maxD)
	default:
		return nil, x.Errorf("Invalid generator")
	}

	return nil, nil
}

// applyFilter applies filters to sg.sorted.
func (sg *SubGraph) applyFilter(ctx context.Context) error {
	if sg.Filter == nil { // No filter.
		return nil
	}
	newSorted, err := runFilter(ctx, sg.DestUIDs, sg.Filter)
	if err != nil {
		return err
	}
	sg.DestUIDs = newSorted
	// For each posting list, intersect with sg.destUIDs.
	for _, l := range sg.uidMatrix {
		algo.IntersectSorted(l, sg.DestUIDs)
	}
	return nil
}

// runFilter traverses filter tree and produce a filtered list of UIDs.
// Input "destUIDs" is the very original list of destination UIDs of a SubGraph.
func runFilter(ctx context.Context, destUIDs *task.List,
	filter *gql.FilterTree) (*task.List, error) {
	if len(filter.FuncName) > 0 { // Leaf node.
		filter.FuncName = strings.ToLower(filter.FuncName)
		x.AssertTruef(len(filter.FuncArgs) == 2,
			"Expect exactly two arguments: pred and predValue")

		switch filter.FuncName {
		case "anyof":
			return anyOf(ctx, destUIDs, filter.FuncArgs[0], filter.FuncArgs[1])
		case "allof":
			return allOf(ctx, destUIDs, filter.FuncArgs[0], filter.FuncArgs[1])
		}
	}

	if filter.Op == "&" {
		// For intersect operator, we process the children serially.
		for _, c := range filter.Child {
			var err error
			if destUIDs, err = runFilter(ctx, destUIDs, c); err != nil {
				return nil, err
			}
		}
		return destUIDs, nil
	}

	// For now, we only handle AND and OR.
	if filter.Op != "|" {
		return destUIDs, x.Errorf("Unknown operator %v", filter.Op)
	}

	// For union operator, we do it in parallel.
	// First, get UIDs for child filters in parallel.
	type resultPair struct {
		uids *task.List
		err  error
	}
	resultChan := make(chan resultPair, len(filter.Child))
	for _, c := range filter.Child {
		go func(c *gql.FilterTree) {
			r, err := runFilter(ctx, destUIDs, c)
			resultChan <- resultPair{r, err}
		}(c)
	}

	lists := make([]*task.List, 0, len(filter.Child))
	// Next, collect the results from above goroutines.
	for i := 0; i < len(filter.Child); i++ {
		r := <-resultChan
		if r.err != nil {
			return destUIDs, r.err
		}
		lists = append(lists, r.uids)
	}
	return algo.MergeSortedLists(lists), nil
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
		algo.IntersectSorted(l, sg.DestUIDs)
		start, end := pageRange(&sg.Params, len(l.Uids))
		l.Uids = l.Uids[start:end]
	}
	// Re-merge the UID matrix.
	sg.DestUIDs = algo.MergeSortedLists(sg.uidMatrix)
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

// outputNode is the generic output / writer for preTraverse.
type outputNode interface {
	AddValue(attr string, v types.Value)
	AddChild(attr string, child outputNode)
	New(attr string) outputNode
	SetUID(uid uint64)
	SetXID(xid string)
}

// protoOutputNode is the proto output for preTraverse.
type protoOutputNode struct {
	*graph.Node
}

// AddValue adds an attribute value for protoOutputNode.
func (p *protoOutputNode) AddValue(attr string, v types.Value) {
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

// ToProtocolBuffer does preorder traversal to build a proto buffer. We have
// used postorder traversal before, but preorder seems simpler and faster for
// most cases.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*graph.Node, error) {
	var seedNode *protoOutputNode
	if sg.SrcUIDs == nil {
		return seedNode.New(sg.Attr).(*protoOutputNode).Node, nil
	}

	x.AssertTrue(len(sg.uidMatrix) == 1)
	n := seedNode.New(sg.Attr)
	ul := sg.uidMatrix[0]
	if sg.Params.GetUID || sg.Params.isDebug {
		n.SetUID(ul.Uids[0])
	}

	if rerr := sg.preTraverse(ul.Uids[0], n); rerr != nil {
		return n.(*protoOutputNode).Node, rerr
	}

	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n.(*protoOutputNode).Node, nil
}

// jsonOutputNode is the JSON output for preTraverse.
type jsonOutputNode struct {
	data map[string]interface{}
}

// AddValue adds an attribute value for jsonOutputNode.
func (p *jsonOutputNode) AddValue(attr string, v types.Value) {
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
	p.data["_uid_"] = fmt.Sprintf("%#x", uid)
}

// SetXID sets XID of a jsonOutputNode.
func (p *jsonOutputNode) SetXID(xid string) {
	p.data["_xid_"] = xid
}

// ToJSON converts the internal subgraph object to JSON format which is then\
// sent to the HTTP client.
func (sg *SubGraph) ToJSON(l *Latency) ([]byte, error) {
	var seedNode *jsonOutputNode
	n := seedNode.New(sg.Attr)
	ul := sg.uidMatrix[0]
	if sg.Params.GetUID || sg.Params.isDebug {
		n.SetUID(ul.Uids[0])
	}

	if err := sg.preTraverse(ul.Uids[0], n); err != nil {
		return nil, err
	}
	root := map[string]interface{}{
		sg.Attr: []map[string]interface{}{n.(*jsonOutputNode).data},
	}
	if sg.Params.isDebug {
		root["server_latency"] = l.ToMap()
	}
	return json.Marshal(root)
}
