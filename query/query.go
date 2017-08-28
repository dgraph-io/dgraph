/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/trace"

	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
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
	GetUid     bool
	Order      string
	OrderDesc  bool
	Var        string
	NeedsVar   []gql.VarContext
	ParentVars map[string]varValue
	FacetVar   map[string]string
	uidToVal   map[uint64]types.Val
	Langs      []string

	// directives.
	Normalize    bool
	Cascade      bool
	IgnoreReflex bool

	From           uint64
	To             uint64
	Facet          *protos.Param
	FacetOrder     string
	FacetOrderDesc bool
	ExploreDepth   uint64
	isInternal     bool   // Determines if processTask has to be called or not.
	ignoreResult   bool   // Node results are ignored.
	Expand         string // Var to use for expand.
	isGroupBy      bool
	groupbyAttrs   []gql.AttrLang
	uidCount       string
	numPaths       int
	parentIds      []uint64 // This is a stack that is maintained and passed down to children.
	IsEmpty        bool     // Won't have any SrcUids or DestUids. Only used to get aggregated vars
}

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr         string
	Params       params
	counts       []uint32
	valueMatrix  []*protos.ValuesList
	uidMatrix    []*protos.List
	facetsMatrix []*protos.FacetsList
	ExpandPreds  []*protos.ValuesList
	GroupbyRes   *groupResults

	// SrcUIDs is a list of unique source UIDs. They are always copies of destUIDs
	// of parent nodes in GraphQL structure.
	SrcUIDs *protos.List
	SrcFunc []string

	FilterOp     string
	Filters      []*SubGraph
	facetsFilter *protos.FilterTree
	MathExp      *mathTree
	Children     []*SubGraph

	// destUIDs is a list of destination UIDs, after applying filters, pagination.
	DestUIDs *protos.List
}

func (sg *SubGraph) IsGroupBy() bool {
	return sg.Params.isGroupBy
}

func (sg *SubGraph) IsInternal() bool {
	return sg.Params.isInternal
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
	x.Printf("%s[%q Alias:%q Func:%v SrcSz:%v Op:%q DestSz:%v IsCount: %v ValueSz:%v]\n",
		prefix, sg.Attr, sg.Params.Alias, sg.SrcFunc, src, sg.FilterOp,
		dst, sg.Params.DoCount, len(sg.valueMatrix))
	for _, f := range sg.Filters {
		f.DebugPrint(prefix + "|-f->")
	}
	for _, c := range sg.Children {
		c.DebugPrint(prefix + "|->")
	}
}

// getValue gets the value from the task.
func getValue(tv *protos.TaskValue) (types.Val, error) {
	vID := types.TypeID(tv.ValType)
	val := types.ValueForType(vID)
	val.Value = tv.Val
	return val, nil
}

var nodePool = sync.Pool{
	New: func() interface{} {
		return &protos.Node{}
	},
}

var nodeCh chan *protos.Node

func release() {
	for n := range nodeCh {
		// In case of mutations, n is nil
		if n == nil {
			continue
		}
		for i := 0; i < len(n.Children); i++ {
			nodeCh <- n.Children[i]
		}
		*n = protos.Node{}
		nodePool.Put(n)
	}
}

func init() {
	nodeCh = make(chan *protos.Node, 1000)
	go release()
}

var (
	ErrEmptyVal = errors.New("query: harmless error, e.g. task.Val is nil")
	ErrWrongAgg = errors.New("Wrong level for var aggregation.")
)

func (sg *SubGraph) isSimilar(ssg *SubGraph) bool {
	if sg.Attr != ssg.Attr {
		return false
	}
	if len(sg.Params.Langs) != len(ssg.Params.Langs) {
		return false
	}
	for i := 0; i < len(sg.Params.Langs) && i < len(ssg.Params.Langs); i++ {
		if sg.Params.Langs[i] != ssg.Params.Langs[i] {
			return false
		}
	}
	if sg.Params.DoCount {
		if ssg.Params.DoCount {
			return true
		}
		return false
	}
	if ssg.Params.DoCount {
		return false
	}
	if len(sg.SrcFunc) > 0 {
		if len(ssg.SrcFunc) > 0 {
			if sg.SrcFunc[0] == ssg.SrcFunc[0] {
				return true
			}
		}
		return false
	}
	return true
}

func (sg *SubGraph) fieldName() string {
	fieldName := sg.Attr
	if sg.Params.Alias != "" {
		fieldName = sg.Params.Alias
	}
	return fieldName
}

func addCount(pc *SubGraph, count uint64, dst outputNode) {
	c := types.ValueForType(types.IntID)
	c.Value = int64(count)
	fieldName := fmt.Sprintf("count(%s)", pc.Attr)
	if pc.Params.Alias != "" {
		fieldName = pc.Params.Alias
	}
	dst.AddValue(fieldName, c)
}

func aggWithVarFieldName(pc *SubGraph) string {
	fieldName := fmt.Sprintf("val(%v)", pc.Params.Var)
	if len(pc.Params.NeedsVar) > 0 {
		fieldName = fmt.Sprintf("val(%v)", pc.Params.NeedsVar[0].Name)
		if len(pc.SrcFunc) > 0 {
			fieldName = fmt.Sprintf("%s(%v)", pc.SrcFunc[0], fieldName)
		}
	}
	if pc.Params.Alias != "" {
		fieldName = pc.Params.Alias
	}
	return fieldName
}

func addInternalNode(pc *SubGraph, uid uint64, dst outputNode) error {
	if pc.Params.uidToVal == nil {
		return x.Errorf("Wrong use of var() with %v.", pc.Params.NeedsVar)
	}
	fieldName := aggWithVarFieldName(pc)
	sv, ok := pc.Params.uidToVal[uid]
	if !ok || sv.Value == nil {
		return nil
	}
	if sv.Tid == types.StringID && sv.Value.(string) == "_nil_" {
		sv.Value = ""
	}
	dst.AddValue(fieldName, sv)
	return nil
}

func addCheckPwd(pc *SubGraph, val *protos.TaskValue, dst outputNode) {
	c := types.ValueForType(types.BoolID)
	c.Value = task.ToBool(val)
	uc := dst.New(pc.Attr)
	uc.AddValue("checkpwd", c)
	dst.AddListChild(pc.Attr, uc)
}

func alreadySeen(parentIds []uint64, uid uint64) bool {
	for _, id := range parentIds {
		if id == uid {
			return true
		}
	}
	return false
}

// This method gets the values and children for a subprotos.
func (sg *SubGraph) preTraverse(uid uint64, dst outputNode) error {
	if sg.Params.IgnoreReflex {
		if sg.Params.parentIds == nil {
			parentIds := make([]uint64, 0, 10)
			sg.Params.parentIds = parentIds
		}
		if alreadySeen(sg.Params.parentIds, uid) {
			// A node can't have itself as the child at any level.
			return nil
		}
		// Push myself to stack before sending this to children.
		sg.Params.parentIds = append(sg.Params.parentIds, uid)
	}
	if sg.Params.GetUid {
		// If we are asked for count() and there are no other children,
		// then we dont return the uids at this level so that UI doesn't render
		// nodes without any other properties.
		if sg.Params.uidCount == "" || len(sg.Children) != 0 {
			dst.SetUID(uid, "_uid_")
		}
	}

	var invalidUids map[uint64]bool
	var facetsNode outputNode
	// We go through all predicate children of the subprotos.
	for _, pc := range sg.Children {
		if pc.Params.ignoreResult {
			continue
		}
		if pc.Params.isGroupBy {
			dst.addGroupby(pc, pc.Attr)
			continue
		}
		if pc.IsInternal() {
			if pc.Params.Expand != "" {
				continue
			}
			if err := addInternalNode(pc, uid, dst); err != nil {
				return err
			}
			continue
		}

		if pc.uidMatrix == nil {
			// Can happen in recurse query.
			continue
		}

		idx := algo.IndexOf(pc.SrcUIDs, uid)
		if idx < 0 {
			continue
		}
		ul := pc.uidMatrix[idx]

		fieldName := pc.fieldName()
		if len(pc.counts) > 0 {
			addCount(pc, uint64(pc.counts[idx]), dst)
		} else if len(pc.SrcFunc) > 0 && pc.SrcFunc[0] == "checkpwd" {
			addCheckPwd(pc, pc.valueMatrix[idx].Values[0], dst)
		} else if len(ul.Uids) > 0 {
			var fcsList []*protos.Facets
			if pc.Params.Facet != nil {
				fcsList = pc.facetsMatrix[idx].FacetsList
			}

			if sg.Params.IgnoreReflex {
				pc.Params.parentIds = sg.Params.parentIds
			}
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			for childIdx, childUID := range ul.Uids {
				if fieldName == "" || (invalidUids != nil && invalidUids[childUID]) {
					continue
				}
				uc := dst.New(fieldName)
				if rerr := pc.preTraverse(childUID, uc); rerr != nil {
					if rerr.Error() == "_INV_" {
						if invalidUids == nil {
							invalidUids = make(map[uint64]bool)
						}

						invalidUids[childUID] = true
						continue // next UID.
					}
					// Some other error.
					x.Printf("Error while traversal: %v", rerr)
					return rerr
				}

				if pc.Params.Facet != nil && len(fcsList) > childIdx {
					fs := fcsList[childIdx]
					fc := dst.New(fieldName)
					for _, f := range fs.Facets {
						fc.AddValue(f.Key, facets.ValFor(f))
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
			if pc.Params.uidCount != "" {
				uc := dst.New(fieldName)
				c := types.ValueForType(types.IntID)
				c.Value = int64(len(ul.Uids))
				uc.AddValue(pc.Params.uidCount, c)
				dst.AddListChild(fieldName, uc)
			}
		} else {
			if pc.Params.Alias == "" && len(pc.Params.Langs) > 0 {
				fieldName += "@"
				fieldName += strings.Join(pc.Params.Langs, ":")
			}

			if pc.Attr == "_uid_" {
				dst.SetUID(uid, pc.fieldName())
				continue
			}

			tv := pc.valueMatrix[idx]
			if bytes.Equal(tv.Values[0].Val, x.Nilbyte) {
				continue
			}

			if pc.Params.Facet != nil && len(pc.facetsMatrix[idx].FacetsList) > 0 {
				fc := dst.New(fieldName)
				// in case of Value we have only one Facets
				for _, f := range pc.facetsMatrix[idx].FacetsList[0].Facets {
					fc.AddValue(f.Key, facets.ValFor(f))
				}
				if !fc.IsEmpty() {
					if facetsNode == nil {
						facetsNode = dst.New("@facets")
					}
					facetsNode.AddMapChild(fieldName, fc, false)
				}
			}

			for _, tv := range pc.valueMatrix[idx].Values {
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

	if sg.Params.IgnoreReflex {
		// Lets pop the stack.
		sg.Params.parentIds = (sg.Params.parentIds)[:len(sg.Params.parentIds)-1]
	}
	if facetsNode != nil && !facetsNode.IsEmpty() {
		dst.AddMapChild("@facets", facetsNode, false)
	}
	return nil
}

// convert from task.Val to types.Value, based on schema appropriate type
// is already set in protos.Value
func convertWithBestEffort(tv *protos.TaskValue, attr string) (types.Val, error) {
	// value would be in binary format with appropriate type
	v, _ := getValue(tv)
	if !v.Tid.IsScalar() {
		return v, x.Errorf("Leaf predicate:'%v' must be a scalar.", attr)
	}
	if bytes.Equal(tv.Val, nil) {
		return v, ErrEmptyVal
	}
	// creates appropriate type from binary format
	sv, err := types.Convert(v, v.Tid)
	x.Checkf(err, "Error while interpreting appropriate type from binary")
	return sv, nil
}

func createProperty(prop string, v types.Val) *protos.Property {
	pval := toProtoValue(v)
	return &protos.Property{Prop: prop, Value: pval}
}

func isPresent(list []string, str string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func mathCopy(dst *mathTree, src *gql.MathTree) error {
	// Either we'll have an operation specified, or the function specified.
	dst.Const = src.Const
	dst.Fn = src.Fn
	dst.Val = src.Val
	dst.Var = src.Var

	for _, mc := range src.Child {
		child := &mathTree{}
		if err := mathCopy(child, mc); err != nil {
			return err
		}
		dst.Child = append(dst.Child, child)
	}
	return nil
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
		isUidFuncWithoutVar := isUidFnWithoutVar(ft.Func)
		if isUidFuncWithoutVar {
			sg.populate(ft.Func.UID)
		} else {
			sg.SrcFunc = append(sg.SrcFunc, ft.Func.Lang)
			sg.SrcFunc = append(sg.SrcFunc, ft.Func.Args...)
			sg.Params.NeedsVar = append(sg.Params.NeedsVar, ft.Func.NeedsVar...)
		}
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

func uniqueKey(gchild *gql.GraphQuery) string {
	key := gchild.Attr
	if gchild.Func != nil {
		key += fmt.Sprintf("%v", gchild.Func)
	}
	// This is the case when we ask for a variable.
	if gchild.Attr == "val" {
		// E.g. a as age, result is returned as var(a)
		if gchild.Var != "" && gchild.Var != "val" {
			key = fmt.Sprintf("val(%v)", gchild.Var)
		} else if len(gchild.NeedsVar) > 0 {
			// For var(s)
			key = fmt.Sprintf("val(%v)", gchild.NeedsVar[0].Name)
		}

		// Could be min(var(x)) && max(var(x))
		if gchild.Func != nil {
			key += gchild.Func.Name
		}
	}
	if gchild.IsCount { // ignore count subgraphs..
		key += "count"
	}
	if len(gchild.Langs) > 0 {
		key += fmt.Sprintf("%v", gchild.Langs)
	}
	if gchild.MathExp != nil {
		// We would only be here if Alias is empty, so Var would be non
		// empty because MathExp should have atleast one of them.
		key = fmt.Sprintf("val(%+v)", gchild.Var)
	}
	if gchild.IsGroupby {
		key += "groupby"
	}
	return key
}

func treeCopy(ctx context.Context, gq *gql.GraphQuery, sg *SubGraph) error {
	// Typically you act on the current node, and leave recursion to deal with
	// children. But, in this case, we don't want to muck with the current
	// node, because of the way we're dealing with the root node.
	// So, we work on the children, and then recurse for grand children.
	attrsSeen := make(map[string]struct{})

	for _, gchild := range gq.Children {
		if (sg.Params.Alias == "shortest" || sg.Params.Alias == "recurse") &&
			gchild.Expand != "" {
			return x.Errorf("expand() not allowed inside shortest/recurse")
		}

		key := ""
		if gchild.Alias != "" {
			key = gchild.Alias
		} else {
			key = uniqueKey(gchild)
		}
		if _, ok := attrsSeen[key]; ok {
			return x.Errorf("%s not allowed multiple times in same sub-query.",
				key)
		}
		attrsSeen[key] = struct{}{}

		args := params{
			Alias:          gchild.Alias,
			Langs:          gchild.Langs,
			GetUid:         sg.Params.GetUid,
			Var:            gchild.Var,
			Normalize:      sg.Params.Normalize,
			isInternal:     gchild.IsInternal,
			Expand:         gchild.Expand,
			isGroupBy:      gchild.IsGroupby,
			groupbyAttrs:   gchild.GroupbyAttrs,
			FacetVar:       gchild.FacetVar,
			uidCount:       gchild.UidCount,
			Cascade:        sg.Params.Cascade,
			FacetOrder:     gchild.FacetOrder,
			FacetOrderDesc: gchild.FacetDesc,
			IgnoreReflex:   sg.Params.IgnoreReflex,
		}
		if gchild.Facets != nil {
			args.Facet = &protos.Param{gchild.Facets.AllKeys, gchild.Facets.Keys}
		}

		args.NeedsVar = append(args.NeedsVar, gchild.NeedsVar...)
		if gchild.IsCount {
			if len(gchild.Children) != 0 {
				return errors.New("Node with count cannot have child attributes")
			}
			args.DoCount = true
		}

		for argk := range gchild.Args {
			if !isValidArg(argk) {
				return x.Errorf("Invalid argument : %s", argk)
			}
		}
		if err := args.fill(gchild); err != nil {
			return err
		}

		if len(args.Order) != 0 && len(args.FacetOrder) != 0 {
			return x.Errorf("Cannot specify order at both args and facets")
		}

		dst := &SubGraph{
			Attr:   gchild.Attr,
			Params: args,
		}
		if gchild.MathExp != nil {
			mathExp := &mathTree{}
			if err := mathCopy(mathExp, gchild.MathExp); err != nil {
				return err
			}
			dst.MathExp = mathExp
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
			dst.SrcFunc = append(dst.SrcFunc, gchild.Func.Lang)
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
	if v, ok := gq.Args["depth"]; ok && (args.Alias == "recurse" ||
		args.Alias == "shortest") {
		from, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return err
		}
		args.ExploreDepth = from
	}
	if v, ok := gq.Args["numpaths"]; ok && args.Alias == "shortest" {
		numPaths, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return err
		}
		args.numPaths = int(numPaths)
	}
	if v, ok := gq.Args["from"]; ok && args.Alias == "shortest" {
		from, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return err
		}
		args.From = uint64(from)
	}
	if v, ok := gq.Args["to"]; ok && args.Alias == "shortest" {
		to, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return err
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

func isDebug(ctx context.Context) bool {
	var debug bool
	// gRPC client passes information about debug as metadata.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		// md is a map[string][]string
		debug = len(md["debug"]) > 0 && md["debug"][0] == "true"
	}
	// HTTP passes information about debug as query parameter which is attached to context.
	return debug || ctx.Value("debug") == "true"
}

func (sg *SubGraph) populate(uids []uint64) error {
	// Put sorted entries in matrix.
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	sg.uidMatrix = []*protos.List{{uids}}
	// User specified list may not be sorted.
	sg.SrcUIDs = &protos.List{uids}
	return nil
}

// newGraph returns the SubGraph and its task query.
func newGraph(ctx context.Context, gq *gql.GraphQuery) (*SubGraph, error) {
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.

	// For the root, the name to be used in result is stored in Alias, not Attr.
	// The attr at root (if present) would stand for the source functions attr.
	args := params{
		GetUid:       isDebug(ctx),
		Alias:        gq.Alias,
		Langs:        gq.Langs,
		Var:          gq.Var,
		ParentVars:   make(map[string]varValue),
		Normalize:    gq.Normalize,
		Cascade:      gq.Cascade,
		isGroupBy:    gq.IsGroupby,
		groupbyAttrs: gq.GroupbyAttrs,
		uidCount:     gq.UidCount,
		IgnoreReflex: gq.IgnoreReflex,
		IsEmpty:      gq.IsEmpty,
	}
	if gq.Facets != nil {
		args.Facet = &protos.Param{gq.Facets.AllKeys, gq.Facets.Keys}
	}

	for _, it := range gq.NeedsVar {
		args.NeedsVar = append(args.NeedsVar, it)
	}

	for argk := range gq.Args {
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
		// Uid function doesnt have Attr. It just has a list of ids
		if gq.Func.Attr != "uid" {
			sg.Attr = gq.Func.Attr
		}
		if !isValidFuncName(gq.Func.Name) {
			return nil, x.Errorf("Invalid function name : %s", gq.Func.Name)
		}
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Name)
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Lang)
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Args...)
	}

	isUidFuncWithoutVar := gq.Func != nil && isUidFnWithoutVar(gq.Func)
	if isUidFuncWithoutVar && len(gq.UID) > 0 {
		if err := sg.populate(gq.UID); err != nil {
			return nil, err
		}
	}

	sg.valueMatrix = createNilValuesList(1)
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

func createNilValuesList(count int) []*protos.ValuesList {
	out := make([]*protos.ValuesList, count)
	emptyList := &protos.ValuesList{
		Values: []*protos.TaskValue{&protos.TaskValue{Val: x.Nilbyte}},
	}
	for i := 0; i < count; i++ {
		out[i] = emptyList
	}
	return out
}

func toFacetsFilter(gft *gql.FilterTree) (*protos.FilterTree, error) {
	if gft == nil {
		return nil, nil
	}
	if gft.Func != nil && len(gft.Func.NeedsVar) != 0 {
		return nil, x.Errorf("Variables not supported in protos.FilterTree")
	}
	ftree := new(protos.FilterTree)
	ftree.Op = gft.Op
	for _, gftc := range gft.Child {
		ftc, err := toFacetsFilter(gftc)
		if err != nil {
			return nil, err
		}
		ftree.Children = append(ftree.Children, ftc)
	}
	if gft.Func != nil {
		ftree.Func = &protos.Function{
			Key:  gft.Func.Attr,
			Name: gft.Func.Name,
			Args: []string{},
		}
		ftree.Func.Args = append(ftree.Func.Args, gft.Func.Args...)
	}
	return ftree, nil
}

// createTaskQuery generates the query buffer.
func createTaskQuery(sg *SubGraph) *protos.Query {
	attr := sg.Attr
	// Might be safer than just checking first byte due to i18n
	reverse := strings.HasPrefix(attr, "~")
	if reverse {
		attr = strings.TrimPrefix(attr, "~")
	}
	out := &protos.Query{
		Attr:         attr,
		Langs:        sg.Params.Langs,
		Reverse:      reverse,
		SrcFunc:      sg.SrcFunc,
		AfterUid:     sg.Params.AfterUID,
		DoCount:      len(sg.Filters) == 0 && sg.Params.DoCount,
		FacetParam:   sg.Params.Facet,
		FacetsFilter: sg.facetsFilter,
	}
	if sg.SrcUIDs != nil {
		out.UidList = sg.SrcUIDs
	}
	return out
}

type varValue struct {
	Uids *protos.List
	Vals map[uint64]types.Val
	path []*SubGraph // This stores the subgraph path from root to var definition.
	// TODO: Check if we can do without this field.
	strList []*protos.ValuesList
}

func evalLevelAgg(doneVars map[string]varValue, sg, parent *SubGraph) (mp map[uint64]types.Val,
	rerr error) {
	if parent == nil {
		return mp, ErrWrongAgg
	}

	needsVar := sg.Params.NeedsVar[0].Name
	if parent.Params.IsEmpty {
		// The aggregated value doesn't really belong to a uid, we put it in uidToVal map
		// corresponding to uid 0 to avoid defining another field in SubGraph.
		vals := doneVars[needsVar].Vals
		mp = make(map[uint64]types.Val)
		if len(vals) == 0 {
			mp[0] = types.Val{Tid: types.FloatID, Value: 0.0}
			return mp, nil
		}

		ag := aggregator{
			name: sg.SrcFunc[0],
		}
		for _, val := range vals {
			ag.Apply(val)
		}
		v, err := ag.Value()
		if err != nil && err != ErrEmptyVal {
			return mp, err
		}
		if v.Value != nil {
			mp[0] = v
		}
		return mp, nil
	}

	var relSG *SubGraph
	for _, ch := range parent.Children {
		if sg == ch {
			continue
		}
		if ch.Params.FacetVar != nil {
			for _, v := range ch.Params.FacetVar {
				if v == needsVar {
					relSG = ch
				}
			}
		}
		for _, cch := range ch.Children {
			// Find the sibling node whose child has the required variable.
			if cch.Params.Var == needsVar {
				relSG = ch
			}
		}
	}
	if relSG == nil {
		return mp, x.Errorf("Invalid variable aggregation. Check the levels.")
	}

	vals := doneVars[needsVar].Vals
	mp = make(map[uint64]types.Val)
	// Go over the sibling node and aggregate.
	for i, list := range relSG.uidMatrix {
		ag := aggregator{
			name: sg.SrcFunc[0],
		}
		for _, uid := range list.Uids {
			if val, ok := vals[uid]; ok {
				ag.Apply(val)
			}
		}
		v, err := ag.Value()
		if err != nil && err != ErrEmptyVal {
			return mp, err
		}
		if v.Value != nil {
			mp[relSG.SrcUIDs.Uids[i]] = v
		}
	}
	return mp, nil
}

func (mt *mathTree) extractVarNodes() []*mathTree {
	var nodeList []*mathTree
	for _, ch := range mt.Child {
		nodeList = append(nodeList, ch.extractVarNodes()...)
	}
	if mt.Var != "" {
		nodeList = append(nodeList, mt)
		return nodeList
	}
	return nodeList
}

// transformTo transforms fromNode to toNode level using the path between them and the
// corresponding uidMatrices.
func (fromNode *varValue) transformTo(toPath []*SubGraph) (map[uint64]types.Val, error) {
	if len(toPath) < len(fromNode.path) {
		return fromNode.Vals, nil
	}

	idx := 0
	for ; idx < len(fromNode.path); idx++ {
		if fromNode.path[idx] != toPath[idx] {
			return fromNode.Vals, nil
		}
	}

	newMap := fromNode.Vals
	if newMap == nil {
		return map[uint64]types.Val{}, nil
	}
	for ; idx < len(toPath); idx++ {
		curNode := toPath[idx]
		tempMap := make(map[uint64]types.Val)
		if idx == 0 {
			continue
		}

		for i := 0; i < len(curNode.uidMatrix); i++ {
			ul := curNode.uidMatrix[i]
			srcUid := curNode.SrcUIDs.Uids[i]
			curVal, ok := newMap[srcUid]
			if !ok || curVal.Value == nil {
				continue
			}
			if curVal.Tid != types.IntID && curVal.Tid != types.FloatID {
				return nil, x.Errorf("Encountered non int/float type for summing")
			}
			for j := 0; j < len(ul.Uids); j++ {
				dstUid := ul.Uids[j]
				ag := aggregator{name: "sum"}
				ag.Apply(curVal)
				ag.Apply(tempMap[dstUid])
				val, err := ag.Value()
				if err != nil {
					continue
				}
				tempMap[dstUid] = val
			}
		}
		newMap = tempMap
	}
	return newMap, nil
}

// transformVars transforms all the variables to the variable at the lowest level
func (sg *SubGraph) transformVars(doneVars map[string]varValue,
	path []*SubGraph) error {
	mNode := sg.MathExp
	mvarList := mNode.extractVarNodes()
	for i := 0; i < len(mvarList); i++ {
		mt := mvarList[i]
		curNode := doneVars[mt.Var]
		newMap, err := curNode.transformTo(path)
		if err != nil {
			return err
		}
		mt.Val = newMap
	}
	return nil
}

func (sg *SubGraph) valueVarAggregation(doneVars map[string]varValue, path []*SubGraph,
	parent *SubGraph) error {
	if !sg.IsInternal() && !sg.IsGroupBy() && !sg.Params.IsEmpty {
		return nil
	}

	// Aggregation function won't be present at root.
	if sg.Params.IsEmpty && parent == nil {
		return nil
	}

	if sg.IsGroupBy() {
		err := sg.processGroupBy(doneVars, path)
		if err != nil {
			return err
		}
	} else if len(sg.SrcFunc) > 0 && !parent.IsGroupBy() && isAggregatorFn(sg.SrcFunc[0]) {
		// Aggregate the value over level.
		mp, err := evalLevelAgg(doneVars, sg, parent)
		if err != nil {
			return err
		}
		if sg.Params.Var != "" {
			it := doneVars[sg.Params.Var]
			it.Vals = mp
			doneVars[sg.Params.Var] = it
		}
		sg.Params.uidToVal = mp
	} else if sg.MathExp != nil {
		// Preprocess to bring all variables to the same level.
		err := sg.transformVars(doneVars, path)
		if err != nil {
			return err
		}

		err = evalMathTree(sg.MathExp)
		if err != nil {
			return err
		}
		if sg.MathExp.Val != nil {
			it := doneVars[sg.Params.Var]
			var isInt, isFloat bool
			for _, v := range sg.MathExp.Val {
				if v.Tid == types.FloatID {
					isFloat = true
				}
				if v.Tid == types.IntID {
					isInt = true
				}
			}
			if isInt && isFloat {
				for k, v := range sg.MathExp.Val {
					if v.Tid == types.IntID {
						v.Tid = types.FloatID
						v.Value = float64(v.Value.(int64))
					}
					sg.MathExp.Val[k] = v
				}
			}

			it.Vals = sg.MathExp.Val
			// The path of math node is the path of max var node used in it.
			it.path = path
			doneVars[sg.Params.Var] = it
			sg.Params.uidToVal = sg.MathExp.Val
		} else if sg.MathExp.Const.Value != nil {
			// Assign the const for all the srcUids.
			mp := make(map[uint64]types.Val)
			rangeOver := sg.SrcUIDs
			if parent == nil {
				rangeOver = sg.DestUIDs
			}
			if rangeOver == nil {
				it := doneVars[sg.Params.Var]
				it.Vals = mp
				doneVars[sg.Params.Var] = it
				return nil
			}
			for _, uid := range rangeOver.Uids {
				mp[uid] = sg.MathExp.Const
			}
			it := doneVars[sg.Params.Var]
			it.Vals = mp
			doneVars[sg.Params.Var] = it
			sg.Params.uidToVal = mp
		} else {
			return x.Errorf("Missing values/constant in math expression")
		}
		// Put it in this node.
	} else if len(sg.Params.NeedsVar) > 0 {
		// This is a var() block.
		srcVar := sg.Params.NeedsVar[0]
		srcMap := doneVars[srcVar.Name]
		// The value var can be empty. No need to check for nil.
		sg.Params.uidToVal = srcMap.Vals
	} else {
		return x.Errorf("Unhandled internal node %v with parent %v", sg.Attr, parent.Attr)
	}
	return nil
}

func (sg *SubGraph) populatePostAggregation(doneVars map[string]varValue, path []*SubGraph,
	parent *SubGraph) error {
	for idx := 0; idx < len(sg.Children); idx++ {
		child := sg.Children[idx]
		path = append(path, sg)
		err := child.populatePostAggregation(doneVars, path, sg)
		path = path[:len(path)-1]
		if err != nil {
			return err
		}
	}
	return sg.valueVarAggregation(doneVars, path, parent)
}

func (sg *SubGraph) updateUidMatrix() {
	for _, l := range sg.uidMatrix {
		if sg.Params.Order != "" {
			// We can't do intersection directly as the list is not sorted by UIDs.
			// So do filter.
			algo.ApplyFilter(l, func(uid uint64, idx int) bool {
				i := algo.IndexOf(sg.DestUIDs, uid) // Binary search.
				if i >= 0 {
					return true
				}
				return false
			})
		} else {
			// If we didn't order on UIDmatrix, it'll be sorted.
			algo.IntersectWith(l, sg.DestUIDs, l)
		}
	}

}

func (sg *SubGraph) populateVarMap(doneVars map[string]varValue,
	sgPath []*SubGraph) error {
	if sg.DestUIDs == nil || sg.IsGroupBy() {
		return nil
	}
	out := make([]uint64, 0, len(sg.DestUIDs.Uids))
	if sg.Params.Alias == "shortest" {
		goto AssignStep
	}

	if len(sg.Filters) > 0 {
		sg.updateUidMatrix()
	}
	for _, child := range sg.Children {
		sgPath = append(sgPath, sg) // Add the current node to path
		child.populateVarMap(doneVars, sgPath)
		sgPath = sgPath[:len(sgPath)-1] // Backtrack
		if !sg.Params.Cascade {
			continue
		}

		// Intersect the UidMatrix with the DestUids as some UIDs might have been removed
		// by other operations. So we need to apply it on the UidMatrix.
		child.updateUidMatrix()
	}

	if !sg.Params.Cascade {
		goto AssignStep
	}

	// Filter out UIDs that don't have atleast one UID in every child.
	for i, uid := range sg.DestUIDs.Uids {
		var exclude bool
		for _, child := range sg.Children {
			// For _uid_ we dont actually populate the uidMatrix or values. So a node asking for
			// _uid_ would always be excluded. Therefore we skip it.
			if child.Attr == "_uid_" {
				continue
			}

			// If the length of child UID list is zero and it has no valid value, then the
			// current UID should be removed from this level.
			if !child.IsInternal() &&
				// Check len before accessing index.
				(len(child.valueMatrix) <= i || len(child.valueMatrix[i].Values[0].Val) == 0) &&
				(len(child.counts) <= i) &&
				(len(child.uidMatrix) <= i || len(child.uidMatrix[i].Uids) == 0) {
				exclude = true
				break
			}
		}
		if !exclude {
			out = append(out, uid)
		}
	}
	// Note the we can't overwrite DestUids, as it'd also modify the SrcUids of
	// next level and the mapping from SrcUids to uidMatrix would be lost.
	sg.DestUIDs = &protos.List{out}

AssignStep:
	return sg.assignVars(doneVars, sgPath)
}

func (sg *SubGraph) assignVars(doneVars map[string]varValue, sgPath []*SubGraph) error {
	if doneVars == nil || (sg.Params.Var == "" && sg.Params.FacetVar == nil) {
		return nil
	}

	sgPathCopy := make([]*SubGraph, len(sgPath))
	copy(sgPathCopy, sgPath)
	err := sg.populateUidValVar(doneVars, sgPathCopy)
	if err != nil {
		return err
	}
	return sg.populateFacetVars(doneVars, sgPathCopy)
}

func (sg *SubGraph) populateUidValVar(doneVars map[string]varValue, sgPath []*SubGraph) error {
	if sg.Params.Var == "" {
		return nil
	}

	if sg.Attr == "_predicate_" {
		// This is a predicates list.
		doneVars[sg.Params.Var] = varValue{
			strList: sg.valueMatrix,
			path:    sgPath,
		}
	} else if len(sg.counts) > 0 {
		// This implies it is a value variable.
		doneVars[sg.Params.Var] = varValue{
			Vals: make(map[uint64]types.Val),
			path: sgPath,
		}
		for idx, uid := range sg.SrcUIDs.Uids {
			val := types.Val{
				Tid:   types.IntID,
				Value: int64(sg.counts[idx]),
			}
			doneVars[sg.Params.Var].Vals[uid] = val
		}
	} else if len(sg.DestUIDs.Uids) != 0 {
		// This implies it is a entity variable.
		doneVars[sg.Params.Var] = varValue{
			Uids: sg.DestUIDs,
			path: sgPath,
		}
	} else if len(sg.valueMatrix) != 0 && sg.SrcUIDs != nil && len(sgPath) != 0 {
		// This implies it is a value variable.
		// NOTE: Value variables cannot be defined and used in the same query block. so
		// checking len(sgPath) is okay.
		doneVars[sg.Params.Var] = varValue{
			Vals: make(map[uint64]types.Val),
			path: sgPath,
		}
		for idx, uid := range sg.SrcUIDs.Uids {
			val, err := convertWithBestEffort(sg.valueMatrix[idx].Values[0], sg.Attr)
			if err != nil {
				continue
			}
			doneVars[sg.Params.Var].Vals[uid] = val
		}
	} else {
		// Insert a empty entry to keep the dependency happy.
		doneVars[sg.Params.Var] = varValue{
			path: sgPath,
			Vals: make(map[uint64]types.Val),
		}
	}
	return nil
}
func (sg *SubGraph) populateFacetVars(doneVars map[string]varValue, sgPath []*SubGraph) error {
	if sg.Params.FacetVar != nil && sg.Params.Facet != nil {
		sgPath = append(sgPath, sg)

		for _, it := range sg.Params.Facet.Keys {
			fvar, ok := sg.Params.FacetVar[it]
			if !ok {
				continue
			}
			doneVars[fvar] = varValue{
				Vals: make(map[uint64]types.Val),
				path: sgPath,
			}
		}

		if len(sg.facetsMatrix) == 0 {
			return nil
		}

		// Note: We ignore the facets if its a value edge as we can't
		// attach the value to any node.
		for i, uids := range sg.uidMatrix {
			for j, uid := range uids.Uids {
				facet := sg.facetsMatrix[i].FacetsList[j]
				for _, f := range facet.Facets {
					fvar, ok := sg.Params.FacetVar[f.Key]
					if ok {
						if pVal, ok := doneVars[fvar].Vals[uid]; !ok {
							doneVars[fvar].Vals[uid] = facets.ValFor(f)
						} else {
							// If the value is int/float we add them up. Else we throw an error as
							// many to one maps are not allowed for other types.
							nVal := facets.ValFor(f)
							if nVal.Tid != types.IntID && nVal.Tid != types.FloatID {
								return x.Errorf("Repeated id with non int/float value for facet var encountered.")
							}
							ag := aggregator{name: "sum"}
							ag.Apply(pVal)
							ag.Apply(nVal)
							fVal, err := ag.Value()
							if err != nil {
								continue
							}
							doneVars[fvar].Vals[uid] = fVal
						}
					}
				}
			}
		}
	}
	return nil
}

func (sg *SubGraph) recursiveFillVars(doneVars map[string]varValue) error {
	err := sg.fillVars(doneVars)
	if err != nil {
		return err
	}
	for _, child := range sg.Children {
		err = child.recursiveFillVars(doneVars)
		if err != nil {
			return err
		}
	}
	for _, fchild := range sg.Filters {
		err = fchild.recursiveFillVars(doneVars)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sg *SubGraph) fillVars(mp map[string]varValue) error {
	lists := make([]*protos.List, 0, 3)
	for _, v := range sg.Params.NeedsVar {
		if l, ok := mp[v.Name]; ok {
			if (v.Typ == gql.ANY_VAR || v.Typ == gql.LIST_VAR) && l.strList != nil {
				sg.ExpandPreds = l.strList
			} else if (v.Typ == gql.ANY_VAR || v.Typ == gql.UID_VAR) && l.Uids != nil {
				lists = append(lists, l.Uids)
			} else if (v.Typ == gql.ANY_VAR || v.Typ == gql.VALUE_VAR) && len(l.Vals) != 0 {
				// This should happen only once.
				sg.Params.uidToVal = l.Vals
			} else if len(l.Vals) != 0 && (v.Typ == gql.ANY_VAR || v.Typ == gql.UID_VAR) {
				// Derive the UID list from value var.
				uids := make([]uint64, 0, len(l.Vals))
				for k := range l.Vals {
					uids = append(uids, k)
				}
				sort.Slice(uids, func(i, j int) bool {
					return uids[i] < uids[j]
				})
				lists = append(lists, &protos.List{uids})
			} else if len(l.Vals) != 0 || l.Uids != nil {
				return x.Errorf("Wrong variable type encountered for var(%v) %v.", v.Name, v.Typ)
			}
		}
	}
	lists = append(lists, sg.DestUIDs)
	sg.DestUIDs = algo.MergeSorted(lists)
	return nil
}

func (sg *SubGraph) ApplyIneqFunc() error {
	if sg.Params.uidToVal == nil {
		return x.Errorf("Expected a valid value map. But got empty.")
	}
	var typ types.TypeID
	for _, v := range sg.Params.uidToVal {
		typ = v.Tid
		break
	}
	val := sg.SrcFunc[3]
	src := types.Val{types.StringID, []byte(val)}
	dst, err := types.Convert(src, typ)
	if err != nil {
		return x.Errorf("Invalid argment %v. Comparing with different type", val)
	}
	if sg.SrcUIDs != nil {
		for _, uid := range sg.SrcUIDs.Uids {
			curVal, ok := sg.Params.uidToVal[uid]
			if ok && types.CompareVals(sg.SrcFunc[0], curVal, dst) {
				sg.DestUIDs.Uids = append(sg.DestUIDs.Uids, uid)
			}
		}
	} else {
		// This means its a root as SrcUIDs is nil
		for uid, curVal := range sg.Params.uidToVal {
			if types.CompareVals(sg.SrcFunc[0], curVal, dst) {
				sg.DestUIDs.Uids = append(sg.DestUIDs.Uids, uid)
			}
		}
		sort.Slice(sg.DestUIDs.Uids, func(i, j int) bool {
			return sg.DestUIDs.Uids[i] < sg.DestUIDs.Uids[j]
		})
		sg.uidMatrix = []*protos.List{sg.DestUIDs}
	}
	return nil
}

func (sg *SubGraph) appendDummyValues() {
	var l protos.List
	var val protos.ValuesList
	for i := 0; i < len(sg.SrcUIDs.Uids); i++ {
		// This is necessary so that preTraverse can be processed smoothly.
		sg.uidMatrix = append(sg.uidMatrix, &l)
		sg.valueMatrix = append(sg.valueMatrix, &val)
	}
}

func uniquePreds(vl []*protos.ValuesList) map[string]struct{} {
	preds := make(map[string]struct{})

	for _, l := range vl {
		for _, v := range l.Values {
			preds[string(v.Val)] = struct{}{}
		}
	}
	return preds
}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances. Note: taskQuery is nil for root node.
func ProcessGraph(ctx context.Context, sg, parent *SubGraph, rch chan error) {
	if sg.Attr == "_uid_" {
		// We dont need to call ProcessGraph for _uid_, as we already have uids
		// populated from parent and there is nothing to process but uidMatrix
		// and values need to have the right sizes so that preTraverse works.
		sg.appendDummyValues()
		rch <- nil
		return
	}
	var err error
	if parent == nil && len(sg.SrcFunc) > 0 && sg.SrcFunc[0] == "uid" {
		// I'm root and I'm using some variable that has been populated.
		// Retain the actual order in uidMatrix. But sort the destUids.
		if sg.SrcUIDs != nil && len(sg.SrcUIDs.Uids) != 0 {
			// I am root. I don't have any function to execute, and my
			// result has been prepared for me already by list passed by the user.
			// uidmatrix retains the order. SrcUids are sorted (in newGraph).
			sg.DestUIDs = sg.SrcUIDs
		} else {
			// Populated variable.
			o := make([]uint64, len(sg.DestUIDs.Uids))
			copy(o, sg.DestUIDs.Uids)
			sg.uidMatrix = []*protos.List{{o}}
			sort.Slice(sg.DestUIDs.Uids, func(i, j int) bool { return sg.DestUIDs.Uids[i] < sg.DestUIDs.Uids[j] })
		}
	} else if len(sg.Attr) == 0 {
		// This is when we have uid function in children.
		if len(sg.SrcFunc) > 0 && sg.SrcFunc[0] == "uid" {
			// If its a uid() filter, we just have to intersect the SrcUIDs with DestUIDs
			// and return.
			sg.fillVars(sg.Params.ParentVars)
			algo.IntersectWith(sg.DestUIDs, sg.SrcUIDs, sg.DestUIDs)
			rch <- nil
			return
		}

		x.AssertTruef(sg.SrcUIDs != nil, "SrcUIDs shouldn't be nil.")
		// If we have a filter SubGraph which only contains an operator,
		// it won't have any attribute to work on.
		// This is to allow providing SrcUIDs to the filter children.
		// Each filter use it's own (shallow) copy of SrcUIDs, so there is no race conditions,
		// when multiple filters replace their sg.DestUIDs
		sg.DestUIDs = &protos.List{sg.SrcUIDs.Uids}
	} else {

		if len(sg.SrcFunc) > 0 && isInequalityFn(sg.SrcFunc[0]) && sg.Attr == "val" {
			// This is a ineq function which uses a value variable.
			err = sg.ApplyIneqFunc()
			if parent != nil {
				rch <- err
				return
			}
		} else {
			taskQuery := createTaskQuery(sg)
			result, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
			if err != nil {
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Error while processing task: %+v", err)
				}
				rch <- err
				return
			}

			sg.uidMatrix = result.UidMatrix
			sg.valueMatrix = result.ValueMatrix
			sg.facetsMatrix = result.FacetMatrix
			sg.counts = result.Counts

			if sg.Params.DoCount {
				if len(sg.Filters) == 0 {
					// If there is a filter, we need to do more work to get the actual count.
					if tr, ok := trace.FromContext(ctx); ok {
						tr.LazyPrintf("Zero uids. Only count requested")
					}
					rch <- nil
					return
				}
				sg.counts = make([]uint32, len(sg.uidMatrix))
			}

			if result.IntersectDest {
				sg.DestUIDs = algo.IntersectSorted(result.UidMatrix)
			} else {
				sg.DestUIDs = algo.MergeSorted(result.UidMatrix)
			}

			if parent == nil {
				// I'm root. We reach here if root had a function.
				sg.uidMatrix = []*protos.List{sg.DestUIDs}
			}
		}
	}

	if sg.DestUIDs == nil || len(sg.DestUIDs.Uids) == 0 {
		// Looks like we're done here. Be careful with nil srcUIDs!
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Zero uids for %q. Num attr children: %v", sg.Attr, len(sg.Children))
		}
		out := sg.Children[:0]
		for _, child := range sg.Children {
			if child.IsInternal() && child.Attr == "expand" {
				continue
			}
			out = append(out, child)
		}
		sg.Children = out // Remove any expand nodes we might have added.
		rch <- nil
		return
	}

	// Run filters if any.
	if len(sg.Filters) > 0 {
		// Run all filters in parallel.
		filterChan := make(chan error, len(sg.Filters))
		for _, filter := range sg.Filters {
			isUidFuncWithoutVar := len(filter.SrcFunc) > 0 && filter.SrcFunc[0] == "uid" &&
				len(filter.Params.NeedsVar) == 0
			// For uid function filter, no need for processing. User already gave us the
			// list. Lets just update DestUIDs.
			if isUidFuncWithoutVar {
				filter.DestUIDs = filter.SrcUIDs
				filterChan <- nil
				continue
			}

			filter.SrcUIDs = sg.DestUIDs
			// Passing the pointer is okay since the filter only reads.
			filter.Params.ParentVars = sg.Params.ParentVars // Pass to the child.
			go ProcessGraph(ctx, filter, sg, filterChan)
		}

		var filterErr error
		for range sg.Filters {
			if err = <-filterChan; err != nil {
				// Store error in a variable and wait for all filters to run
				// before returning. Else tracing causes crashes.
				filterErr = err
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Error while processing filter task: %+v", err)
				}
			}
		}

		if filterErr != nil {
			rch <- filterErr
			return
		}

		// Now apply the results from filter.
		var lists []*protos.List
		for _, filter := range sg.Filters {
			lists = append(lists, filter.DestUIDs)
		}
		if sg.FilterOp == "or" {
			sg.DestUIDs = algo.MergeSorted(lists)
		} else if sg.FilterOp == "not" {
			x.AssertTrue(len(sg.Filters) == 1)
			sg.DestUIDs = algo.Difference(sg.DestUIDs, sg.Filters[0].DestUIDs)
		} else {
			sg.DestUIDs = algo.IntersectSorted(lists)
		}
	}

	if len(sg.Params.Order) == 0 && len(sg.Params.FacetOrder) == 0 {
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

	// We store any variable defined by this node in the map and pass it on
	// to the children which might depend on it.
	sg.assignVars(sg.Params.ParentVars, []*SubGraph{})

	// Here we consider handling count with filtering. We do this after
	// pagination because otherwise, we need to do the count with pagination
	// taken into account. For example, a PL might have only 50 entries but the
	// user wants to skip 100 entries and return 10 entries. In this case, you
	// should return a count of 0, not 10.
	// take care of the order
	if sg.Params.DoCount {
		x.AssertTrue(len(sg.Filters) > 0)
		sg.counts = make([]uint32, len(sg.uidMatrix))
		sg.updateUidMatrix()
		for i, ul := range sg.uidMatrix {
			// A possible optimization is to return the size of the intersection
			// without forming the intersection.
			sg.counts[i] = uint32(len(ul.Uids))
		}
		rch <- nil
		return
	}

	var out []*SubGraph
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]

		if child.Params.Expand != "" {
			if child.Params.Expand == "_all_" {
				// Get the predicate list for expansion. Otherwise we already
				// have the list populated.
				child.ExpandPreds, err = GetNodePredicates(ctx, sg.DestUIDs)
				if err != nil {
					rch <- err
					return
				}
			}

			up := uniquePreds(child.ExpandPreds)
			for k, _ := range up {
				temp := new(SubGraph)
				*temp = *child
				temp.Params.isInternal = false
				temp.Params.Expand = ""
				temp.Attr = k
				for _, ch := range sg.Children {
					if ch.isSimilar(temp) {
						rch <- x.Errorf("Repeated subgraph while using expand()")
						return
					}
				}
				out = append(out, temp)
			}
		} else {
			out = append(out, child)
			continue
		}
	}
	sg.Children = out

	if sg.IsGroupBy() {
		// Add the attrs required by groupby nodes
		for _, it := range sg.Params.groupbyAttrs {
			sg.Children = append(sg.Children, &SubGraph{
				Attr: it.Attr,
				Params: params{
					ignoreResult: true,
					Langs:        it.Langs,
				},
			})
		}
	}

	childChan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.Params.ParentVars = make(map[string]varValue)
		for k, v := range sg.Params.ParentVars {
			child.Params.ParentVars[k] = v
		}

		child.SrcUIDs = sg.DestUIDs // Make the connection.
		if child.IsInternal() {
			// We dont have to execute these nodes.
			continue
		}
		go ProcessGraph(ctx, child, sg, childChan)
	}

	var childErr error
	// Now get all the results back.
	for _, child := range sg.Children {
		if child.IsInternal() {
			continue
		}
		if err = <-childChan; err != nil {
			childErr = err
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while processing child task: %+v", err)
			}
		}
	}
	rch <- childErr
}

// applyWindow applies windowing to sg.sorted.
func (sg *SubGraph) applyPagination(ctx context.Context) error {
	params := sg.Params

	if params.Count == 0 && params.Offset == 0 { // No pagination.
		return nil
	}

	sg.updateUidMatrix()
	for i := 0; i < len(sg.uidMatrix); i++ {
		// Apply the offsets.
		start, end := x.PageRange(sg.Params.Count, sg.Params.Offset, len(sg.uidMatrix[i].Uids))
		sg.uidMatrix[i].Uids = sg.uidMatrix[i].Uids[start:end]
	}
	// Re-merge the UID matrix.
	sg.DestUIDs = algo.MergeSorted(sg.uidMatrix)
	return nil
}

// applyOrderAndPagination orders each posting list by a given attribute
// before applying pagination.
func (sg *SubGraph) applyOrderAndPagination(ctx context.Context) error {
	if len(sg.Params.Order) == 0 && len(sg.Params.FacetOrder) == 0 {
		return nil
	}

	sg.updateUidMatrix()

	// See if we need to apply order based on facet.
	if len(sg.Params.FacetOrder) != 0 {
		return sg.sortAndPaginateUsingFacet(ctx)
	}
	for _, it := range sg.Params.NeedsVar {
		if it.Name == sg.Params.Order && (it.Typ == gql.VALUE_VAR) {
			// If the Order name is same as var name and it's a value variable, we sort using that variable.
			return sg.sortAndPaginateUsingVar(ctx)
		}
	}

	if sg.Params.Count == 0 {
		// Only retrieve up to 1000 results by default.
		sg.Params.Count = 1000
	}
	sort := &protos.SortMessage{
		Attr:      sg.Params.Order,
		Langs:     sg.Params.Langs,
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
	sg.uidMatrix = result.UidMatrix

	// Update the destUids as we might have removed some UIDs.
	sg.updateDestUids(ctx)
	return nil
}

func (sg *SubGraph) updateDestUids(ctx context.Context) {
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
}

func (sg *SubGraph) sortAndPaginateUsingFacet(ctx context.Context) error {
	if sg.facetsMatrix == nil {
		return nil
	}
	orderby := sg.Params.FacetOrder
	for i := 0; i < len(sg.uidMatrix); i++ {
		ul := sg.uidMatrix[i]
		fl := sg.facetsMatrix[i]
		uids := ul.Uids[:0]
		values := make([]types.Val, 0, len(ul.Uids))
		facetList := fl.FacetsList[:0]
		for j := 0; j < len(ul.Uids); j++ {
			uid := ul.Uids[j]
			f := fl.FacetsList[j]
			for _, it := range f.Facets {
				if it.Key == orderby {
					values = append(values, facets.ValFor(it))
					uids = append(uids, uid)
					facetList = append(facetList, f)
					break
				}
			}
		}
		if len(values) == 0 {
			continue
		}
		types.SortWithFacet(values, &protos.List{uids}, facetList, sg.Params.FacetOrderDesc)
		sg.uidMatrix[i].Uids = uids
		// We need to update the facetmarix corresponding to changes to uidmatrix.
		sg.facetsMatrix[i].FacetsList = facetList
	}

	if sg.Params.Count != 0 || sg.Params.Offset != 0 {
		// Apply the pagination.
		for i := 0; i < len(sg.uidMatrix); i++ {
			start, end := x.PageRange(sg.Params.Count, sg.Params.Offset, len(sg.uidMatrix[i].Uids))
			sg.uidMatrix[i].Uids = sg.uidMatrix[i].Uids[start:end]
			// We also have to paginate the facetsMatrix for safety.
			sg.facetsMatrix[i].FacetsList = sg.facetsMatrix[i].FacetsList[start:end]
		}
	}

	// Update the destUids as we might have removed some UIDs.
	sg.updateDestUids(ctx)
	return nil
}

func (sg *SubGraph) sortAndPaginateUsingVar(ctx context.Context) error {
	if sg.Params.uidToVal == nil {
		return x.Errorf("Variable: [%s] used before definition.", sg.Params.Order)
	}
	for i := 0; i < len(sg.uidMatrix); i++ {
		ul := sg.uidMatrix[i]
		uids := make([]uint64, 0, len(ul.Uids))
		values := make([]types.Val, 0, len(ul.Uids))
		for _, uid := range ul.Uids {
			v, ok := sg.Params.uidToVal[uid]
			if !ok {
				// We skip the UIDs which don't have a value.
				continue
			}
			values = append(values, v)
			uids = append(uids, uid)
		}
		if len(values) == 0 {
			continue
		}
		types.Sort(values, &protos.List{uids}, sg.Params.OrderDesc)
		sg.uidMatrix[i].Uids = uids
	}

	if sg.Params.Count != 0 || sg.Params.Offset != 0 {
		// Apply the pagination.
		for i := 0; i < len(sg.uidMatrix); i++ {
			start, end := x.PageRange(sg.Params.Count, sg.Params.Offset, len(sg.uidMatrix[i].Uids))
			sg.uidMatrix[i].Uids = sg.uidMatrix[i].Uids[start:end]
		}
	}

	// Update the destUids as we might have removed some UIDs.
	sg.updateDestUids(ctx)
	return nil
}

// isValidArg checks if arg passed is valid keyword.
func isValidArg(a string) bool {
	switch a {
	case "numpaths", "from", "to", "orderasc", "orderdesc", "first", "offset", "after", "depth":
		return true
	}
	return false
}

// isValidFuncName checks if fn passed is valid keyword.
func isValidFuncName(f string) bool {
	switch f {
	case "anyofterms", "allofterms", "val", "regexp", "anyoftext", "alloftext",
		"has", "uid", "uid_in":
		return true
	}
	return isCompareFn(f) || types.IsGeoFunc(f)
}

func isCompareFn(f string) bool {
	switch f {
	case "le", "ge", "lt", "gt", "eq":
		return true
	}
	return false
}

func isInequalityFn(f string) bool {
	switch f {
	case "eq", "le", "ge", "gt", "lt":
		return true
	}
	return false
}

func isAggregatorFn(f string) bool {
	switch f {
	case "min", "max", "sum", "avg":
		return true
	}
	return false
}

func isUidFnWithoutVar(f *gql.Function) bool {
	return f.Name == "uid" && len(f.NeedsVar) == 0
}

func GetNodePredicates(ctx context.Context, uids *protos.List) ([]*protos.ValuesList, error) {
	temp := new(SubGraph)
	temp.Attr = "_predicate_"
	temp.SrcUIDs = uids
	taskQuery := createTaskQuery(temp)
	result, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
	if err != nil {
		return nil, err
	}
	return result.ValueMatrix, nil
}

func GetAllPredicates(subGraphs []*SubGraph) (predicates []string) {
	predicatesMap := make(map[string]bool)
	for _, sg := range subGraphs {
		sg.getAllPredicates(predicatesMap)
	}
	predicates = make([]string, 0, len(predicatesMap))
	for predicate := range predicatesMap {
		predicates = append(predicates, predicate)
	}
	return predicates
}

func (sg *SubGraph) getAllPredicates(predicates map[string]bool) {
	if len(sg.Attr) != 0 {
		predicates[sg.Attr] = true
	}
	if len(sg.Params.Order) != 0 {
		predicates[sg.Params.Order] = true
	}
	if len(sg.Params.groupbyAttrs) != 0 {
		for _, pred := range sg.Params.groupbyAttrs {
			predicates[pred.Attr] = true
		}
	}

	for _, filter := range sg.Filters {
		filter.getAllPredicates(predicates)
	}
	for _, child := range sg.Children {
		child.getAllPredicates(predicates)
	}
}

// convert the new UIDs to hex string.
func ConvertUidsToHex(m map[string]uint64) (res map[string]string) {
	res = make(map[string]string)
	for k, v := range m {
		res[k] = fmt.Sprintf("%#x", v)
	}
	return
}

func parseFacets(nquads []*protos.NQuad) error {
	var err error
	for _, nq := range nquads {
		if len(nq.Facets) == 0 {
			continue
		}
		for idx, f := range nq.Facets {
			if len(f.Value) == 0 {
				// Only do this for client which sends the facet as a string in f.Val
				if f, err = facets.FacetFor(f.Key, f.Val); err != nil {
					return err
				}
			}
			nq.Facets[idx] = f
		}

	}
	return nil
}

// Go client sends facets as string k-v pairs. So they need to parsed and tokenized
// on the server.
func parseFacetsInMutation(mu *gql.Mutation) error {
	if err := parseFacets(mu.Set); err != nil {
		return err
	}
	if err := parseFacets(mu.Del); err != nil {
		return err
	}
	return nil
}

// QueryRequest wraps the state that is used when executing query.
// Initially Latency and GqlQuery needs to be set. Subgraphs, Vars
// and schemaUpdate are filled when processing query.
type QueryRequest struct {
	Latency  *Latency
	GqlQuery *gql.Result

	Subgraphs []*SubGraph

	vars         map[string]varValue
	SchemaUpdate []*protos.SchemaUpdate
}

// ProcessQuery processes query part of the request (without mutations).
// Fills Subgraphs and Vars.
func (req *QueryRequest) ProcessQuery(ctx context.Context) error {
	var err error

	// doneVars stores the processed variables.
	req.vars = make(map[string]varValue)
	loopStart := time.Now()
	queries := req.GqlQuery.Query
	for i := 0; i < len(queries); i++ {
		gq := queries[i]

		if gq == nil || (len(gq.UID) == 0 && gq.Func == nil && len(gq.NeedsVar) == 0 &&
			gq.Alias != "shortest" && !gq.IsEmpty) {
			err := x.Errorf("Invalid query, query internal id is zero and generator is nil")
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf(err.Error())
			}
			return err
		}
		sg, err := ToSubGraph(ctx, gq)
		if err != nil {
			return err
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Query parsed")
		}
		req.Subgraphs = append(req.Subgraphs, sg)
	}
	req.Latency.Parsing += time.Since(loopStart)

	execStart := time.Now()
	hasExecuted := make([]bool, len(req.Subgraphs))
	numQueriesDone := 0

	// canExecute returns true if a query block is ready to execute with all the variables
	// that it depends on are already populated or are defined in the same block.
	canExecute := func(idx int) bool {
		for _, v := range req.GqlQuery.QueryVars[idx].Needs {
			// here we check if this block defines the variable v.
			var selfDep bool
			for _, vd := range req.GqlQuery.QueryVars[idx].Defines {
				if v == vd {
					selfDep = true
					break
				}
			}
			// The variable should be defined in this block or should have already been
			// populated by some other block, otherwise we are not ready to execute yet.
			_, ok := req.vars[v]
			if !ok && !selfDep {
				return false
			}
		}
		return true
	}

	var shortestSg []*SubGraph
	for i := 0; i < len(req.Subgraphs) && numQueriesDone < len(req.Subgraphs); i++ {
		errChan := make(chan error, len(req.Subgraphs))
		var idxList []int
		// If we have N blocks in a query, it can take a maximum of N iterations for all of them
		// to be executed.
		for idx := 0; idx < len(req.Subgraphs); idx++ {
			if hasExecuted[idx] {
				continue
			}
			sg := req.Subgraphs[idx]
			// Check the list for the requires variables.
			if !canExecute(idx) {
				continue
			}

			err = sg.recursiveFillVars(req.vars)
			if err != nil {
				return err
			}
			hasExecuted[idx] = true
			numQueriesDone++
			idxList = append(idxList, idx)
			// Doesn't need to be executed as it just does aggregation and math functions.
			if sg.Params.IsEmpty {
				errChan <- nil
				continue
			}

			if sg.Params.Alias == "shortest" {
				// We allow only one shortest path block per query.
				go func() {
					shortestSg, err = ShortestPath(ctx, sg)
					errChan <- err
				}()
			} else if sg.Params.Alias == "recurse" {
				go func() {
					errChan <- Recurse(ctx, sg)
				}()
			} else {
				go ProcessGraph(ctx, sg, nil, errChan)
			}
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Graph processed")
			}
		}

		var ferr error
		// Wait for the execution that was started in this iteration.
		for i := 0; i < len(idxList); i++ {
			if err = <-errChan; err != nil {
				ferr = err
				if tr, ok := trace.FromContext(ctx); ok {
					tr.LazyPrintf("Error while processing Query: %+v", err)
				}
			}
		}
		if ferr != nil {
			return ferr
		}

		// If the executed subgraph had some variable defined in it, Populate it in the map.
		for _, idx := range idxList {
			sg := req.Subgraphs[idx]
			var sgPath []*SubGraph
			err = sg.populateVarMap(req.vars, sgPath)
			if err != nil {
				return err
			}
			err = sg.populatePostAggregation(req.vars, []*SubGraph{}, nil)
			if err != nil {
				return err
			}
		}
	}

	// Ensure all the queries are executed.
	for _, it := range hasExecuted {
		if !it {
			return x.Errorf("Query couldn't be executed")
		}
	}
	req.Latency.Processing += time.Since(execStart)

	// If we had a shortestPath SG, append it to the result.
	if len(shortestSg) != 0 {
		req.Subgraphs = append(req.Subgraphs, shortestSg...)
	}
	return nil
}

var MutationNotAllowedErr = x.Errorf("Mutations are forbidden on this server.")

type InvalidRequestError struct {
	err error
}

func (e *InvalidRequestError) Error() string {
	return "invalid request: " + e.err.Error()
}

type InternalError struct {
	err error
}

func (e *InternalError) Error() string {
	return "internal error: " + e.err.Error()
}

func (qr *QueryRequest) prepareMutation() (err error) {
	if len(qr.GqlQuery.Mutation.Schema) > 0 {
		if qr.SchemaUpdate, err = schema.Parse(qr.GqlQuery.Mutation.Schema); err != nil {
			return x.Wrapf(&InvalidRequestError{err: err}, "failed to parse schema")
		}
	}
	if err = parseFacetsInMutation(qr.GqlQuery.Mutation); err != nil {
		return err
	}
	return
}

func (qr *QueryRequest) processNquads(ctx context.Context, nquads gql.NQuads, newUids map[string]uint64) error {
	var err error
	var mr InternalMutation
	if !nquads.IsEmpty() {
		if mr, err = ToInternal(ctx, nquads, qr.vars, newUids); err != nil {
			return x.Wrapf(&InternalError{err: err}, "failed to convert NQuads to edges")
		}
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("converted nquads to directed edges")
	}
	m := protos.Mutations{Edges: mr.Edges, Schema: qr.SchemaUpdate}
	if err = ApplyMutations(ctx, &m); err != nil {
		return x.Wrapf(&InternalError{err: err}, "failed to apply mutations")
	}
	return nil
}

type ExecuteResult struct {
	Subgraphs   []*SubGraph
	SchemaNode  []*protos.SchemaNode
	Allocations map[string]uint64
}

func (qr *QueryRequest) ProcessWithMutation(ctx context.Context) (er ExecuteResult, err error) {
	// If we have mutations that don't depend on query, run them first.
	mutationAllowed, ok := ctx.Value("mutation_allowed").(bool)
	if !ok {
		mutationAllowed = false
	}

	var depSet, indepSet, depDel, indepDel gql.NQuads
	var newUids map[string]uint64
	if qr.GqlQuery.Mutation != nil {
		if qr.GqlQuery.Mutation.HasOps() && !mutationAllowed {
			return er, x.Wrap(&InvalidRequestError{err: MutationNotAllowedErr})
		}

		if err = qr.prepareMutation(); err != nil {
			return er, err
		}

		depSet, indepSet = gql.WrapNQ(qr.GqlQuery.Mutation.Set, protos.DirectedEdge_SET).
			Partition(gql.HasVariables)

		depDel, indepDel = gql.WrapNQ(qr.GqlQuery.Mutation.Del, protos.DirectedEdge_DEL).
			Partition(gql.HasVariables)

		nquads := indepSet.Add(indepDel)
		nquadsTemp := nquads.Add(depDel).Add(depSet)
		if newUids, err = AssignUids(ctx, nquadsTemp); err != nil {
			return er, err
		}

		er.Allocations = StripBlankNode(newUids)

		err = qr.processNquads(ctx, nquads, newUids)
		if err != nil {
			return er, err
		}
	}

	if len(qr.GqlQuery.Query) == 0 && qr.GqlQuery.Schema == nil {
		return er, nil
	}

	err = qr.ProcessQuery(ctx)
	if err != nil {
		return er, err
	}
	er.Subgraphs = qr.Subgraphs

	nquads := depSet.Add(depDel)
	if !nquads.IsEmpty() {
		if err = qr.processNquads(ctx, nquads, newUids); err != nil {
			return er, err
		}
	}

	if qr.GqlQuery.Schema != nil {
		if er.SchemaNode, err = worker.GetSchemaOverNetwork(ctx, qr.GqlQuery.Schema); err != nil {
			return er, x.Wrapf(&InternalError{err: err}, "error while fetching schema")
		}
	}
	return er, nil
}

func StripBlankNode(mp map[string]uint64) map[string]uint64 {
	temp := make(map[string]uint64)
	for k, v := range mp {
		if strings.HasPrefix(k, "_:") {
			temp[k[2:]] = v
		}
	}
	return temp
}
