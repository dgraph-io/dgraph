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
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos"
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
	Alias        string
	Count        int
	Offset       int
	AfterUID     uint64
	DoCount      bool
	GetUID       bool
	Order        string
	OrderDesc    bool
	isDebug      bool
	Var          string
	NeedsVar     []gql.VarContext
	ParentVars   map[string]varValue
	FacetVar     map[string]string
	uidToVal     map[uint64]types.Val
	Langs        []string
	Normalize    bool
	Cascade      bool
	From         uint64
	To           uint64
	Facet        *protos.Param
	RecurseDepth uint64
	isInternal   bool   // Determines if processTask has to be called or not.
	isListNode   bool   // This is for _predicate_ block.
	ignoreResult bool   // Node results are ignored.
	Expand       string // Var to use for expand.
	isGroupBy    bool
	groupbyAttrs []string
	uidCount     bool
}

// SubGraph is the way to represent data internally. It contains both the
// query and the response. Once generated, this can then be encoded to other
// client convenient formats, like GraphQL / JSON.
type SubGraph struct {
	Attr         string
	Params       params
	counts       []uint32
	values       []*protos.TaskValue
	uidMatrix    []*protos.List
	facetsMatrix []*protos.FacetsList
	ExpandPreds  []*protos.TaskValue
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

func (sg *SubGraph) IsListNode() bool {
	return sg.Params.isListNode
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
		dst, sg.Params.DoCount, len(sg.values))
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

// This method gets the values and children for a subprotos.
func (sg *SubGraph) preTraverse(uid uint64, dst, parent outputNode) error {
	invalidUids := make(map[uint64]bool)
	uidAlreadySet := false

	facetsNode := dst.New("@facets")
	// We go through all predicate children of the subprotos.
	for _, pc := range sg.Children {
		if pc.Params.ignoreResult {
			continue
		}
		if pc.Params.isGroupBy {
			g := dst.New(pc.Attr)
			for _, grp := range pc.GroupbyRes.group {
				uc := g.New("@groupby")
				for _, it := range grp.keys {
					uc.AddValue(it.attr, it.key)
				}
				for _, it := range grp.aggregates {
					uc.AddValue(it.attr, it.key)
				}
				g.AddListChild("@groupby", uc)
			}
			dst.AddMapChild(pc.Attr, g, false)
			continue
		}
		if pc.IsInternal() {
			if pc.Params.Expand != "" {
				continue
			}
			if pc.Params.uidToVal == nil {
				return x.Errorf("Wrong use of var() with %v.", pc.Params.NeedsVar)
			}
			fieldName := fmt.Sprintf("var(%v)", pc.Params.Var)
			if len(pc.Params.NeedsVar) > 0 {
				fieldName = fmt.Sprintf("var(%v)", pc.Params.NeedsVar[0].Name)
				if len(pc.SrcFunc) > 0 {
					fieldName = fmt.Sprintf("%s(%v)", pc.SrcFunc[0], fieldName)
				}
			}
			if pc.Params.Alias != "" {
				fieldName = pc.Params.Alias
			}
			sv, ok := pc.Params.uidToVal[uid]
			if !ok || sv.Value == nil {
				continue
			}
			if sv.Tid == types.StringID && sv.Value.(string) == "_nil_" {
				sv.Value = ""
			}
			dst.AddValue(fieldName, sv)
			continue
		}

		if pc.IsListNode() {
			for _, val := range pc.values {
				v, err := getValue(val)
				if err != nil {
					return err
				}
				sv, err := types.Convert(v, v.Tid)
				uc := dst.New(pc.Attr)
				uc.AddValue("_name_", sv)
				dst.AddListChild(pc.Attr, uc)
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

		fieldName := pc.Attr
		if pc.Params.Alias != "" {
			fieldName = pc.Params.Alias
		}
		if !uidAlreadySet && (sg.Params.GetUID || sg.Params.isDebug) {
			uidAlreadySet = true
			dst.SetUID(uid)
		}

		if pc.Params.DoCount {
			c := types.ValueForType(types.IntID)
			c.Value = int64(pc.counts[idx])
			fieldName = fmt.Sprintf("count(%s)", pc.Attr)
			if pc.Params.Alias != "" {
				fieldName = pc.Params.Alias
			}
			dst.AddValue(fieldName, c)
			continue
		}

		if len(pc.SrcFunc) > 0 && pc.SrcFunc[0] == "checkpwd" {
			c := types.ValueForType(types.BoolID)
			c.Value = task.ToBool(pc.values[idx])
			uc := dst.New(pc.Attr)
			uc.AddValue("checkpwd", c)
			dst.AddListChild(pc.Attr, uc)
		} else if len(ul.Uids) > 0 {
			// We create as many predicate entity children as the length of uids for
			// this predicate.
			var fcsList []*protos.Facets
			if pc.Params.Facet != nil {
				fcsList = pc.facetsMatrix[idx].FacetsList
			}

			for childIdx, childUID := range ul.Uids {
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
			if pc.Params.uidCount {
				uc := dst.New(fieldName)
				c := types.ValueForType(types.IntID)
				c.Value = int64(len(ul.Uids))
				uc.AddValue("count", c)
				dst.AddListChild(fieldName, uc)
			}
		} else {
			if pc.Params.Alias == "" && len(pc.Params.Langs) > 0 {
				fieldName += "@"
				for _, it := range pc.Params.Langs {
					fieldName += it + ":"
				}
				fieldName = fieldName[:len(fieldName)-1]
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
					fc.AddValue(f.Key, facets.ValFor(f))
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
				xidVal := txt.Value.(string)
				// If xid is empty, then we don't wan't to set it.
				if xidVal == "" {
					continue
				}
				dst.SetXID(xidVal)
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
		sg.SrcFunc = append(sg.SrcFunc, ft.Func.Lang)
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

func uniqueKey(gchild *gql.GraphQuery) string {
	key := gchild.Attr
	if gchild.Func != nil {
		key += fmt.Sprintf("%v", gchild.Func)
	}
	if len(gchild.NeedsVar) > 0 {
		key += fmt.Sprintf("%v", gchild.NeedsVar)
	}
	if gchild.IsCount { // ignore count subgraphs..
		key += "count"
	}
	if len(gchild.Langs) > 0 {
		key += fmt.Sprintf("%v", gchild.Langs)
	}
	if gchild.MathExp != nil {
		key += fmt.Sprintf("%+v", gchild.MathExp)
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
			Alias:        gchild.Alias,
			Langs:        gchild.Langs,
			isDebug:      sg.Params.isDebug,
			Var:          gchild.Var,
			Normalize:    sg.Params.Normalize,
			isInternal:   gchild.IsInternal,
			Expand:       gchild.Expand,
			isGroupBy:    gchild.IsGroupby,
			groupbyAttrs: gchild.GroupbyAttrs,
			FacetVar:     gchild.FacetVar,
			uidCount:     gchild.UidCount,
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
	if v, ok := gq.Args["depth"]; ok && args.Alias == "recurse" {
		from, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			return err
		}
		args.RecurseDepth = from
	}
	if v, ok := gq.Args["from"]; ok && args.Alias == "shortest" {
		from, err := strconv.ParseUint(v, 0, 64)
		if err != nil {
			// Treat it as an XID.
			from = farm.Fingerprint64([]byte(v))
		}
		args.From = uint64(from)
	}
	if v, ok := gq.Args["to"]; ok && args.Alias == "shortest" {
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

// newGraph returns the SubGraph and its task query.
func newGraph(ctx context.Context, gq *gql.GraphQuery) (*SubGraph, error) {
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.
	if len(gq.UID) == 0 && gq.Func == nil && len(gq.NeedsVar) == 0 && gq.Alias != "shortest" {
		err := x.Errorf("Invalid query, query internal id is zero and generator is nil")
		x.TraceError(ctx, err)
		return nil, err
	}

	// For the root, the name to be used in result is stored in Alias, not Attr.
	// The attr at root (if present) would stand for the source functions attr.
	args := params{
		isDebug:    isDebug(ctx),
		Alias:      gq.Alias,
		Langs:      gq.Langs,
		Var:        gq.Var,
		ParentVars: make(map[string]varValue),
		Normalize:  gq.Normalize,
		Cascade:    gq.Cascade,
		isGroupBy:  gq.IsGroupby,
		uidCount:   gq.UidCount,
	}
	if gq.Facets != nil {
		args.Facet = &protos.Param{gq.Facets.AllKeys, gq.Facets.Keys}
	}

	for _, it := range gq.NeedsVar {
		args.NeedsVar = append(args.NeedsVar, it)
	}

	if gq.IsCount {
		if len(gq.Children) != 0 {
			return nil, fmt.Errorf("Cannot have children attributes when asking for count.")
		}
		args.DoCount = true
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
		sg.Attr = gq.Func.Attr
		if !isValidFuncName(gq.Func.Name) {
			return nil, x.Errorf("Invalid function name : %s", gq.Func.Name)
		}
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Name)
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Lang)
		sg.SrcFunc = append(sg.SrcFunc, gq.Func.Args...)
	}
	if len(gq.UID) > 0 {
		o := make([]uint64, len(gq.UID))
		copy(o, gq.UID)
		sg.uidMatrix = []*protos.List{{gq.UID}}
		// User specified list may not be sorted.
		sort.Slice(o, func(i, j int) bool { return o[i] < o[j] })
		sg.SrcUIDs = &protos.List{o}
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

func createNilValuesList(count int) []*protos.TaskValue {
	out := make([]*protos.TaskValue, count)
	for i := 0; i < count; i++ {
		out[i] = &protos.TaskValue{
			Val: x.Nilbyte,
		}
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
	uids *protos.List
	vals map[uint64]types.Val
	path []*SubGraph // This stores the subgraph path from root to var definition.
	// TODO: Check if we can do without this field.
	strList []*protos.TaskValue
}

func evalLevelAgg(doneVars map[string]varValue, sg, parent *SubGraph) (mp map[uint64]types.Val,
	rerr error) {
	if parent == nil {
		return mp, ErrWrongAgg
	}
	var relSG *SubGraph
	needsVar := sg.Params.NeedsVar[0].Name
	for _, ch := range parent.Children {
		if sg == ch {
			continue
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

	vals := doneVars[needsVar].vals
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
		if err != nil {
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
func (fromNode *varValue) transformTo(toNode varValue) (map[uint64]types.Val, error) {
	x.AssertTrue(len(toNode.path) >= len(fromNode.path))
	idx := 0
	for ; idx < len(fromNode.path); idx++ {
		if fromNode.path[idx] != toNode.path[idx] {
			return nil, x.Errorf("Invalid combination of variables in math")
		}
	}

	newMap := fromNode.vals
	if newMap == nil {
		newMap = make(map[uint64]types.Val)
	}
	for ; idx < len(toNode.path); idx++ {
		curNode := toNode.path[idx]
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
func transformVars(mNode *mathTree, doneVars map[string]varValue) error {
	mvarList := mNode.extractVarNodes()
	// Iterate over the node list to find the node at the lowest level.
	var maxVar string
	var maxLevel int
	for _, mt := range mvarList {
		mvarVal, ok := doneVars[mt.Var]
		if !ok {
			return x.Errorf("Variable not yet populated: %v", mt.Var)
		}
		if maxLevel < len(mvarVal.path) {
			maxLevel = len(mvarVal.path)
			maxVar = mt.Var
		}
	}

	maxNode := doneVars[maxVar]
	for i := 0; i < len(mvarList); i++ {
		mt := mvarList[i]
		curNode := doneVars[mt.Var]
		newMap, err := curNode.transformTo(maxNode)
		if err != nil {
			return err
		}
		mt.Val = newMap
	}
	return nil
}

func (sg *SubGraph) valueVarAggregation(doneVars map[string]varValue, parent *SubGraph) error {
	if !sg.IsInternal() && !sg.IsGroupBy() {
		return nil
	}

	if sg.IsGroupBy() {
		err := sg.processGroupBy(doneVars)
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
			doneVars[sg.Params.Var] = varValue{
				vals: mp,
			}
		}
		sg.Params.uidToVal = mp
	} else if sg.MathExp != nil {
		// Preprocess to bring all variables to the same level.
		err := transformVars(sg.MathExp, doneVars)
		if err != nil {
			return err
		}

		err = evalMathTree(sg.MathExp)
		if err != nil {
			return err
		}
		if sg.MathExp.Val != nil {
			doneVars[sg.Params.Var] = varValue{vals: sg.MathExp.Val}
		} else if sg.MathExp.Const.Value != nil {
			// Assign the const for all the srcUids.
			mp := make(map[uint64]types.Val)
			for _, uid := range sg.SrcUIDs.Uids {
				mp[uid] = sg.MathExp.Const
			}
			doneVars[sg.Params.Var] = varValue{vals: mp}
		} else {
			return x.Errorf("Missing values/constant in math expression")
		}
		// Put it in this node.
		sg.Params.uidToVal = sg.MathExp.Val
	} else if len(sg.Params.NeedsVar) > 0 {
		// This is a var() block.
		srcVar := sg.Params.NeedsVar[0]
		srcMap := doneVars[srcVar.Name]
		if srcMap.vals == nil {
			return x.Errorf("Missing value variable %v", srcVar)
		}
		sg.Params.uidToVal = srcMap.vals
	} else {
		return x.Errorf("Unhandled internal node %v with parent %v", sg.Attr, parent.Attr)
	}
	return nil
}

func (sg *SubGraph) populatePostAggregation(doneVars map[string]varValue, parent *SubGraph) error {
	for idx := 0; idx < len(sg.Children); idx++ {
		child := sg.Children[idx]
		err := child.populatePostAggregation(doneVars, sg)
		if err != nil {
			return err
		}
	}
	return sg.valueVarAggregation(doneVars, parent)
}

func ProcessQuery(ctx context.Context, res gql.Result, l *Latency) ([]*SubGraph, error) {
	var sgl []*SubGraph
	var err error

	// doneVars will store the UID list of the corresponding variables.
	doneVars := make(map[string]varValue)
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
			_, ok := doneVars[v]
			if !ok && !selfDep {
				return false
			}
		}
		return true
	}

	var shortestSg *SubGraph
	for i := 0; i < len(sgl) && numQueriesDone < len(sgl); i++ {
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

			err = sg.recursiveFillVars(doneVars)
			if err != nil {
				return nil, err
			}
			hasExecuted[idx] = true
			numQueriesDone++
			idxList = append(idxList, idx)
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
			x.Trace(ctx, "Graph processed")
		}

		// Wait for the execution that was started in this iteration.
		for i := 0; i < len(idxList); i++ {
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
			isCascade := sg.Params.Cascade || shouldCascade(res, idx)
			var sgPath []*SubGraph
			err = sg.populateVarMap(doneVars, isCascade, sgPath)
			if err != nil {
				return nil, err
			}
			err = sg.populatePostAggregation(doneVars, nil)
			if err != nil {
				return nil, err
			}
		}
	}

	// Ensure all the queries are executed.
	for _, it := range hasExecuted {
		if !it {
			return nil, x.Errorf("Query couldn't be executed")
		}
	}
	l.Processing += time.Since(execStart)

	// If we had a shortestPath SG, append it to the result.
	if shortestSg != nil {
		sgl = append(sgl, shortestSg)
	}
	return sgl, nil
}

// shouldCascade returns true if the query block is not self depenedent and we should
// remove the uids from the bottom up if the children are empty.
func shouldCascade(res gql.Result, idx int) bool {
	if res.Query[idx].Attr == "shortest" {
		return false
	}

	if len(res.QueryVars[idx].Defines) == 0 {
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

func (sg *SubGraph) populateVarMap(doneVars map[string]varValue, isCascade bool,
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
		child.populateVarMap(doneVars, isCascade, sgPath)
		sgPath = sgPath[:len(sgPath)-1] // Backtrack
		if !isCascade {
			continue
		}

		// Intersect the UidMatrix with the DestUids as some UIDs might have been removed
		// by other operations. So we need to apply it on the UidMatrix.
		child.updateUidMatrix()
	}

	if !isCascade {
		goto AssignStep
	}

	// Filter out UIDs that don't have atleast one UID in every child.
	for i, uid := range sg.DestUIDs.Uids {
		var exclude bool
		for _, child := range sg.Children {
			// If the length of child UID list is zero and it has no valid value, then the
			// current UID should be removed from this level.
			if !child.IsInternal() && (len(child.values) <= i || len(child.values[i].Val) == 0) && (len(child.counts) <= i) &&
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
	if sg.Params.Var != "" {
		if sg.IsListNode() {
			// This is a predicates list.
			doneVars[sg.Params.Var] = varValue{
				strList: sg.values,
				path:    sgPath,
			}
		} else if len(sg.DestUIDs.Uids) != 0 {
			// This implies it is a entity variable.
			doneVars[sg.Params.Var] = varValue{
				uids: sg.DestUIDs,
				path: sgPath,
			}
		} else if len(sg.counts) != 0 {
			// This implies it is a value variable.
			doneVars[sg.Params.Var] = varValue{
				vals: make(map[uint64]types.Val),
				path: sgPath,
			}
			for idx, uid := range sg.SrcUIDs.Uids {
				val := types.Val{
					Tid:   types.IntID,
					Value: int64(sg.counts[idx]),
				}
				doneVars[sg.Params.Var].vals[uid] = val
			}
		} else if len(sg.values) != 0 && sg.SrcUIDs != nil && len(sgPath) != 0 {
			// This implies it is a value variable.
			// NOTE: Value variables cannot be defined and used in the same query block. so
			// checking len(sgPath) is okay.
			doneVars[sg.Params.Var] = varValue{
				vals: make(map[uint64]types.Val),
				path: sgPath,
			}
			for idx, uid := range sg.SrcUIDs.Uids {
				val, err := convertWithBestEffort(sg.values[idx], sg.Attr)
				if err != nil {
					continue
				}
				doneVars[sg.Params.Var].vals[uid] = val
			}
		} else {
			// Insert a empty entry to keep the dependency happy.
			doneVars[sg.Params.Var] = varValue{
				path: sgPath,
			}
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
				vals: make(map[uint64]types.Val),
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
						if pVal, ok := doneVars[fvar].vals[uid]; !ok {
							doneVars[fvar].vals[uid] = facets.ValFor(f)
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
							doneVars[fvar].vals[uid] = fVal
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
			} else if (v.Typ == gql.ANY_VAR || v.Typ == gql.UID_VAR) && l.uids != nil {
				lists = append(lists, l.uids)
			} else if (v.Typ == gql.ANY_VAR || v.Typ == gql.VALUE_VAR) && len(l.vals) != 0 {
				// This should happened only once.
				sg.Params.uidToVal = l.vals
			} else if len(l.vals) != 0 && (v.Typ == gql.ANY_VAR || v.Typ == gql.UID_VAR) {
				// Derive the UID list from value var.
				uids := make([]uint64, 0, len(l.vals))
				for k := range l.vals {
					uids = append(uids, k)
				}
				sort.Slice(uids, func(i, j int) bool {
					return uids[i] < uids[j]
				})
				lists = append(lists, &protos.List{uids})
			} else if len(l.vals) != 0 || l.uids != nil {
				return x.Errorf("Wrong variable type encountered for var(%v) %v.", v.Name, v.Typ)
			}
		}
	}
	lists = append(lists, sg.DestUIDs)
	sg.DestUIDs = algo.MergeSorted(lists)
	return nil
}

func populateDestUIDs(result *protos.Result, sg *SubGraph) {
	if result.IntersectDest {
		sg.DestUIDs = algo.IntersectSorted(result.UidMatrix)
	} else {
		sg.DestUIDs = algo.MergeSorted(result.UidMatrix)
	}

}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances. Note: taskQuery is nil for root node.
func ProcessGraph(ctx context.Context, sg, parent *SubGraph, rch chan error) {
	var err error
	if parent == nil && len(sg.SrcFunc) == 0 {
		// I'm root and I'm using some varaible that has been populated.
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
		// If we have a filter SubGraph which only contains an operator,
		// it won't have any attribute to work on.
		// This is to allow providing SrcUIDs to the filter children.
		sg.DestUIDs = sg.SrcUIDs
	} else {
		if len(sg.SrcFunc) > 0 && sg.SrcFunc[0] == "var" {
			// If its a var() filter, we just have to intersect the SrcUIDs with DestUIDs
			// and return.
			sg.fillVars(sg.Params.ParentVars)
			algo.IntersectWith(sg.DestUIDs, sg.SrcUIDs, sg.DestUIDs)
			rch <- nil
			return
		}

		if sg.Attr == "count" && sg.Params.DoCount {
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

		if sg.Attr == "_predicate_" {
			sg.Params.isListNode = true
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

		populateDestUIDs(result, sg)
		if parent == nil {
			// I'm root. We reach here if root had a function.
			sg.uidMatrix = []*protos.List{sg.DestUIDs}
		}
	}

	if sg.DestUIDs == nil || len(sg.DestUIDs.Uids) == 0 {
		// Looks like we're done here. Be careful with nil srcUIDs!
		x.Trace(ctx, "Zero uids for %q. Num attr children: %v", sg.Attr, len(sg.Children))
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
			filter.SrcUIDs = sg.DestUIDs
			filter.Params.ParentVars = sg.Params.ParentVars // Pass to the child.
			go ProcessGraph(ctx, filter, sg, filterChan)
		}

		for range sg.Filters {
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
		var lists []*protos.List
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

	// We are asked for count at root. Filtering, ordering and pagination is done.
	// We can return now.
	if parent == nil && sg.Params.DoCount {
		rch <- nil
		return
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
		for i, ul := range sg.uidMatrix {
			// A possible optimization is to return the size of the intersection
			// without forming the intersection.
			algo.IntersectWith(ul, sg.DestUIDs, ul)
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

			for _, v := range child.ExpandPreds {
				temp := new(SubGraph)
				*temp = *child
				temp.Params.isInternal = false
				temp.Params.Expand = ""
				if v.ValType != int32(types.StringID) {
					rch <- x.Errorf("Expected a string type")
					return
				}
				temp.Attr = string(v.Val)
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
		for _, attr := range sg.Params.groupbyAttrs {
			sg.Children = append(sg.Children, &SubGraph{
				Attr: attr,
				Params: params{
					ignoreResult: true,
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

	// Now get all the results back.
	for _, child := range sg.Children {
		if child.IsInternal() {
			continue
		}
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
	if len(sg.Params.Order) == 0 {
		return nil
	}
	if sg.Params.Count == 0 {
		// Only retrieve up to 1000 results by default.
		sg.Params.Count = 1000
	}

	sg.updateUidMatrix()

	for _, it := range sg.Params.NeedsVar {
		if it.Name == sg.Params.Order {
			// If the Order name is same as var name, we sort using that variable.
			return sg.sortAndPaginateUsingVar(ctx)
		}
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
	sg.uidMatrix = result.GetUidMatrix()

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

func (sg *SubGraph) sortAndPaginateUsingVar(ctx context.Context) error {
	if sg.Params.uidToVal == nil {
		return nil
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
	case "from", "to", "orderasc", "orderdesc", "first", "offset", "after", "depth":
		return true
	}
	return false
}

// isValidFuncName checks if fn passed is valid keyword.
func isValidFuncName(f string) bool {
	switch f {
	case "anyofterms", "allofterms", "var", "regexp", "anyoftext", "alloftext":
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

func isAggregatorFn(f string) bool {
	switch f {
	case "min", "max", "sum", "avg":
		return true
	}
	return false
}

func GetNodePredicates(ctx context.Context, uids *protos.List) ([]*protos.TaskValue, error) {
	temp := new(SubGraph)
	temp.Attr = "_predicate_"
	temp.SrcUIDs = uids
	temp.Params.isListNode = true
	taskQuery := createTaskQuery(temp)
	result, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
	if err != nil {
		return nil, err
	}
	return result.Values, nil
}
