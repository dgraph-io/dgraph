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
	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
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
	GetCount uint16
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
	GeoFilter *geo.Filter // TODO: We shouldn't have a special case for this.

	Counts *x.CountList
	Values *x.ValueList
	Result []*algo.UIDList

	// SrcUIDs is a list of unique source UIDs. They are always copies of destUIDs
	// of parent nodes in GraphQL structure.
	SrcUIDs *algo.UIDList

	// destUIDs is a list of destination UIDs, after applying filters, pagination.
	DestUIDs *algo.UIDList
}

// getValue gets the value from the task.
func getValue(tv task.Value) (types.Value, error) {
	vType := tv.ValType()
	valBytes := tv.ValBytes()
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
func (sg *SubGraph) preTraverse(uid uint64, dst preOutput) error {
	invalidUids := make(map[uint64]bool)
	// We go through all predicate children of the subgraph.
	for _, pc := range sg.Children {
		idx := pc.SrcUIDs.IndexOf(uid)
		x.AssertTruef(idx >= 0, "Attribute with uid not found in child Query uids")
		ul := pc.Result[idx]

		if pc.Counts != nil && pc.Counts.CountLength() > 0 {
			c := types.Int32(pc.Counts.Count(idx))
			uc := dst.New(pc.Attr)
			uc.AddValue("_count_", &c)
			dst.AddChild(pc.Attr, uc)
		} else if ul.Size() > 0 || len(pc.Children) > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			for i := 0; i < ul.Size(); i++ {
				uid := ul.Get(i)
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
			var tv task.Value
			if ok := pc.Values.Values(&tv, idx); !ok {
				return x.Errorf("While parsing value")
			}

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
					if bytes.Equal(tv.ValBytes(), nil) || err != nil {
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
					if bytes.Equal(tv.ValBytes(), nil) || err != nil {
						continue
					}
				}
				dst.AddValue(pc.Attr, sv)
			}
		}
	}

	//dst.Properties, dst.Children = properties, children
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
			sg.Params.GetCount = 1
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
		SrcUIDs: algo.NewUIDList([]uint64{euid}),
	}

	{
		// Encode uid into result flatbuffer.
		b := flatbuffers.NewBuilder(0)
		b.Finish(algo.NewUIDList([]uint64{euid}).AddTo(b))
		buf := b.FinishedBytes()
		tl := task.GetRootAsUidList(buf, 0)
		ul := new(algo.UIDList)
		ul.FromTask(tl)
		sg.Result = []*algo.UIDList{ul}
	}

	{
		// Also need to add nil value to keep this consistent.
		sg.Values = createNilValuesList(1)
	}
	return sg, nil
}

func createNilValuesList(count int) *x.ValueList {
	b := flatbuffers.NewBuilder(0)
	offsets := make([]flatbuffers.UOffsetT, count)
	for i := 0; i < count; i++ {
		bvo := b.CreateByteVector(x.Nilbyte)
		task.ValueStart(b)
		task.ValueAddVal(b, bvo)
		offsets[i] = task.ValueEnd(b)
	}

	task.ValueListStartValuesVector(b, count)
	for i := 0; i < count; i++ {
		b.PrependUOffsetT(offsets[i])
	}
	voffset := b.EndVector(count)

	task.ValueListStart(b)
	task.ValueListAddValues(b, voffset)
	b.Finish(task.ValueListEnd(b))
	buf := b.FinishedBytes()

	out := new(x.ValueList)
	x.Check(out.UnmarshalBinary(buf))
	return out
}

// createTaskQuery generates the query buffer.
func createTaskQuery(sg *SubGraph, uids *algo.UIDList, tokens []string,
	intersect *algo.UIDList) []byte {
	x.AssertTrue(uids == nil || tokens == nil)

	b := flatbuffers.NewBuilder(0)
	var vend flatbuffers.UOffsetT
	if uids != nil {
		task.QueryStartUidsVector(b, uids.Size())
		for i := uids.Size() - 1; i >= 0; i-- {
			b.PrependUint64(uids.Get(i))
		}
		vend = b.EndVector(uids.Size())
	} else {
		offsets := make([]flatbuffers.UOffsetT, 0, len(tokens))
		for _, token := range tokens {
			offsets = append(offsets, b.CreateString(token))
		}
		task.QueryStartTokensVector(b, len(tokens))
		for i := len(tokens) - 1; i >= 0; i-- {
			b.PrependUOffsetT(offsets[i])
		}
		vend = b.EndVector(len(tokens))
	}

	var intersectOffset flatbuffers.UOffsetT
	if intersect != nil {
		x.AssertTrue(uids == nil)
		x.AssertTrue(len(tokens) > 0)
		intersectOffset = intersect.AddTo(b)
	}

	ao := b.CreateString(sg.Attr)
	task.QueryStart(b)
	task.QueryAddAttr(b, ao)
	if uids != nil {
		task.QueryAddUids(b, vend)
	} else {
		task.QueryAddTokens(b, vend)
	}
	if intersect != nil {
		task.QueryAddToIntersect(b, intersectOffset)
	}
	task.QueryAddCount(b, int32(sg.Params.Count))
	task.QueryAddOffset(b, int32(sg.Params.Offset))
	task.QueryAddAfterUid(b, sg.Params.AfterUID)
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

		r := task.GetRootAsResult(resultBuf, 0)

		// Extract UIDLists from task.Result.
		sg.Result = algo.FromTaskResult(r)

		// Extract values from task.Result.
		sg.Values = new(x.ValueList)
		x.AssertTrue(r.Values(&sg.Values.ValueList) != nil)
		if sg.Values.ValuesLength() > 0 {
			var v task.Value
			if sg.Values.Values(&v, 0) {
				x.Trace(ctx, "Sample value for attr: %v Val: %v", sg.Attr, string(v.ValBytes()))
			}
		}

		// Extract counts from task.Result.
		sg.Counts = new(x.CountList)
		x.AssertTrue(r.Count(&sg.Counts.CountList) != nil)
	}

	if sg.Params.GetCount == 1 {
		x.Trace(ctx, "Zero uids. Only count requested")
		rch <- nil
		return
	}

	if err = sg.applyGeoQuery(ctx); err != nil {
		rch <- err
		return
	}

	sg.DestUIDs = algo.MergeLists(sg.Result)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
		rch <- err
		return
	}

	if sg.DestUIDs.Size() == 0 {
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
			var tv task.Value
			for i := 0; i < node.Values.ValuesLength(); i++ {
				uid := sg.DestUIDs.Get(i)
				if ok := node.Values.Values(&tv, i); !ok {
					invalidUids[uid] = true
				}

				valBytes := tv.ValBytes()
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
	sg.DestUIDs.ApplyFilter(func(uid uint64, idx int) bool {
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
	for _, l := range sg.Result {
		l.Intersect(sg.DestUIDs)
	}
	return nil
}

// runFilter traverses filter tree and produce a filtered list of UIDs.
// Input "destUIDs" is the very original list of destination UIDs of a SubGraph.
func runFilter(ctx context.Context, destUIDs *algo.UIDList,
	filter *gql.FilterTree) (*algo.UIDList, error) {
	if len(filter.FuncName) > 0 { // Leaf node.
		filter.FuncName = strings.ToLower(filter.FuncName) // Not sure if needed.
		isAnyOf := filter.FuncName == "anyof"
		isAllOf := filter.FuncName == "allof"
		x.AssertTruef(isAnyOf || isAllOf, "FuncName invalid: %s", filter.FuncName)
		x.AssertTruef(len(filter.FuncArgs) == 2,
			"Expect exactly two arguments: pred and predValue")

		attr := filter.FuncArgs[0]
		sg := &SubGraph{Attr: attr}
		sgChan := make(chan error, 1)

		// Tokenize FuncArgs[1].
		tokenizer, err := tok.NewTokenizer([]byte(filter.FuncArgs[1]))
		if err != nil {
			return nil, x.Errorf("Could not create tokenizer: %v", filter.FuncArgs[1])
		}
		defer tokenizer.Destroy()
		x.AssertTrue(tokenizer != nil)
		tokens := tokenizer.Tokens()
		taskQuery := createTaskQuery(sg, nil, tokens, destUIDs)
		go ProcessGraph(ctx, sg, taskQuery, sgChan)
		select {
		case <-ctx.Done():
			return nil, x.Wrap(ctx.Err())
		case err = <-sgChan:
			if err != nil {
				return nil, err
			}
		}

		x.AssertTrue(len(sg.Result) == len(tokens))
		if isAnyOf {
			return algo.MergeLists(sg.Result), nil
		}
		return algo.IntersectLists(sg.Result), nil
	}

	// For now, we only handle AND and OR.
	if filter.Op != "&" && filter.Op != "|" {
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
	if filter.Op == "|" {
		return algo.MergeLists(lists), nil
	}
	x.AssertTrue(filter.Op == "&")
	return algo.IntersectLists(lists), nil
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
	x.AssertTrue(sg.SrcUIDs.Size() == len(sg.Result))
	for _, l := range sg.Result {
		l.Intersect(sg.DestUIDs)
		start, end := pageRange(&sg.Params, l.Size())
		l.Slice(start, end)
	}
	// Re-merge the UID matrix.
	sg.DestUIDs = algo.MergeLists(sg.Result)
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

	b := flatbuffers.NewBuilder(0)
	ao := b.CreateString(sg.Params.Order)

	// Add UID matrix.
	uidOffsets := make([]flatbuffers.UOffsetT, 0, len(sg.Result))
	for _, ul := range sg.Result {
		uidOffsets = append(uidOffsets, ul.AddTo(b))
	}

	task.SortStartUidmatrixVector(b, len(uidOffsets))
	for i := len(uidOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(uidOffsets[i])
	}
	uEnd := b.EndVector(len(uidOffsets))

	task.SortStart(b)
	task.SortAddAttr(b, ao)
	task.SortAddUidmatrix(b, uEnd)
	task.SortAddOffset(b, int32(sg.Params.Offset))
	task.SortAddCount(b, int32(sg.Params.Count))
	b.Finish(task.SortEnd(b))

	resultData, err := worker.SortOverNetwork(ctx, b.FinishedBytes())
	if err != nil {
		return err
	}

	// Copy result into our UID matrix.
	result := task.GetRootAsSortResult(resultData, 0)
	x.AssertTrue(result.UidmatrixLength() == len(sg.Result))
	sg.Result = algo.FromSortResult(result)

	// Update sg.destUID. Iterate over the UID matrix (which is not sorted by
	// UID). For each element in UID matrix, we do a binary search in the
	// current destUID and mark it. Then we scan over this bool array and
	// rebuild destUIDs.
	included := make([]bool, sg.DestUIDs.Size())
	for _, ul := range sg.Result {
		for i := 0; i < ul.Size(); i++ {
			uid := ul.Get(i)
			idx := sg.DestUIDs.IndexOf(uid) // Binary search.
			if idx >= 0 {
				included[idx] = true
			}
		}
	}
	sg.DestUIDs.ApplyFilter(
		func(uid uint64, idx int) bool { return included[idx] })
	return nil
}

// preOutput is the generic output / writer for preTraverse.
type preOutput interface {
	AddValue(attr string, v types.Value)
	AddChild(attr string, child preOutput)
	New(attr string) preOutput
	SetUID(uid uint64)
	SetXID(xid string)
}

// protoPreOutput is the proto output for preTraverse.
type protoPreOutput struct {
	*graph.Node
}

// AddValue adds an attribute value for protoPreOutput.
func (p *protoPreOutput) AddValue(attr string, v types.Value) {
	p.Node.Properties = append(p.Node.Properties, createProperty(attr, v))
}

// AddChild adds a child for protoPreOutput.
func (p *protoPreOutput) AddChild(attr string, child preOutput) {
	p.Node.Children = append(p.Node.Children, child.(*protoPreOutput).Node)
}

// New creates a new node for protoPreOutput.
func (p *protoPreOutput) New(attr string) preOutput {
	uc := nodePool.Get().(*graph.Node)
	uc.Attribute = attr
	return &protoPreOutput{uc}
}

// SetUID sets UID of a protoPreOutput.
func (p *protoPreOutput) SetUID(uid uint64) { p.Node.Uid = uid }

// SetXID sets XID of a protoPreOutput.
func (p *protoPreOutput) SetXID(xid string) { p.Node.Xid = xid }

// ToProtocolBuffer does preorder traversal to build a proto buffer. We have
// used postorder traversal before, but preorder seems simpler and faster for
// most cases.
func (sg *SubGraph) ToProtocolBuffer(l *Latency) (*graph.Node, error) {
	var dummy *protoPreOutput
	if sg.SrcUIDs == nil {
		return dummy.New(sg.Attr).(*protoPreOutput).Node, nil
	}

	x.AssertTrue(len(sg.Result) == 1)
	n := dummy.New(sg.Attr)
	ul := sg.Result[0]
	if sg.Params.GetUID || sg.Params.isDebug {
		n.SetUID(ul.Get(0))
	}

	if rerr := sg.preTraverse(ul.Get(0), n); rerr != nil {
		return n.(*protoPreOutput).Node, rerr
	}

	l.ProtocolBuffer = time.Since(l.Start) - l.Parsing - l.Processing
	return n.(*protoPreOutput).Node, nil
}

// jsonPreOutput is the JSON output for preTraverse.
type jsonPreOutput struct {
	data map[string]interface{}
}

// AddValue adds an attribute value for jsonPreOutput.
func (p *jsonPreOutput) AddValue(attr string, v types.Value) {
	p.data[attr] = v
}

// AddChild adds a child for jsonPreOutput.
func (p *jsonPreOutput) AddChild(attr string, child preOutput) {
	a := p.data[attr]
	if a == nil {
		// Need to do this because we cannot cast nil interface to
		// []map[string]interface{}.
		a = make([]map[string]interface{}, 0, 5)
	}
	p.data[attr] = append(a.([]map[string]interface{}),
		child.(*jsonPreOutput).data)
}

// New creates a new node for jsonPreOutput.
func (p *jsonPreOutput) New(attr string) preOutput {
	return &jsonPreOutput{make(map[string]interface{})}
}

// SetUID sets UID of a jsonPreOutput.
func (p *jsonPreOutput) SetUID(uid uint64) {
	p.data["_uid_"] = fmt.Sprintf("%#x", uid)
}

// SetXID sets XID of a jsonPreOutput.
func (p *jsonPreOutput) SetXID(xid string) {
	p.data["_xid_"] = xid
}

// ToJSON converts the internal subgraph object to JSON format which is then\
// sent to the HTTP client.
func (sg *SubGraph) ToJSON(l *Latency) ([]byte, error) {
	var dummy *jsonPreOutput
	n := dummy.New(sg.Attr)
	ul := sg.Result[0]
	if sg.Params.GetUID || sg.Params.isDebug {
		n.SetUID(ul.Get(0))
	}

	if err := sg.preTraverse(ul.Get(0), n); err != nil {
		return nil, err
	}
	root := map[string]interface{}{
		sg.Attr: []map[string]interface{}{n.(*jsonPreOutput).data},
	}

	if sg.GeoFilter != nil {
		// This part for Geo filter is hackish. We should clean up soon.
		x.AssertTrue(sg.Attr == "geometry")
		x.AssertTrue(len(sg.Result) == 1 || len(sg.Result) == 2)
		x.AssertTruef(sg.SrcUIDs.Size() == 1 || sg.SrcUIDs.Size() == 2,
			sg.SrcUIDs.DebugString())
		x.AssertTruef(sg.DestUIDs.Size() == 1 || sg.DestUIDs.Size() == 2,
			sg.DestUIDs.DebugString())
		root = root["geometry"].([]map[string]interface{})[0]
	}
	return json.Marshal(root)
}
