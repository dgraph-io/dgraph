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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
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
	m["parsing"] = x.Round(l.Parsing).String()
	m["processing"] = x.Round(l.Processing).String()
	m["json"] = x.Round(j).String()
	m["total"] = x.Round(time.Since(l.Start)).String()
	return m
}

type params struct {
	Alias      string
	Count      int
	Offset     int
	AfterUID   uint64
	DoCount    bool
	GetUID     bool
	Order      string
	OrderDesc  bool
	isDebug    bool
	Var        string
	NeedsVar   []string
	ParentVars map[string]*task.List
	Normalize  bool
	From       uint64
	To         uint64
	Facet      *facets.Param
}

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr         string
	Params       params
	counts       []uint32
	values       []*task.Value
	uidMatrix    []*task.List
	facetsMatrix []*facets.List

	// SrcUIDs is a list of unique source UIDs. They are always copies of destUIDs
	// of parent nodes in GraphQL structure.
	SrcUIDs *task.List
	SrcFunc []string

	FilterOp     string
	Filters      []*SubGraph
	facetsFilter *facets.FilterTree
	Children     []*SubGraph

	// destUIDs is a list of destination UIDs, after applying filters, pagination.
	DestUIDs *task.List
}

// DebugPrint prints out the SubGraph tree in a nice format for debugging purposes.
func (sg *SubGraph) DebugPrint(prefix string) {
	var src, dst int
	if sg.SrcUIDs != nil {
		src = algo.ListLen(sg.SrcUIDs)
	}
	if sg.DestUIDs != nil {
		dst = algo.ListLen(sg.DestUIDs)
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

var (
	ErrEmptyVal = errors.New("query: harmless error, e.g. task.Val is nil")
)

// This method gets the values and children for a subgraph.
func (sg *SubGraph) preTraverse(uid uint64, dst, parent outputNode) error {
	invalidUids := make(map[uint64]bool)
	uidAlreadySet := false

	facetsNode := dst.New("@facets")
	// We go through all predicate children of the subgraph.
	for _, pc := range sg.Children {
		idxi, idxj := algo.IndexOf(pc.SrcUIDs, uid)
		if idxi < 0 {
			continue
		}
		idx := algo.Idx(pc.SrcUIDs, idxi, idxj)
		ul := pc.uidMatrix[idx]

		fieldName := pc.Attr
		if pc.Params.Alias != "" {
			fieldName = pc.Params.Alias
		}
		if !uidAlreadySet && (sg.Params.GetUID || sg.Params.isDebug) {
			uidAlreadySet = true
			dst.SetUID(uid)
		}
		if len(pc.counts) > 0 {
			c := types.ValueForType(types.Int32ID)
			c.Value = int32(pc.counts[idx])
			uc := dst.New(pc.Attr)
			uc.AddValue("count", c)
			dst.AddListChild(pc.Attr, uc)
		} else if len(pc.SrcFunc) > 0 && isAggregatorFn(pc.SrcFunc[0]) {
			if idx > 0 { // aggregator will put value at index 0; place once
				continue
			}
			// add sg.Attr as child on 'parent' instead of 'dst', otherwise
			// within output, aggregator will messed with other attrs
			uc := parent.New(sg.Attr)
			name := fmt.Sprintf("%s(%s)", pc.SrcFunc[0], pc.Attr)
			sv, err := convertWithBestEffort(pc.values[0], pc.Attr)
			if err != nil && err != ErrEmptyVal {
				return err
			}
			uc.AddValue(name, sv)
			parent.AddListChild(sg.Attr, uc)
		} else if len(pc.SrcFunc) > 0 && pc.SrcFunc[0] == "checkpwd" {
			c := types.ValueForType(types.BoolID)
			c.Value = task.ToBool(pc.values[idx])
			uc := dst.New(pc.Attr)
			uc.AddValue("checkpwd", c)
			dst.AddListChild(pc.Attr, uc)
		} else if algo.ListLen(ul) > 0 || len(pc.Children) > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			var fcsList []*facets.Facets
			if pc.Params.Facet != nil {
				fcsList = pc.facetsMatrix[idx].FacetsList
			}
			it := algo.NewListIterator(ul)
			for childIdx := -1; it.Valid(); it.Next() {
				childIdx++
				childUID := it.Val()
				if invalidUids[childUID] {
					continue
				}
				uc := dst.New(fieldName)
				if rerr := pc.preTraverse(childUID, uc, dst); rerr != nil {
					if rerr.Error() == "_INV_" {
						invalidUids[childUID] = true
						continue // next UID.
					}
					// Some other error.
					log.Printf("Error while traversal: %v", rerr)
					return rerr
				}
				if pc.Params.Facet != nil && len(fcsList) > childIdx {
					fs := fcsList[childIdx]
					fc := dst.New(fieldName)
					for _, f := range fs.Facets {
						fc.AddValue(f.Key, types.ValFor(f))
					}
					if !fc.IsEmpty() {
						fcParent := dst.New("_")
						fcParent.AddMapChild("_", fc, false)
						uc.AddMapChild("@facets", fcParent, true)
					}
				}
				if !uc.IsEmpty() {
					dst.AddListChild(fieldName, uc)
				}
			}
		} else {
			// attribute is non-scalar, just let it go
			if typ, terr := schema.State().TypeOf(pc.Attr); terr == nil && !typ.IsScalar() {
				continue
			}
			tv := pc.values[idx]
			v, err := getValue(tv)
			if err != nil {
				return err
			}
			if pc.Params.Facet != nil && len(pc.facetsMatrix[idx].FacetsList) > 0 {
				fc := dst.New(fieldName)
				// in case of Value we have only one Facets
				for _, f := range pc.facetsMatrix[idx].FacetsList[0].Facets {
					fc.AddValue(f.Key, types.ValFor(f))
				}
				if !fc.IsEmpty() {
					facetsNode.AddMapChild(fieldName, fc, false)
				}
			}

			if pc.Attr == "_xid_" {
				txt, err := types.Convert(v, types.StringID)
				if err != nil {
					return err
				}
				dst.SetXID(txt.Value.(string))
			} else if pc.Attr == "_uid_" {
				if !uidAlreadySet {
					uidAlreadySet = true
					dst.SetUID(uid)
				}
			} else {
				// if conversion not possible, we ignore it in the result.
				sv, convErr := convertWithBestEffort(tv, pc.Attr)
				if convErr == ErrEmptyVal {
					continue
				} else if convErr != nil {
					return convErr
				}
				// Only strings can have empty values.
				if sv.Tid == types.StringID && sv.Value.(string) == "_nil_" {
					sv.Value = ""
				}
				if !pc.Params.Normalize {
					dst.AddValue(fieldName, sv)
					continue
				}
				// If the query had the normalize directive, then we only add nodes
				// with an Alias.
				if pc.Params.Alias != "" {
					dst.AddValue(fieldName, sv)
				}
			}
		}
	}

	if !facetsNode.IsEmpty() {
		dst.AddMapChild("@facets", facetsNode, false)
	}
	return nil
}

// convert from task.Val to types.Value which is determined by attr
// if convert failed, try convert to types.StringID
func convertWithBestEffort(tv *task.Value, attr string) (types.Val, error) {
	v, _ := getValue(tv)
	typ, err := schema.State().TypeOf(attr)
	sv := types.ValueForType(types.StringID)
	if err == nil {
		// Try to coerce types if this is an optional scalar outside an
		// object definition.
		if !typ.IsScalar() {
			return sv, x.Errorf("Leaf predicate:'%v' must be a scalar.", attr)
		}
		sv, err = types.Convert(v, typ)
		if bytes.Equal(tv.Val, nil) || err != nil {
			return sv, ErrEmptyVal
		}
	} else {
		sv, err = types.Convert(v, types.StringID)
		x.Check(err)
	}
	if bytes.Equal(tv.Val, nil) {
		return sv, ErrEmptyVal
	}
	return sv, nil
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

func filterCopy(sg *SubGraph, ft *gql.FilterTree) error {
	// Either we'll have an operation specified, or the function specified.
	if len(ft.Op) > 0 {
		sg.FilterOp = ft.Op
	} else {
		sg.Attr = ft.Func.Attr
		if !isValidFuncName(ft.Func.Name) {
			return x.Errorf("Invalid function name : %s", ft.Func.Name)
		}
		sg.SrcFunc = append(sg.SrcFunc, ft.Func.Name)
		sg.SrcFunc = append(sg.SrcFunc, ft.Func.Args...)
		sg.Params.NeedsVar = append(sg.Params.NeedsVar, ft.Func.NeedsVar...)
	}
	for _, ftc := range ft.Child {
		child := &SubGraph{}
		if err := filterCopy(child, ftc); err != nil {
			return err
		}
		sg.Filters = append(sg.Filters, child)
	}
	return nil
}

func treeCopy(ctx context.Context, gq *gql.GraphQuery, sg *SubGraph) error {
	// Typically you act on the current node, and leave recursion to deal with
	// children. But, in this case, we don't want to muck with the current
	// node, because of the way we're dealing with the root node.
	// So, we work on the children, and then recurse for grand children.

	for _, gchild := range gq.Children {
		if gchild.Attr == "_uid_" {
			sg.Params.GetUID = true
		} else if gchild.Attr == "password" { // query password is forbidden
			if gchild.Func == nil || !gchild.Func.IsPasswordVerifier() {
				return errors.New("Password is not fetchable")
			}
		}

		args := params{
			Alias:     gchild.Alias,
			isDebug:   sg.Params.isDebug,
			Var:       gchild.Var,
			Normalize: sg.Params.Normalize,
		}
		if gchild.Facets != nil {
			args.Facet = &facets.Param{gchild.Facets.AllKeys, gchild.Facets.Keys}
		}

		args.NeedsVar = append(args.NeedsVar, gchild.NeedsVar...)
		if gchild.IsCount {
			if len(gchild.Children) != 0 {
				return errors.New("Node with count cannot have child attributes")
			}
			args.DoCount = true
		}

		for argk, _ := range gchild.Args {
			if !isValidArg(argk) {
				return x.Errorf("Invalid argument : %s", argk)
			}
		}
		if err := args.fill(gchild); err != nil {
			return err
		}

		dst := &SubGraph{
			Attr:   gchild.Attr,
			Params: args,
		}

		if gchild.Func != nil &&
			(gchild.Func.IsAggregator() || gchild.Func.IsPasswordVerifier()) {
			f := gchild.Func.Name
			if len(gchild.Children) != 0 {
				note := fmt.Sprintf("Node with %q cant have child attr", f)
				return errors.New(note)
			}
			// embedded filter will cause ambiguous output like following,
			// director.film @filter(gt(initial_release_date, "2016")) {
			//    min(initial_release_date @filter(gt(initial_release_date, "1986"))
			// }
			if gchild.Filter != nil {
				note := fmt.Sprintf("Node with %q cant have filter,", f) +
					" please place the filter on the upper level"
				return errors.New(note)
			}
			dst.SrcFunc = append(dst.SrcFunc, gchild.Func.Name)
			dst.SrcFunc = append(dst.SrcFunc, gchild.Func.Args...)
		}

		if gchild.Filter != nil {
			dstf := &SubGraph{}
			if err := filterCopy(dstf, gchild.Filter); err != nil {
				return err
			}
			dst.Filters = append(dst.Filters, dstf)
		}

		if gchild.FacetsFilter != nil {
			facetsFilter, err := toFacetsFilter(gchild.FacetsFilter)
			if err != nil {
				return err
			}
			dst.facetsFilter = facetsFilter
		}

		sg.Children = append(sg.Children, dst)
		if err := treeCopy(ctx, gchild, dst); err != nil {
			return err
		}
	}
	return nil
}

func (args *params) fill(gq *gql.GraphQuery) error {

	if v, ok := gq.Args["offset"]; ok {
		offset, err := strconv.ParseInt(v, 0, 32)
		if err != nil {
			return err
		}
		args.Offset = int(offset)
	}
	if v, ok := gq.Args["after"]; ok {
		after, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return err
		}
		args.AfterUID = uint64(after)
	}
	if v, ok := gq.Args["from"]; ok {
		from, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			// Treat it as an XID.
			from = farm.Fingerprint64([]byte(v))
		}
		args.From = uint64(from)
	}
	if v, ok := gq.Args["to"]; ok {
		to, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			// Treat it as an XID.
			to = farm.Fingerprint64([]byte(v))
		}
		args.To = uint64(to)
	}
	if v, ok := gq.Args["first"]; ok {
		first, err := strconv.ParseInt(v, 0, 32)
		if err != nil {
			return err
		}
		args.Count = int(first)
	}
	if v, ok := gq.Args["orderasc"]; ok {
		args.Order = v
	} else if v, ok := gq.Args["orderdesc"]; ok {
		args.Order = v
		args.OrderDesc = true
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
	if len(gq.UID) == 0 && gq.Func == nil && len(gq.NeedsVar) == 0 && gq.Alias != "shortest" {
		err := x.Errorf("Invalid query, query internal id is zero and generator is nil")
		x.TraceError(ctx, err)
		return nil, err
	}

	var debug bool
	// gRPC client passes information about debug as metadata.
	if md, ok := metadata.FromContext(ctx); ok {
		// md is a map[string][]string
		debug = len(md["debug"]) > 0 && md["debug"][0] == "true"
	}
	// HTTP passes information about debug as query parameter which is attached to context.
	debug = debug || ctx.Value("debug") == "true"

	// For the root, the name to be used in result is stored in Alias, not Attr.
	// The attr at root (if present) would stand for the source functions attr.
	args := params{
		isDebug:    debug,
		Alias:      gq.Alias,
		Var:        gq.Var,
		ParentVars: make(map[string]*task.List),
		Normalize:  gq.Normalize,
	}
	if gq.Facets != nil {
		args.Facet = &facets.Param{gq.Facets.AllKeys, gq.Facets.Keys}
	}

	for _, it := range gq.NeedsVar {
		args.NeedsVar = append(args.NeedsVar, it)
	}

	for argk, _ := range gq.Args {
		if !isValidArg(argk) {
			return nil, x.Errorf("Invalid argument : %s", argk)
		}
	}
	if err := args.fill(gq); err != nil {
		return nil, err
	}

	sg := &SubGraph{
		Params: args,
	}

	if gq.Func != nil {
		sg.Attr = gq.Func.Attr
		if !isValidFuncName(gq.Func.Name) {
			return nil, x.Errorf("Invalid function name : %s", gq.Func.Name)
		}
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Name)
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Args...)
	}
	if len(gq.UID) > 0 {
		o := new(task.List)
		sg.SrcUIDs = new(task.List)
		ito := algo.NewWriteIterator(o, 0)
		itsg := algo.NewWriteIterator(sg.SrcUIDs, 0)
		for _, uid := range gq.UID {
			ito.Append(uid)
			itsg.Append(uid)
		}
		ito.End()
		itsg.End()
		sg.uidMatrix = []*task.List{o}
		// User specified list may not be sorted.
		algo.Sort(sg.SrcUIDs)
	}
	sg.values = createNilValuesList(1)
	// Copy roots filter.
	if gq.Filter != nil {
		sgf := &SubGraph{}
		if err := filterCopy(sgf, gq.Filter); err != nil {
			return nil, err
		}
		sg.Filters = append(sg.Filters, sgf)
	}
	if gq.FacetsFilter != nil {
		facetsFilter, err := toFacetsFilter(gq.FacetsFilter)
		if err != nil {
			return nil, err
		}
		sg.facetsFilter = facetsFilter
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

func toFacetsFilter(gft *gql.FilterTree) (*facets.FilterTree, error) {
	if gft == nil {
		return nil, nil
	}
	if gft.Func != nil && len(gft.Func.NeedsVar) != 0 {
		return nil, x.Errorf("Variables not supported in facets.FilterTree")
	}
	ftree := new(facets.FilterTree)
	ftree.Op = gft.Op
	for _, gftc := range gft.Child {
		if ftc, err := toFacetsFilter(gftc); err != nil {
			return nil, err
		} else {
			ftree.Children = append(ftree.Children, ftc)
		}
	}
	if gft.Func != nil {
		ftree.Func = &facets.Function{
			Key:  gft.Func.Attr,
			Name: gft.Func.Name,
			Args: []string{},
		}
		ftree.Func.Args = append(ftree.Func.Args, gft.Func.Args...)
	}
	return ftree, nil
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
		Attr:         attr,
		Reverse:      reverse,
		SrcFunc:      sg.SrcFunc,
		Count:        int32(sg.Params.Count),
		Offset:       int32(sg.Params.Offset),
		AfterUid:     sg.Params.AfterUID,
		DoCount:      len(sg.Filters) == 0 && sg.Params.DoCount,
		FacetParam:   sg.Params.Facet,
		FacetsFilter: sg.facetsFilter,
	}
	if sg.SrcUIDs != nil {
		out.Uids = sg.SrcUIDs
	}
	return out
}

func ProcessQuery(ctx context.Context, res gql.Result, l *Latency) ([]*SubGraph, error) {
	var sgl []*SubGraph

	// doneVars will store the UID list of the corresponding variables.
	doneVars := make(map[string]*task.List)
	loopStart := time.Now()
	for i := 0; i < len(res.Query); i++ {
		gq := res.Query[i]
		if gq == nil || (len(gq.UID) == 0 && gq.Func == nil &&
			len(gq.NeedsVar) == 0 && gq.Alias != "shortest") {
			continue
		}
		sg, err := ToSubGraph(ctx, gq)
		if err != nil {
			return nil, err
		}
		x.Trace(ctx, "Query parsed")
		sgl = append(sgl, sg)
	}
	l.Parsing += time.Since(loopStart)

	execStart := time.Now()
	hasExecuted := make([]bool, len(sgl))
	numQueriesDone := 0

	// canExecute returns true if a query block is ready to execute with all the variables
	// that it depends on are already populated or are defined in the same block.
	canExecute := func(idx int) bool {
		for _, v := range res.QueryVars[idx].Needs {
			// here we check if this block defines the variable v.
			var selfDep bool
			for _, vd := range res.QueryVars[idx].Defines {
				if v == vd {
					selfDep = true
					break
				}
			}
			// The variable should be defined in this block or should have already been
			// populated by some other block, otherwise we are not ready to execute yet.
			if _, ok := doneVars[v]; !ok && !selfDep {
				return false
			}
		}
		return true
	}

	for numQueriesDone < len(sgl) {
		errChan := make(chan error, len(sgl))
		var idxList []int
		// If we have N blocks in a query, it can take a maximum of N iterations for all of them
		// to be executed.
		for idx := 0; idx < len(sgl); idx++ {
			if hasExecuted[idx] {
				continue
			}
			sg := sgl[idx]
			// Check the list for the requires variables.
			if !canExecute(idx) {
				continue
			}

			sg.recursiveFillVars(doneVars)
			hasExecuted[idx] = true
			numQueriesDone++
			idxList = append(idxList, idx)
			if sg.Params.Alias == "shortest" {
				err := ShortestPath(ctx, sg)
				if err != nil {
					return nil, err
				}
			} else {
				go ProcessGraph(ctx, sg, nil, errChan)
			}
			x.Trace(ctx, "Graph processed")
		}

		// Wait for the execution that was started in this iteration.
		for i := 0; i < len(idxList); i++ {
			if sgl[idxList[i]].Params.Alias == "shortest" {
				continue
			}
			select {
			case err := <-errChan:
				if err != nil {
					x.TraceError(ctx, x.Wrapf(err, "Error while processing Query"))
					return nil, err
				}
			case <-ctx.Done():
				x.TraceError(ctx, x.Wrapf(ctx.Err(), "Context done before full execution"))
				return nil, ctx.Err()
			}
		}

		// If the executed subgraph had some variable defined in it, Populate it in the map.
		for _, idx := range idxList {
			sg := sgl[idx]
			if len(res.QueryVars[idx].Defines) == 0 {
				continue
			}

			isCascade := shouldCascade(res, idx)
			populateVarMap(sg, doneVars, isCascade)
		}
	}

	// Ensure all the queries are executed.
	for _, it := range hasExecuted {
		if !it {
			return nil, x.Errorf("Query couldn't be executed")
		}
	}
	l.Processing += time.Since(execStart)

	return sgl, nil
}

// shouldCascade returns true if the query block is not self depenedent and we should
// remove the uids from the bottom up if the children are empty.
func shouldCascade(res gql.Result, idx int) bool {
	if res.Query[idx].Attr == "shortest" {
		return false
	}

	for _, def := range res.QueryVars[idx].Defines {
		for _, need := range res.QueryVars[idx].Needs {
			if def == need {
				return false
			}
		}
	}
	return true
}

// TODO(Ashwin): Benchmark this function.
func populateVarMap(sg *SubGraph, doneVars map[string]*task.List, isCascade bool) {
	o := new(task.List)
	out := algo.NewWriteIterator(o, 0)
	it := algo.NewListIterator(sg.DestUIDs)
	i := -1
	if sg.Params.Alias == "shortest" {
		goto AssignStep
	}
	for _, child := range sg.Children {
		populateVarMap(child, doneVars, isCascade)
		if !isCascade {
			continue
		}

		// Intersect the UidMatrix with the DestUids as some UIDs might have been removed
		// by other operations. So we need to apply it on the UidMatrix.
		for _, l := range child.uidMatrix {
			algo.IntersectWith(l, child.DestUIDs)
		}
	}

	if !isCascade {
		goto AssignStep
	}

	// Filter out UIDs that don't have atleast one UID in every child.
	for ; it.Valid(); it.Next() {
		i++
		var exclude bool
		for _, child := range sg.Children {
			// If the length of child UID list is zero and it has no valid value, then the
			// current UID should be removed from this level.
			if len(child.values[i].Val) == 0 && algo.ListLen(child.uidMatrix[i]) == 0 {
				exclude = true
				break
			}
		}
		if !exclude {
			out.Append(it.Val())
		}
	}
	out.End()
	// Note the we can't overwrite DestUids, as it'd also modify the SrcUids of
	// next level and the mapping from SrcUids to uidMatrix would be lost.
	sg.DestUIDs = o

AssignStep:
	if sg.Params.Var != "" {
		doneVars[sg.Params.Var] = sg.DestUIDs
	}
}

func (sg *SubGraph) recursiveFillVars(doneVars map[string]*task.List) {
	sg.fillVars(doneVars)
	for _, child := range sg.Children {
		child.recursiveFillVars(doneVars)
	}
	for _, fchild := range sg.Filters {
		fchild.recursiveFillVars(doneVars)
	}
}

func (sg *SubGraph) fillVars(mp map[string]*task.List) {
	lists := make([]*task.List, 0, 3)
	if sg.DestUIDs != nil {
		lists = append(lists, sg.DestUIDs)
	}
	for _, v := range sg.Params.NeedsVar {
		if l, ok := mp[v]; ok {
			lists = append(lists, l)
		}
	}
	sg.DestUIDs = algo.MergeSorted(lists)
}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances. Note: taskQuery is nil for root node.
func ProcessGraph(ctx context.Context, sg, parent *SubGraph, rch chan error) {
	var err error
	if len(sg.Params.NeedsVar) != 0 && len(sg.SrcFunc) == 0 {
		// Retain the actual order in uidMatrix. But sort the destUids.
		sg.uidMatrix = []*task.List{sg.DestUIDs}
		it := algo.NewListIterator(sg.DestUIDs)
		var o task.List
		wit := algo.NewWriteIterator(&o, 0)
		for ; it.Valid(); it.Next() {
			wit.Append(it.Val())
		}
		wit.End()
		algo.Sort(&o)
		sg.DestUIDs = &o
	} else if len(sg.Attr) == 0 {
		// If we have a filter SubGraph which only contains an operator,
		// it won't have any attribute to work on.
		// This is to allow providing SrcUIDs to the filter children.
		sg.DestUIDs = sg.SrcUIDs
	} else if parent == nil && len(sg.SrcFunc) == 0 {
		// I am root. I don't have any function to execute, and my
		// result has been prepared for me already.
		sg.DestUIDs = sg.SrcUIDs
	} else {
		if len(sg.SrcFunc) > 0 && sg.SrcFunc[0] == "id" {
			// If its an id() filter, we just have to intersect the SrcUIDs with DestUIDs
			// and return.
			sg.fillVars(sg.Params.ParentVars)
			algo.IntersectWith(sg.DestUIDs, sg.SrcUIDs)
			rch <- nil
			return
		}

		taskQuery := createTaskQuery(sg)
		result, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while processing task"))
			rch <- err
			return
		}

		sg.uidMatrix = result.UidMatrix
		sg.values = result.Values
		sg.facetsMatrix = result.FacetMatrix
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

		if parent == nil {
			// I'm root. We reach here if root had a function.
			sg.uidMatrix = []*task.List{sg.DestUIDs}
		}
	}

	if sg.DestUIDs == nil || algo.ListLen(sg.DestUIDs) == 0 {
		// Looks like we're done here. Be careful with nil srcUIDs!
		x.Trace(ctx, "Zero uids for %q. Num attr children: %v", sg.Attr, len(sg.Children))
		rch <- nil
		return
	}

	// Run filters if any.
	if len(sg.Filters) > 0 {
		// Run all filters in parallel.
		filterChan := make(chan error, len(sg.Filters))
		for _, filter := range sg.Filters {
			filter.SrcUIDs = sg.DestUIDs
			filter.Params.ParentVars = sg.Params.ParentVars // Pass to the child.
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
		if sg.FilterOp == "or" {
			sg.DestUIDs = algo.MergeSorted(lists)
		} else if sg.FilterOp == "not" {
			x.AssertTrue(len(sg.Filters) == 1)
			algo.Difference(sg.DestUIDs, sg.Filters[0].DestUIDs)
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

	// If the current node defines a variable, we store it in the map and pass it on
	// to the children later which might depend on it.
	if sg.Params.Var != "" {
		sg.Params.ParentVars[sg.Params.Var] = sg.DestUIDs
	}

	// Here we consider handling count with filtering. We do this after
	// pagination because otherwise, we need to do the count with pagination
	// taken into account. For example, a PL might have only 50 entries but the
	// user wants to skip 100 entries and return 10 entries. In this case, you
	// should return a count of 0, not 10.

	// take care of the order
	if sg.Params.DoCount {
		x.AssertTrue(len(sg.Filters) > 0)
		sg.counts = make([]uint32, len(sg.uidMatrix))
		for i, ul := range sg.uidMatrix {
			// A possible optimization is to return the size of the intersection
			// without forming the intersection.
			algo.IntersectWith(ul, sg.DestUIDs)
			sg.counts[i] = uint32(algo.ListLen(ul))
		}
		rch <- nil
		return
	}

	childChan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.Params.ParentVars = sg.Params.ParentVars // Pass to the child.
		child.SrcUIDs = sg.DestUIDs                    // Make the connection.
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
	for i := 0; i < len(sg.uidMatrix); i++ { //_, l := range sg.uidMatrix {
		algo.IntersectWith(sg.uidMatrix[i], sg.DestUIDs)
		start, end := pageRange(&sg.Params, algo.ListLen(sg.uidMatrix[i]))
		algo.Slice(sg.uidMatrix[i], start, end)
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
	included := make([]bool, algo.ListLen(sg.DestUIDs))
	for _, ul := range sg.uidMatrix {
		it := algo.NewListIterator(ul)
		for ; it.Valid(); it.Next() {
			uid := it.Val()
			idxi, idxj := algo.IndexOf(sg.DestUIDs, uid) // Binary search.
			if idxi >= 0 {
				idx := algo.Idx(sg.DestUIDs, idxi, idxj)
				included[idx] = true
			}
		}
	}
	algo.ApplyFilter(sg.DestUIDs,
		func(uid uint64, idx int) bool { return included[idx] })
	return nil
}

// isValidArg checks if arg passed is valid keyword.
func isValidArg(a string) bool {
	switch a {
	case "from", "to", "orderasc", "orderdesc", "first", "offset", "after":
		return true
	}
	return false
}

// isValidFuncName checks if fn passed is valid keyword.
func isValidFuncName(f string) bool {
	switch f {
	case "anyof", "allof", "id":
		return true
	}
	return isCompareFn(f) || types.IsGeoFunc(f)
}

func isCompareFn(f string) bool {
	switch f {
	case "leq", "geq", "lt", "gt", "eq":
		return true
	}
	return false
}

func isAggregatorFn(f string) bool {
	switch f {
	case "min", "max", "sum":
		return true
	}
	return false
}
