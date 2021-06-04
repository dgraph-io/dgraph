/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package query

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/sroar"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	otrace "go.opencensus.io/trace"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
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
 * This would be represented in SubGraph format pb.y, as such:
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
	Start           time.Time     `json:"-"`
	Parsing         time.Duration `json:"query_parsing"`
	AssignTimestamp time.Duration `json:"assign_timestamp"`
	Processing      time.Duration `json:"processing"`
	Json            time.Duration `json:"json_conversion"`
}

// params contains the list of parameters required to execute a SubGraph.
type params struct {
	// Alias is the value of the predicate's alias, if any.
	Alias string
	// Count is the value of "first" parameter in the query.
	Count int
	// Offset is the value of the "offset" parameter.
	Offset int
	// Random is the value of the "random" parameter
	Random int
	// AfterUID is the value of the "after" parameter.
	AfterUID uint64
	// DoCount is true if the count of the predicate is requested instead of its value.
	DoCount bool
	// GetUid is true if the uid should be returned. Used for debug requests.
	GetUid bool
	// Order is the list of predicates to sort by and their sort order.
	Order []*pb.Order
	// Langs is the list of languages and their preferred order for looking up a predicate value.
	Langs []string

	// Facet tells us about the requested facets and their aliases.
	Facet *pb.FacetParams
	// FacetsOrder keeps ordering for facets. Each entry stores name of the facet key and
	// OrderDesc(will be true if results should be ordered by desc order of key) information for it.
	FacetsOrder []*gql.FacetOrder

	// Var is the name of the variable defined in this SubGraph
	// (e.g. in "x as name", this would be x).
	Var string
	// FacetVar is a map of predicate to the facet variable alias
	// for e.g. @facets(L1 as weight) the map would be { "weight": "L1" }
	FacetVar map[string]string
	// NeedsVar is the list of variables required by this SubGraph along with their type.
	NeedsVar []gql.VarContext

	// ParentVars is a map of variables passed down recursively to children of a SubGraph in a query
	// block. These are used to filter uids defined in a parent using a variable.
	// TODO (pawan) - This can potentially be simplified to a map[string]*pb.List since we don't
	// support reading from value variables defined in the parent and other fields that are part
	// of varValue.
	ParentVars map[string]varValue

	// UidToVal is the mapping of uid to values. This is populated into a SubGraph from a value
	// variable that is part of req.Vars. This value variable would have been defined
	// in some other query.
	UidToVal map[uint64]types.Val

	// Normalize is true if the @normalize directive is specified.
	Normalize bool
	// Recurse is true if the @recurse directive is specified.
	Recurse bool
	// RecurseArgs stores the arguments passed to the @recurse directive.
	RecurseArgs gql.RecurseArgs
	// Cascade is the list of predicates to apply @cascade to.
	// __all__ is special to mean @cascade i.e. all the children of this subgraph are mandatory
	// and should have values otherwise the node will be excluded.
	Cascade *CascadeArgs
	// IgnoreReflex is true if the @ignorereflex directive is specified.
	IgnoreReflex bool

	// ShortestPathArgs contains the from and to functions to execute a shortest path query.
	ShortestPathArgs gql.ShortestPathArgs
	// From is the node from which to run the shortest path algorithm.
	From uint64
	// To is the destination node of the shortest path algorithm
	To uint64
	// NumPaths is used for k-shortest path query to specify number of paths to return.
	NumPaths int
	// MaxWeight is the max weight allowed in a path returned by the shortest path algorithm.
	MaxWeight float64
	// MinWeight is the min weight allowed in a path returned by the shortest path algorithm.
	MinWeight float64

	// ExploreDepth is used by recurse and shortest path queries to specify the maximum graph
	// depth to explore.
	ExploreDepth *uint64

	// IsInternal determines if processTask has to be called or not.
	IsInternal bool
	// IgnoreResult is true if the node results are to be ignored.
	IgnoreResult bool
	// Expand holds the argument passed to the expand function.
	Expand string

	// IsGroupBy is true if @groupby is specified.
	IsGroupBy bool // True if @groupby is specified.
	// GroupbyAttrs holds the list of attributes to group by.
	GroupbyAttrs []gql.GroupByAttr

	// ParentIds is a stack that is maintained and passed down to children.
	ParentIds []uint64
	// IsEmpty is true if the subgraph doesn't have any SrcUids or DestUids.
	// Only used to get aggregated vars
	IsEmpty bool
	// ExpandAll is true if all the language values should be expanded.
	ExpandAll bool
	// Shortest is true when the subgraph holds the results of a shortest paths query.
	Shortest bool
	// AllowedPreds is a list of predicates accessible to query in context of ACL.
	// For OSS this should remain nil.
	AllowedPreds []string
}

// CascadeArgs stores the arguments needed to process @cascade directive.
// It is introduced to ensure correct behaviour for cascade with pagination.
type CascadeArgs struct {
	Fields []string
	First  int
	Offset int
}

type pathMetadata struct {
	weight float64 // Total weight of the path.
}

// Function holds the information about gql functions.
type Function struct {
	Name       string    // Specifies the name of the function.
	Args       []gql.Arg // Contains the arguments of the function.
	IsCount    bool      // gt(count(friends),0)
	IsValueVar bool      // eq(val(s), 10)
	IsLenVar   bool      // eq(len(s), 10)
}

// SubGraph is the way to represent data. It contains both the request parameters and the response.
// Once generated, this can then be encoded to other client convenient formats, like GraphQL / JSON.
// SubGraphs are recursively nested. Each SubGraph contain the following:
// * SrcUIDS: A list of UIDs that were sent to this query. If this subgraph is a child graph, then the
//            DestUIDs of the parent must match the SrcUIDs of the children.
// * DestUIDs: A list of UIDs for which there can be output found in the Children field
// * Children: A list of child results for this query
// * valueMatrix: A list of values, against a single attribute, such as name (for a scalar subgraph).
//                This must be the same length as the SrcUIDs
// * uidMatrix: A list of outgoing edges. This must be same length as the SrcUIDs list.
// Example, say we are creating a SubGraph for a query "users", which returns one user with name 'Foo', you may get
// SubGraph
//   Params: { Alias: "users" }
//   SrcUIDs: [1]
//   DestUIDs: [1]
//   uidMatrix: [[1]]
//   Children:
//     SubGraph:
//       Attr: "name"
//       SrcUIDs: [1]
//       uidMatrix: [[]]
//       valueMatrix: [["Foo"]]
type SubGraph struct {
	ReadTs      uint64
	Cache       int
	Attr        string
	UnknownAttr bool
	// read only parameters which are populated before the execution of the query and are used to
	// execute this query.
	Params params

	// count stores the count of an edge (predicate). There would be one value corresponding to each
	// uid in SrcUIDs.
	counts []uint32
	// valueMatrix is a slice of ValueList. If this SubGraph is for a scalar predicate type, then
	// there would be one list for each uid in SrcUIDs storing the value of the predicate.
	// The individual elements of the slice are a ValueList because we support scalar predicates
	// of list type. For non-list type scalar predicates, there would be only one value in every
	// ValueList.
	valueMatrix []*pb.ValueList
	// uidMatrix is a slice of List. There would be one List corresponding to each uid in SrcUIDs.
	// In graph terms, a list is a slice of outgoing edges from a node.
	uidMatrix []*pb.List

	// facetsMatrix contains the facet values. There would a list corresponding to each uid in
	// uidMatrix.
	// TODO: Would make sense to move these to a map.
	facetsMatrix []*pb.FacetsList
	ExpandPreds  []*pb.ValueList
	GroupbyRes   []*groupResults // one result for each uid list.
	LangTags     []*pb.LangList

	// SrcUIDs is a list of unique source UIDs. They are always copies of destUIDs
	// of parent nodes in GraphQL structure.
	SrcUIDs *pb.List
	// SrcFunc specified using func. Should only be non-nil at root. At other levels,
	// filters are used.
	SrcFunc *Function

	FilterOp     string
	Filters      []*SubGraph // List of filters specified at the current node.
	facetsFilter *pb.FilterTree
	MathExp      *mathTree
	Children     []*SubGraph // children of the current node, should be empty for leaf nodes.

	// destUIDs is a list of destination UIDs, after applying filters, pagination.
	DestMap *sroar.Bitmap

	// OrderedUIDs is used to store the UIDs in some order, used for shortest path.
	OrderedUIDs *pb.List
	List        bool // whether predicate is of list type

	pathMeta *pathMetadata
}

func (sg *SubGraph) recurse(set func(sg *SubGraph)) {
	set(sg)
	for _, child := range sg.Children {
		child.recurse(set)
	}
	for _, filter := range sg.Filters {
		filter.recurse(set)
	}
}

// IsGroupBy returns whether this subgraph is part of a groupBy query.
func (sg *SubGraph) IsGroupBy() bool {
	return sg.Params.IsGroupBy
}

// IsInternal returns whether this subgraph is marked as internal.
func (sg *SubGraph) IsInternal() bool {
	return sg.Params.IsInternal
}

func (sg *SubGraph) createSrcFunction(gf *gql.Function) {
	if gf == nil {
		return
	}

	sg.SrcFunc = &Function{
		Name:       gf.Name,
		Args:       append(gf.Args[:0:0], gf.Args...),
		IsCount:    gf.IsCount,
		IsValueVar: gf.IsValueVar,
		IsLenVar:   gf.IsLenVar,
	}

	// type function is just an alias for eq(type, "dgraph.type").
	if gf.Name == "type" {
		sg.Attr = "dgraph.type"
		sg.SrcFunc.Name = "eq"
		sg.SrcFunc.IsCount = false
		sg.SrcFunc.IsValueVar = false
		sg.SrcFunc.IsLenVar = false
		return
	}

	if gf.Lang != "" {
		sg.Params.Langs = append(sg.Params.Langs, gf.Lang)
	}
}

// DebugPrint prints out the SubGraph tree in a nice format for debugging purposes.
func (sg *SubGraph) DebugPrint(prefix string) {
	var src, dst int
	if sg.SrcUIDs != nil {
		src = len(sg.SrcUIDs.Uids)
	}
	if sg.DestMap != nil {
		dst = int(sg.DestMap.GetCardinality())
	}
	glog.Infof("%s[%q Alias:%q Func:%v SrcSz:%v Op:%q DestSz:%v IsCount: %v ValueSz:%v]\n",
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
func getValue(tv *pb.TaskValue) (types.Val, error) {
	vID := types.TypeID(tv.ValType)
	val := types.ValueForType(vID)
	val.Value = tv.Val
	return val, nil
}

var (
	// ErrEmptyVal is returned when a value is empty.
	ErrEmptyVal = errors.New("Query: harmless error, e.g. task.Val is nil")
	// ErrWrongAgg is returned when value aggregation is attempted in the root level of a query.
	ErrWrongAgg = errors.New("Wrong level for var aggregation")
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
		return ssg.Params.DoCount
	}
	if ssg.Params.DoCount {
		return false
	}
	if sg.SrcFunc != nil {
		if ssg.SrcFunc != nil && sg.SrcFunc.Name == ssg.SrcFunc.Name {
			return true
		}
		return false
	}
	// Below check doesn't differentiate between different filters.
	// It is added to differential between `hasFriend` and `hasFriend @filter()`
	if sg.Filters != nil {
		if ssg.Filters != nil && len(sg.Filters) == len(ssg.Filters) {
			return true
		}
		return false
	}
	return true
}

func isEmptyIneqFnWithVar(sg *SubGraph) bool {
	return sg.SrcFunc != nil && isInequalityFn(sg.SrcFunc.Name) && len(sg.SrcFunc.Args) == 0 &&
		len(sg.Params.NeedsVar) > 0
}

// convert from task.Val to types.Value, based on schema appropriate type
// is already set in api.Value
func convertWithBestEffort(tv *pb.TaskValue, attr string) (types.Val, error) {
	// value would be in binary format with appropriate type
	tid := types.TypeID(tv.ValType)
	if !tid.IsScalar() {
		return types.Val{}, errors.Errorf("Leaf predicate:'%v' must be a scalar.", attr)
	}

	// creates appropriate type from binary format
	sv, err := types.Convert(types.Val{Tid: types.BinaryID, Value: tv.Val}, tid)
	if err != nil {
		// This can happen when a mutation ingests corrupt data into the database.
		return types.Val{}, errors.Wrapf(err, "error interpreting appropriate type for %v", attr)
	}
	return sv, nil
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
			return errors.Errorf("Invalid function name: %s", ft.Func.Name)
		}

		if isUidFnWithoutVar(ft.Func) {
			sg.SrcFunc = &Function{Name: ft.Func.Name}
			if err := sg.populate(ft.Func.UID); err != nil {
				return err
			}
		} else {
			if ft.Func.Attr == "uid" {
				return errors.Errorf(`Argument cannot be "uid"`)
			}
			sg.createSrcFunction(ft.Func)
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
		switch {
		case gchild.Var != "" && gchild.Var != "val":
			key = fmt.Sprintf("val(%v)", gchild.Var)
		case len(gchild.NeedsVar) > 0:
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

func treeCopy(gq *gql.GraphQuery, sg *SubGraph) error {
	// Typically you act on the current node, and leave recursion to deal with
	// children. But, in this case, we don't want to muck with the current
	// node, because of the way we're dealing with the root node.
	// So, we work on the children, and then recurse for grand children.
	attrsSeen := make(map[string]struct{})

	for _, gchild := range gq.Children {
		if sg.Params.Alias == "shortest" && gchild.Expand != "" {
			return errors.Errorf("expand() not allowed inside shortest")
		}

		key := ""
		if gchild.Alias != "" {
			key = gchild.Alias
		} else {
			key = uniqueKey(gchild)
		}
		if _, ok := attrsSeen[key]; ok {
			return errors.Errorf("%s not allowed multiple times in same sub-query.",
				key)
		}
		attrsSeen[key] = struct{}{}

		args := params{
			Alias:        gchild.Alias,
			Expand:       gchild.Expand,
			Facet:        gchild.Facets,
			FacetsOrder:  gchild.FacetsOrder,
			FacetVar:     gchild.FacetVar,
			GetUid:       sg.Params.GetUid,
			IgnoreReflex: sg.Params.IgnoreReflex,
			Langs:        gchild.Langs,
			NeedsVar:     append(gchild.NeedsVar[:0:0], gchild.NeedsVar...),
			Normalize:    gchild.Normalize || sg.Params.Normalize,
			Order:        gchild.Order,
			Var:          gchild.Var,
			GroupbyAttrs: gchild.GroupbyAttrs,
			IsGroupBy:    gchild.IsGroupby,
			IsInternal:   gchild.IsInternal,
			Cascade:      &CascadeArgs{},
		}

		// Inherit from the parent.
		if len(sg.Params.Cascade.Fields) > 0 {
			args.Cascade.Fields = append(args.Cascade.Fields, sg.Params.Cascade.Fields...)
		}
		// Allow over-riding at this level.
		if len(gchild.Cascade) > 0 {
			args.Cascade.Fields = gchild.Cascade
		}

		// Remove pagination arguments from the query if @cascade is mentioned since
		// pagination will be applied post processing the data.
		if len(args.Cascade.Fields) > 0 {
			args.addCascadePaginationArguments(gchild)
		}

		if gchild.IsCount {
			if len(gchild.Children) != 0 {
				return errors.New("Node with count cannot have child attributes")
			}
			args.DoCount = true
		}

		for argk := range gchild.Args {
			if !isValidArg(argk) {
				return errors.Errorf("Invalid argument: %s", argk)
			}
		}
		if err := args.fill(gchild); err != nil {
			return err
		}

		if len(args.Order) != 0 && len(args.FacetsOrder) != 0 {
			return errors.Errorf("Cannot specify order at both args and facets")
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
			if len(gchild.Children) != 0 {
				return errors.Errorf("Node with %q cant have child attr", gchild.Func.Name)
			}
			// embedded filter will cause ambiguous output like following,
			// director.film @filter(gt(initial_release_date, "2016")) {
			//    min(initial_release_date @filter(gt(initial_release_date, "1986"))
			// }
			if gchild.Filter != nil {
				return errors.Errorf(
					"Node with %q cant have filter, please place the filter on the upper level",
					gchild.Func.Name)
			}
			if gchild.Func.Attr == "uid" {
				return errors.Errorf(`Argument cannot be "uid"`)
			}
			dst.createSrcFunction(gchild.Func)
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
		if err := treeCopy(gchild, dst); err != nil {
			return err
		}
	}
	return nil
}

func (args *params) addCascadePaginationArguments(gq *gql.GraphQuery) {
	args.Cascade.First, _ = strconv.Atoi(gq.Args["first"])
	delete(gq.Args, "first")
	args.Cascade.Offset, _ = strconv.Atoi(gq.Args["offset"])
	delete(gq.Args, "offset")
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
		args.AfterUID = after
	}

	if args.Alias == "shortest" {
		if v, ok := gq.Args["depth"]; ok {
			depth, err := strconv.ParseUint(v, 0, 64)
			if err != nil {
				return err
			}
			args.ExploreDepth = &depth
		}

		if v, ok := gq.Args["numpaths"]; ok {
			numPaths, err := strconv.ParseUint(v, 0, 64)
			if err != nil {
				return err
			}
			args.NumPaths = int(numPaths)
		}

		if v, ok := gq.Args["maxweight"]; ok {
			maxWeight, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
			args.MaxWeight = maxWeight
		} else if !ok {
			args.MaxWeight = math.MaxFloat64
		}

		if v, ok := gq.Args["minweight"]; ok {
			minWeight, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
			args.MinWeight = minWeight
		} else if !ok {
			args.MinWeight = -math.MaxFloat64
		}

		if gq.ShortestPathArgs.From == nil || gq.ShortestPathArgs.To == nil {
			return errors.Errorf("from/to can't be nil for shortest path")
		}
		if len(gq.ShortestPathArgs.From.UID) > 0 {
			args.From = gq.ShortestPathArgs.From.UID[0]
		}
		if len(gq.ShortestPathArgs.To.UID) > 0 {
			args.To = gq.ShortestPathArgs.To.UID[0]
		}
	}

	if v, ok := gq.Args["first"]; ok {
		first, err := strconv.ParseInt(v, 0, 32)
		if err != nil {
			return err
		}
		args.Count = int(first)
	}

	if v, ok := gq.Args["random"]; ok {
		random, err := strconv.ParseInt(v, 0, 32)
		if err != nil {
			return err
		}
		args.Random = int(random)
	}

	return nil
}

// ToSubGraph converts the GraphQuery into the pb.SubGraph instance type.
func ToSubGraph(ctx context.Context, gq *gql.GraphQuery) (*SubGraph, error) {
	sg, err := newGraph(ctx, gq)
	if err != nil {
		return nil, err
	}
	err = treeCopy(gq, sg)
	if err != nil {
		return nil, err
	}
	return sg, err
}

// ContextKey is used to set options in the context object.
type ContextKey int

const (
	// DebugKey is the key used to toggle debug mode.
	DebugKey ContextKey = iota
)

func isDebug(ctx context.Context) bool {
	var debug bool

	// gRPC client passes information about debug as metadata.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		// md is a map[string][]string
		if len(md["debug"]) > 0 {
			// We ignore the error here, because in error case,
			// debug would be false which is what we want.
			debug, _ = strconv.ParseBool(md["debug"][0])
		}
	}

	// HTTP passes information about debug as query parameter which is attached to context.
	d, _ := ctx.Value(DebugKey).(bool)
	return debug || d
}

func (sg *SubGraph) populate(uids []uint64) error {
	// Put sorted entries in matrix.
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	sg.uidMatrix = []*pb.List{{Uids: uids}}
	// User specified list may not be sorted.
	sg.SrcUIDs = &pb.List{Uids: uids}
	return nil
}

// newGraph returns the SubGraph and its task query.
func newGraph(ctx context.Context, gq *gql.GraphQuery) (*SubGraph, error) {
	// This would set the Result field in SubGraph,
	// and populate the children for attributes.

	// For the root, the name to be used in result is stored in Alias, not Attr.
	// The attr at root (if present) would stand for the source functions attr.
	args := params{
		Alias:            gq.Alias,
		Cascade:          &CascadeArgs{Fields: gq.Cascade},
		GetUid:           isDebug(ctx),
		IgnoreReflex:     gq.IgnoreReflex,
		IsEmpty:          gq.IsEmpty,
		Langs:            gq.Langs,
		NeedsVar:         append(gq.NeedsVar[:0:0], gq.NeedsVar...),
		Normalize:        gq.Normalize,
		Order:            gq.Order,
		ParentVars:       make(map[string]varValue),
		Recurse:          gq.Recurse,
		RecurseArgs:      gq.RecurseArgs,
		ShortestPathArgs: gq.ShortestPathArgs,
		Var:              gq.Var,
		GroupbyAttrs:     gq.GroupbyAttrs,
		IsGroupBy:        gq.IsGroupby,
		AllowedPreds:     gq.AllowedPreds,
	}

	// Remove pagination arguments from the query if @cascade is mentioned since
	// pagination will be applied post processing the data.
	if len(args.Cascade.Fields) > 0 {
		args.addCascadePaginationArguments(gq)
	}

	for argk := range gq.Args {
		if !isValidArg(argk) {
			return nil, errors.Errorf("Invalid argument: %s", argk)
		}
	}
	if err := args.fill(gq); err != nil {
		return nil, errors.Wrapf(err, "while filling args")
	}

	sg := &SubGraph{Params: args}

	if gq.Func != nil {
		// Uid function doesnt have Attr. It just has a list of ids
		if gq.Func.Attr != "uid" {
			sg.Attr = gq.Func.Attr
		} else {
			// Disallow uid as attribute - issue#3110
			if len(gq.Func.UID) == 0 {
				return nil, errors.Errorf(`Argument cannot be "uid"`)
			}
		}
		if !isValidFuncName(gq.Func.Name) {
			return nil, errors.Errorf("Invalid function name: %s", gq.Func.Name)
		}

		sg.createSrcFunction(gq.Func)
	}

	if isUidFnWithoutVar(gq.Func) && len(gq.UID) > 0 {
		if err := sg.populate(gq.UID); err != nil {
			return nil, errors.Wrapf(err, "while populating UIDs")
		}
	}

	// Copy roots filter.
	if gq.Filter != nil {
		sgf := &SubGraph{}
		if err := filterCopy(sgf, gq.Filter); err != nil {
			return nil, errors.Wrapf(err, "while copying filter")
		}
		sg.Filters = append(sg.Filters, sgf)
	}
	if gq.FacetsFilter != nil {
		facetsFilter, err := toFacetsFilter(gq.FacetsFilter)
		if err != nil {
			return nil, errors.Wrapf(err, "while converting to facets filter")
		}
		sg.facetsFilter = facetsFilter
	}
	return sg, nil
}

func toFacetsFilter(gft *gql.FilterTree) (*pb.FilterTree, error) {
	if gft == nil {
		return nil, nil
	}
	if gft.Func != nil && len(gft.Func.NeedsVar) != 0 {
		return nil, errors.Errorf("Variables not supported in pb.FilterTree")
	}
	ftree := &pb.FilterTree{Op: gft.Op}
	for _, gftc := range gft.Child {
		ftc, err := toFacetsFilter(gftc)
		if err != nil {
			return nil, err
		}
		ftree.Children = append(ftree.Children, ftc)
	}
	if gft.Func != nil {
		ftree.Func = &pb.Function{
			Key:  gft.Func.Attr,
			Name: gft.Func.Name,
		}
		// TODO(Janardhan): Handle variable in facets later.
		for _, arg := range gft.Func.Args {
			ftree.Func.Args = append(ftree.Func.Args, arg.Value)
		}
	}
	return ftree, nil
}

// createTaskQuery generates the query buffer.
func createTaskQuery(ctx context.Context, sg *SubGraph) (*pb.Query, error) {
	namespace, err := x.ExtractNamespace(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "While creating query task")
	}
	attr := sg.Attr
	// Might be safer than just checking first byte due to i18n
	reverse := strings.HasPrefix(attr, "~")
	if reverse {
		attr = strings.TrimPrefix(attr, "~")
	}
	var srcFunc *pb.SrcFunction
	if sg.SrcFunc != nil {
		srcFunc = &pb.SrcFunction{}
		srcFunc.Name = sg.SrcFunc.Name
		srcFunc.IsCount = sg.SrcFunc.IsCount
		for _, arg := range sg.SrcFunc.Args {
			srcFunc.Args = append(srcFunc.Args, arg.Value)
			if arg.IsValueVar {
				return nil, errors.Errorf("Unsupported use of value var")
			}
		}
	}

	// If the lang is set to *, query all the languages.
	if len(sg.Params.Langs) == 1 && sg.Params.Langs[0] == "*" {
		sg.Params.ExpandAll = true
	}

	// first is to limit how many results we want.
	first, offset := calculatePaginationParams(sg)

	out := &pb.Query{
		ReadTs:       sg.ReadTs,
		Cache:        int32(sg.Cache),
		Attr:         x.NamespaceAttr(namespace, attr),
		Langs:        sg.Params.Langs,
		Reverse:      reverse,
		SrcFunc:      srcFunc,
		AfterUid:     sg.Params.AfterUID,
		DoCount:      len(sg.Filters) == 0 && sg.Params.DoCount,
		FacetParam:   sg.Params.Facet,
		FacetsFilter: sg.facetsFilter,
		ExpandAll:    sg.Params.ExpandAll,
		First:        first,
		Offset:       offset,
	}

	// Use the orderedUIDs if present, it will only be present for the shortest path case.
	if sg.OrderedUIDs != nil {
		out.UidList = sg.OrderedUIDs
	} else if sg.SrcUIDs != nil {
		out.UidList = sg.SrcUIDs
	}
	return out, nil
}

// calculatePaginationParams returns the (count, offset) of result
// we need to proceed query further down.
func calculatePaginationParams(sg *SubGraph) (int32, int32) {
	// by default count is zero. (zero will retrieve all the results)
	count := math.MaxInt32
	// In order to limit we have to make sure that the this level met the following conditions
	// - No Filter (We can't filter until we have all the uids)
	// {
	//   q(func: has(name), first:1)@filter(eq(father, "schoolboy")) {
	//     name
	//     father
	//   }
	// }
	// - No Ordering (We need all the results to do the sorting)
	// {
	//   q(func: has(name), first:1, orderasc: name) {
	//     name
	//   }
	// }
	// - should not be one of those function which fetches some results and then do further
	// processing to narrow down the result. For example: allofterm will fetch the index postings
	// for each term and then do an intersection.
	// TODO: Look into how we can optimize queries involving these functions.

	shouldExclude := false
	if sg.SrcFunc != nil {
		switch sg.SrcFunc.Name {
		case "regexp", "alloftext", "allofterms", "match":
			shouldExclude = true
		default:
			shouldExclude = false
		}
	}

	if len(sg.Filters) == 0 && len(sg.Params.Order) == 0 && !shouldExclude {
		if sg.Params.Count != 0 {
			return int32(sg.Params.Count), int32(sg.Params.Offset)
		}
	}
	// make offset = 0, if there is need to fetch all the results.
	return int32(count), 0
}

// varValue is a generic representation of a variable and holds multiple things.
// TODO(pawan) - Come back to this and document what do individual fields mean and when are they
// populated.
type varValue struct {
	UidMap *sroar.Bitmap // list of uids if this denotes a uid variable.
	Vals   map[uint64]types.Val
	path   []*SubGraph // This stores the subgraph path from root to var definition.
	// strList stores the valueMatrix corresponding to a predicate and is later used in
	// expand(val(x)) query.
	strList     []*pb.ValueList
	OrderedUIDs *pb.List
}

func evalLevelAgg(
	doneVars map[string]varValue,
	sg, parent *SubGraph) (map[uint64]types.Val, error) {
	var mp map[uint64]types.Val

	if parent == nil {
		return nil, ErrWrongAgg
	}

	needsVar := sg.Params.NeedsVar[0].Name
	if parent.Params.IsEmpty {
		// The aggregated value doesn't really belong to a uid, we put it in UidToVal map
		// corresponding to uid 0 to avoid defining another field in SubGraph.
		vals := doneVars[needsVar].Vals

		ag := aggregator{
			name: sg.SrcFunc.Name,
		}
		for _, val := range vals {
			ag.Apply(val)
		}
		v, err := ag.Value()
		if err != nil && err != ErrEmptyVal {
			return nil, err
		}
		if v.Value != nil {
			mp = make(map[uint64]types.Val)
			mp[0] = v
		}
		return mp, nil
	}

	var relSG *SubGraph
	for _, ch := range parent.Children {
		if sg == ch {
			continue
		}
		for _, v := range ch.Params.FacetVar {
			if v == needsVar {
				relSG = ch
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
		return nil, errors.Errorf("Invalid variable aggregation. Check the levels.")
	}

	vals := doneVars[needsVar].Vals
	mp = make(map[uint64]types.Val)
	// Go over the sibling node and aggregate.
	for i, list := range relSG.uidMatrix {
		ag := aggregator{
			name: sg.SrcFunc.Name,
		}
		for _, uid := range list.Uids {
			if val, ok := vals[uid]; ok {
				ag.Apply(val)
			}
		}
		v, err := ag.Value()
		if err != nil && err != ErrEmptyVal {
			return nil, err
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

	if len(fromNode.Vals) == 0 {
		return fromNode.Vals, nil
	}

	newMap := fromNode.Vals
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
				return nil, errors.Errorf("Encountered non int/float type for summing")
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
func (sg *SubGraph) transformVars(doneVars map[string]varValue, path []*SubGraph) error {
	mNode := sg.MathExp
	mvarList := mNode.extractVarNodes()
	for i := 0; i < len(mvarList); i++ {
		mt := mvarList[i]
		curNode := doneVars[mt.Var]
		newMap, err := curNode.transformTo(path)
		if err != nil {
			return err
		}

		// This is the result of setting the result of count(uid) to a variable.
		// Treat this value as a constant.
		if val, ok := newMap[math.MaxUint64]; ok && len(newMap) == 1 {
			mt.Const = val
			continue
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

	switch {
	case sg.IsGroupBy():
		if err := sg.processGroupBy(doneVars, path); err != nil {
			return err
		}
	case sg.SrcFunc != nil && !parent.IsGroupBy() && isAggregatorFn(sg.SrcFunc.Name):
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
		sg.Params.UidToVal = mp
	case sg.MathExp != nil:
		// Preprocess to bring all variables to the same level.
		err := sg.transformVars(doneVars, path)
		if err != nil {
			return err
		}

		err = evalMathTree(sg.MathExp)
		if err != nil {
			return err
		}

		switch {
		case len(sg.MathExp.Val) != 0:
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
			sg.Params.UidToVal = sg.MathExp.Val
		case sg.MathExp.Const.Value != nil:
			// Assign the const for all the srcUids.
			mp := make(map[uint64]types.Val)
			rangeOver := sg.SrcUIDs
			if parent == nil {
				rangeOver = &pb.List{Uids: sg.DestMap.ToArray()}
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
			sg.Params.UidToVal = mp
		default:
			glog.V(3).Info("Warning: Math expression is using unassigned values or constants")
		}
		// Put it in this node.
	case len(sg.Params.NeedsVar) > 0:
		// This is a var() block.
		srcVar := sg.Params.NeedsVar[0]
		srcMap := doneVars[srcVar.Name]
		// The value var can be empty. No need to check for nil.
		sg.Params.UidToVal = srcMap.Vals
	case sg.Attr == "uid" && sg.Params.DoCount:
		// This is the count(uid) case.
		// We will do the computation later while constructing the result.
	default:
		return errors.Errorf("Unhandled pb.node <%v> with parent <%v>", sg.Attr, parent.Attr)
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

// Filters might have updated the destuids. facetMatrix should also be updated to exclude uids that
// were removed..
func (sg *SubGraph) updateFacetMatrix() {
	if len(sg.facetsMatrix) != len(sg.uidMatrix) {
		return
	}

	for lidx, l := range sg.uidMatrix {
		// For scalar predicates, uid list would be empty, we don't need to update facetsMatrix.
		// If its an uid predicate and uid list is empty then also we don't need to update
		// facetsMatrix, as results won't be returned to client in outputnode.go.
		if len(l.Uids) == 0 {
			continue
		}
		out := sg.facetsMatrix[lidx].FacetsList[:0]
		for idx, uid := range l.Uids {
			// If uid wasn't filtered then we keep the facet for it.
			if sg.DestMap.Contains(uid) {
				out = append(out, sg.facetsMatrix[lidx].FacetsList[idx])
			}
		}
		sg.facetsMatrix[lidx].FacetsList = out
	}
}

// updateUidMatrix is used to filter out the uids in uidMatrix which are not part of DestUIDs
// anymore. Some uids might have been removed from DestUids after application of filters,
// we remove them from the uidMatrix as well.
// If the query didn't specify sorting, we can just intersect the DestUids with lists in the
// uidMatrix since they are both sorted. Otherwise we must filter out the uids within the
// lists in uidMatrix which are not in DestUIDs.
func (sg *SubGraph) updateUidMatrix() {
	sg.updateFacetMatrix()
	for _, l := range sg.uidMatrix {
		if len(sg.Params.Order) > 0 || len(sg.Params.FacetsOrder) > 0 {
			// We can't do intersection directly as the list is not sorted by UIDs.
			// So do filter.
			algo.ApplyFilter(l, func(uid uint64, idx int) bool {
				return sg.DestMap.Contains(uid)
			})
		} else {
			// If we didn't order on UIDmatrix, it'll be sorted.
			// algo.IntersectWith(l, sg.DestUIDs, l)
			// TODO: Doing intersection with list would require conversion to Bitmap and conversion
			// back. Instead, just directly applyfilter like above.
			algo.ApplyFilter(l, func(uid uint64, idx int) bool {
				return sg.DestMap.Contains(uid)
			})
		}
	}
}

// populateVarMap stores the value of the variable defined in this SubGraph into req.Vars so that it
// is available to other queries as well. It is called after a query has been executed.
// TODO (pawan) - This function also transforms the DestUids and uidMatrix if the query is a cascade
// query which should probably happen before.
func (sg *SubGraph) populateVarMap(doneVars map[string]varValue, sgPath []*SubGraph) error {
	if sg.DestMap == nil || sg.IsGroupBy() {
		return nil
	}

	cascadeArgMap := make(map[string]bool)
	for _, pred := range sg.Params.Cascade.Fields {
		cascadeArgMap[pred] = true
	}
	cascadeAllPreds := cascadeArgMap["__all__"]

	if sg.Params.Alias == "shortest" {
		return sg.updateVars(doneVars, sgPath)
	}

	if len(sg.Filters) > 0 {
		sg.updateUidMatrix()
	}

	for _, child := range sg.Children {
		sgPath = append(sgPath, sg) // Add the current node to path
		if err := child.populateVarMap(doneVars, sgPath); err != nil {
			return err
		}
		sgPath = sgPath[:len(sgPath)-1] // Backtrack
		if len(child.Params.Cascade.Fields) == 0 {
			continue
		}

		// Intersect the UidMatrix with the DestUids as some UIDs might have been removed
		// by other operations. So we need to apply it on the UidMatrix.
		child.updateUidMatrix()

		// Apply pagination after the @cascade.
		if len(child.Params.Cascade.Fields) > 0 && (child.Params.Cascade.First != 0 || child.Params.Cascade.Offset != 0) {
			for i := 0; i < len(child.uidMatrix); i++ {
				start, end := x.PageRange(child.Params.Cascade.First, child.Params.Cascade.Offset, len(child.uidMatrix[i].Uids))
				child.uidMatrix[i].Uids = child.uidMatrix[i].Uids[start:end]
			}
		}
	}

	if len(sg.Params.Cascade.Fields) == 0 {
		return sg.updateVars(doneVars, sgPath)
	}

	// Filter out UIDs that don't have atleast one UID in every child.
	itr := sg.DestMap.NewIterator()
	out := sg.DestMap.Clone()
	for i := 0; itr.HasNext(); i++ {
		uid := itr.Next()
		var exclude bool
		for _, child := range sg.Children {
			// For uid we dont actually populate the uidMatrix or values. So a node asking for
			// uid would always be excluded. Therefore we skip it.
			if child.Attr == "uid" {
				continue
			}

			// If the length of child UID list is zero and it has no valid value, then the
			// current UID should be removed from this level.
			if (cascadeAllPreds || cascadeArgMap[child.Attr]) &&
				!child.IsInternal() &&
				// Check len before accessing index.
				(len(child.valueMatrix) <= i || len(child.valueMatrix[i].Values) == 0) &&
				(len(child.counts) <= i) &&
				(len(child.uidMatrix) <= i || len(child.uidMatrix[i].Uids) == 0) {
				exclude = true
				break
			}
		}
		if exclude {
			out.Remove(uid)
		}
	}
	// Note the we can't overwrite DestUids, as it'd also modify the SrcUids of
	// next level and the mapping from SrcUids to uidMatrix would be lost.
	// TODO: Consider if we should have such a tight coupling with SrcUids and DestMap. Better to do
	// a copy on write arrangement.
	sg.DestMap = out
	return sg.updateVars(doneVars, sgPath)
}

// updateVars is used to update the doneVars map with the value of the variable from the SubGraph.
// The variable could be a uid or a value variable.
// It is called twice
// 1. To populate sg.Params.ParentVars map with the value of a variable to pass down to children
// subgraphs in a query.
// 2. To populate req.Vars, which is used by other queries requiring variables..
func (sg *SubGraph) updateVars(doneVars map[string]varValue, sgPath []*SubGraph) error {
	// NOTE: although we initialize doneVars (req.Vars) in ProcessQuery, this nil check is for
	// non-root lookups that happen to other nodes. Don't use len(doneVars) == 0 !
	if doneVars == nil || (sg.Params.Var == "" && sg.Params.FacetVar == nil) {
		return nil
	}

	sgPathCopy := append(sgPath[:0:0], sgPath...)
	if err := sg.populateUidValVar(doneVars, sgPathCopy); err != nil {
		return err
	}
	return sg.populateFacetVars(doneVars, sgPathCopy)
}

// populateUidValVar populates the value of the variable into doneVars.
func (sg *SubGraph) populateUidValVar(doneVars map[string]varValue, sgPath []*SubGraph) error {
	if sg.Params.Var == "" {
		return nil
	}

	var v varValue
	var ok bool

	switch {
	case len(sg.counts) > 0:
		// 1. When count of a predicate is assigned a variable, we store the mapping of uid =>
		// count(predicate).

		// This implies it is a value variable.
		doneVars[sg.Params.Var] = varValue{
			OrderedUIDs: sg.OrderedUIDs,
			Vals:        make(map[uint64]types.Val),
			path:        sgPath,
			strList:     sg.valueMatrix,
		}
		for idx, uid := range sg.SrcUIDs.Uids {
			val := types.Val{
				Tid:   types.IntID,
				Value: int64(sg.counts[idx]),
			}
			doneVars[sg.Params.Var].Vals[uid] = val
		}
	case sg.Params.DoCount && sg.Attr == "uid" && sg.IsInternal():
		// 2. This is the case where count(uid) is requested in the query and stored as variable.
		// In this case there is just one value which is stored corresponding to the uid
		// math.MaxUint64 which isn't entirely correct as there could be an actual uid with that
		// value.
		doneVars[sg.Params.Var] = varValue{
			OrderedUIDs: sg.OrderedUIDs,
			Vals:        make(map[uint64]types.Val),
			path:        sgPath,
			strList:     sg.valueMatrix,
		}

		// Because we are counting the number of UIDs in parent
		// we use the length of SrcUIDs instead of DestUIDs.
		val := types.Val{
			Tid:   types.IntID,
			Value: int64(len(sg.SrcUIDs.Uids)),
		}
		doneVars[sg.Params.Var].Vals[math.MaxUint64] = val
	case !sg.DestMap.IsEmpty() || (sg.Attr == "uid" && sg.SrcUIDs != nil):
		// 3. A uid variable. The variable could be defined in one of two places.
		// a) Either on the actual predicate.
		//    me(func: (...)) {
		//      a as friend
		//    }
		//
		// b) Or on the uid edge
		//    me(func:(...)) {
		//      friend {
		//        a as uid
		//      }
		//    }

		// Uid variable could be defined using uid or a predicate.
		var uids *sroar.Bitmap
		if sg.Attr == "uid" {
			uids = codec.FromList(sg.SrcUIDs)
		} else {
			// Avoid an upfront Clone.
			// TODO(Ahsan): See if we want to implement this function.
			// sg.DestMap.SetCopyOnWrite(true)
			uids = sg.DestMap
		}

		if v, ok = doneVars[sg.Params.Var]; !ok {
			doneVars[sg.Params.Var] = varValue{
				OrderedUIDs: sg.OrderedUIDs,
				UidMap:      uids,
				path:        sgPath,
				Vals:        make(map[uint64]types.Val),
				strList:     sg.valueMatrix,
			}
			return nil
		}
		// For a recurse query this can happen. We don't allow using the same variable more than
		// once otherwise.
		v.UidMap.Or(uids)
		doneVars[sg.Params.Var] = v
	case len(sg.valueMatrix) != 0 && sg.SrcUIDs != nil && len(sgPath) != 0:
		// 4. A value variable. We get the first value from every list thats part of ValueMatrix
		// and store it corresponding to a uid in SrcUIDs.
		if v, ok = doneVars[sg.Params.Var]; !ok {
			v.Vals = make(map[uint64]types.Val)
			v.path = sgPath
			v.strList = sg.valueMatrix
		}

		for idx, uid := range sg.SrcUIDs.Uids {
			if len(sg.valueMatrix[idx].Values) > 1 {
				return errors.Errorf("Value variables not supported for predicate with list type.")
			}

			if len(sg.valueMatrix[idx].Values) == 0 {
				continue
			}
			val, err := convertWithBestEffort(sg.valueMatrix[idx].Values[0], sg.Attr)
			if err != nil {
				continue
			}
			v.Vals[uid] = val
		}
		doneVars[sg.Params.Var] = v
	default:
		// If the variable already existed and now we see it again without any DestUIDs or
		// ValueMatrix then lets just return.
		if _, ok := doneVars[sg.Params.Var]; ok {
			return nil
		}
		// Insert a empty entry to keep the dependency happy.
		doneVars[sg.Params.Var] = varValue{
			OrderedUIDs: sg.OrderedUIDs,
			path:        sgPath,
			Vals:        make(map[uint64]types.Val),
			strList:     sg.valueMatrix,
			UidMap:      sroar.NewBitmap(),
		}
	}
	return nil
}

// populateFacetVars walks the facetsMatrix to compute the value of a facet variable.
// It sums up the value for float/int type facets so that there is only variable corresponding
// to each uid in the uidMatrix.
func (sg *SubGraph) populateFacetVars(doneVars map[string]varValue, sgPath []*SubGraph) error {
	if len(sg.Params.FacetVar) == 0 || sg.Params.Facet == nil {
		return nil
	}

	sgPath = append(sgPath, sg)
	for _, it := range sg.Params.Facet.Param {
		fvar, ok := sg.Params.FacetVar[it.Key]
		if !ok {
			continue
		}
		// Assign an empty value for every facet that was assigned to a variable and hence is part
		// of FacetVar.
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
				if !ok {
					continue
				}
				if pVal, ok := doneVars[fvar].Vals[uid]; !ok {
					fVal, err := facets.ValFor(f)
					if err != nil {
						return err
					}

					doneVars[fvar].Vals[uid] = fVal
				} else {
					// If the value is int/float we add them up. Else we throw an error as
					// many to one maps are not allowed for other types.
					nVal, err := facets.ValFor(f)
					if err != nil {
						return err
					}

					if nVal.Tid != types.IntID && nVal.Tid != types.FloatID {
						return errors.Errorf("Repeated id with non int/float value for " +
							"facet var encountered.")
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
	return nil
}

// recursiveFillVars fills the value of variables before a query is to be processed using the result
// of the values (doneVars) computed by other queries that were successfully run before this query.
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

// fillShortestPathVars reads value of the uid variable from mp map and fills it into From and To
// parameters.
func (sg *SubGraph) fillShortestPathVars(mp map[string]varValue) error {
	// The uidVar.Uids can be nil or have an empty uid list if the variable didn't
	// return any uids. This would mean sg.Params.From or sg.Params.To is 0 and the
	// query would return an empty result.

	if sg.Params.ShortestPathArgs.From != nil && len(sg.Params.ShortestPathArgs.From.NeedsVar) > 0 {
		fromVar := sg.Params.ShortestPathArgs.From.NeedsVar[0].Name
		uidVar, ok := mp[fromVar]
		if !ok {
			return errors.Errorf("value of from var(%s) should have already been populated",
				fromVar)
		}
		if !uidVar.UidMap.IsEmpty() {
			if uidVar.UidMap.GetCardinality() > 1 {
				return errors.Errorf("from variable(%s) should only expand to 1 uid", fromVar)
			}
			sg.Params.From = uidVar.UidMap.Minimum()
		}
	}

	if sg.Params.ShortestPathArgs.To != nil && len(sg.Params.ShortestPathArgs.To.NeedsVar) > 0 {
		toVar := sg.Params.ShortestPathArgs.To.NeedsVar[0].Name
		uidVar, ok := mp[toVar]
		if !ok {
			return errors.Errorf("value of to var(%s) should have already been populated",
				toVar)
		}
		if !uidVar.UidMap.IsEmpty() {
			if uidVar.UidMap.GetCardinality() > 1 {
				return errors.Errorf("to variable(%s) should only expand to 1 uid", toVar)
			}
			sg.Params.To = uidVar.UidMap.Minimum()
		}
	}
	return nil
}

// fillVars reads the value corresponding to a variable from the map mp and stores it inside
// SubGraph. This value is then later used for execution of the SubGraph.
func (sg *SubGraph) fillVars(mp map[string]varValue) error {
	if sg.Params.Alias == "shortest" {
		if err := sg.fillShortestPathVars(mp); err != nil {
			return err
		}
	}

	out := sroar.NewBitmap()
	// Go through all the variables in NeedsVar and see if we have a value for them in the map. If
	// we do, then we store that value in the appropriate variable inside SubGraph.
	for _, v := range sg.Params.NeedsVar {
		l, ok := mp[v.Name]
		if !ok {
			continue
		}
		switch {
		case (v.Typ == gql.AnyVar || v.Typ == gql.ListVar) && l.strList != nil:
			// This is for the case when we use expand(val(x)) with a value variable.
			// We populate the list of values into ExpandPreds and use that for the expand query
			// later.
			// TODO: If we support value vars for list type then this needn't be true
			sg.ExpandPreds = l.strList

		case (v.Typ == gql.UidVar && sg.SrcFunc != nil && sg.SrcFunc.Name == "uid_in"):
			srcFuncArgs := sg.SrcFunc.Args[:0]
			itr := l.UidMap.NewIterator()
			for itr.HasNext() {
				uid := itr.Next()
				// We use base 10 here because the uid parser expects the uid to be in base 10.
				arg := gql.Arg{Value: strconv.FormatUint(uid, 10)}
				srcFuncArgs = append(srcFuncArgs, arg)
			}
			sg.SrcFunc.Args = srcFuncArgs

		case (v.Typ == gql.AnyVar || v.Typ == gql.UidVar) && !l.UidMap.IsEmpty():
			if l.OrderedUIDs != nil {
				// TODO(Ahsan): There should only be one shortest path block in a query. So, we can
				// assume the below assertion to hold. Need to double-check this.
				x.AssertTrue(sg.OrderedUIDs == nil)
				sg.OrderedUIDs = l.OrderedUIDs
			}
			out.Or(l.UidMap)

		case (v.Typ == gql.AnyVar || v.Typ == gql.ValueVar):
			// This should happen only once.
			// TODO: This allows only one value var per subgraph, change it later
			sg.Params.UidToVal = l.Vals

		case (v.Typ == gql.AnyVar || v.Typ == gql.UidVar) && len(l.Vals) != 0:
			// Derive the UID list from value var.
			for k := range l.Vals {
				out.Set(k)
			}

		case len(l.Vals) != 0 || !l.UidMap.IsEmpty():
			return errors.Errorf("Wrong variable type encountered for var(%v) %v.", v.Name, v.Typ)

		default:
			glog.V(3).Infof("Warning: reached default case in fillVars for var: %v", v.Name)
		}
	}
	if err := sg.replaceVarInFunc(); err != nil {
		return err
	}

	if sg.DestMap != nil {
		// Don't add sg.DestUIDs in case its size is 0.
		// This is to avoiding adding nil (empty element) to lists.
		out.Or(sg.DestMap)
	}

	sg.DestMap = out
	return nil
}

// replaceVarInFunc gets values stored inside UidToVal(coming from a value variable defined in some
// other query) and adds them as arguments to the SrcFunc in SubGraph.
// E.g. - func: eq(score, val(myscore))
// NOTE - We disallow vars in facets filter so we don't need to worry about that as of now.
func (sg *SubGraph) replaceVarInFunc() error {
	if sg.SrcFunc == nil {
		return nil
	}
	var args []gql.Arg
	// Iterate over the args and replace value args with their values
	for _, arg := range sg.SrcFunc.Args {
		if !arg.IsValueVar {
			args = append(args, arg)
			continue
		}
		if len(sg.Params.UidToVal) == 0 {
			// This means that the variable didn't have any values and hence there is nothing to add
			// to args.
			break
		}
		// We don't care about uids, just take all the values and put as args.
		// There would be only one value var per subgraph as per current assumptions.
		seenArgs := make(map[string]struct{})
		for _, v := range sg.Params.UidToVal {
			data := types.ValueForType(types.StringID)
			if err := types.Marshal(v, &data); err != nil {
				return err
			}
			val := data.Value.(string)
			if _, ok := seenArgs[val]; ok {
				continue
			}
			seenArgs[val] = struct{}{}
			args = append(args, gql.Arg{Value: val})
		}
	}
	sg.SrcFunc.Args = args
	return nil
}

// Used to evaluate an inequality function which uses a value variable instead of a predicate.
// E.g.
// 1. func: eq(val(x), 35) or @filter(eq(val(x), 35)
// 2. func: ge(val(x), 40) or @filter(ge(val(x), 40)
// ... other inequality functions
// The function filters uids corresponding to the variable which satisfy the inequality and stores
// the filtered uids in DestUIDs.
func (sg *SubGraph) applyIneqFunc() error {
	if len(sg.Params.UidToVal) == 0 {
		// Expected a valid value map. But got empty.
		// Don't return error, return empty - issue #2610
		return nil
	}

	// A mapping of uid to their value should have already been stored in UidToVal.
	// Find out the type of value using the first value in the map and try to convert the function
	// argument to that type to make sure we can compare them. If we can't return an error.
	var typ types.TypeID
	for _, v := range sg.Params.UidToVal {
		typ = v.Tid
		break
	}
	val := sg.SrcFunc.Args[0].Value
	src := types.Val{Tid: types.StringID, Value: []byte(val)}
	dst, err := types.Convert(src, typ)
	if err != nil {
		return errors.Errorf("Invalid argment %v. Comparing with different type", val)
	}

	if sg.SrcUIDs != nil {
		// This means its a filter.
		for _, uid := range sg.SrcUIDs.Uids {
			curVal, ok := sg.Params.UidToVal[uid]
			if ok && types.CompareVals(sg.SrcFunc.Name, curVal, dst) {
				sg.DestMap.Set(uid)
			}
		}
	} else {
		// This means it's a function at root as SrcUIDs is nil
		for uid, curVal := range sg.Params.UidToVal {
			if types.CompareVals(sg.SrcFunc.Name, curVal, dst) {
				sg.DestMap.Set(uid)
			}
		}
		sg.uidMatrix = []*pb.List{codec.ToList(sg.DestMap)}
	}
	return nil
}

func (sg *SubGraph) appendDummyValues() {
	if sg.SrcUIDs == nil || len(sg.SrcUIDs.Uids) == 0 {
		return
	}
	var l pb.List
	var val pb.ValueList
	for range sg.SrcUIDs.Uids {
		// This is necessary so that preTraverse can be processed smoothly.
		sg.uidMatrix = append(sg.uidMatrix, &l)
		sg.valueMatrix = append(sg.valueMatrix, &val)
	}
}

func getPredsFromVals(vl []*pb.ValueList) []string {
	preds := make([]string, 0)
	for _, l := range vl {
		for _, v := range l.Values {
			if len(v.Val) > 0 {
				preds = append(preds, string(v.Val))
			}
		}
	}
	return preds
}

func uniquePreds(list []string) []string {
	predMap := make(map[string]struct{})
	for _, item := range list {
		predMap[item] = struct{}{}
	}

	preds := make([]string, 0, len(predMap))
	for pred := range predMap {
		preds = append(preds, pred)
	}
	return preds
}

func recursiveCopy(dst *SubGraph, src *SubGraph) {
	dst.Attr = src.Attr
	dst.Params = src.Params
	dst.Params.ParentVars = make(map[string]varValue)
	for k, v := range src.Params.ParentVars {
		dst.Params.ParentVars[k] = v
	}

	dst.copyFiltersRecurse(src)
	dst.ReadTs = src.ReadTs

	for _, c := range src.Children {
		copyChild := new(SubGraph)
		recursiveCopy(copyChild, c)
		dst.Children = append(dst.Children, copyChild)
	}
}

func expandSubgraph(ctx context.Context, sg *SubGraph) ([]*SubGraph, error) {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "expandSubgraph: "+sg.Attr)
	defer stop()

	namespace, err := x.ExtractNamespace(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "While expanding subgraph")
	}
	out := make([]*SubGraph, 0, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]

		if child.Params.Expand == "" {
			out = append(out, child)
			continue
		}

		var preds []string
		typeNames, err := getNodeTypes(ctx, sg)
		if err != nil {
			return out, err
		}

		switch child.Params.Expand {
		// It could be expand(_all_) or expand(val(x)).
		case "_all_":
			span.Annotate(nil, "expand(_all_)")
			if len(typeNames) == 0 {
				break
			}

			preds = getPredicatesFromTypes(namespace, typeNames)
			// We check if enterprise is enabled and only
			// restrict preds to allowed preds if ACL is turned on.
			if worker.EnterpriseEnabled() && sg.Params.AllowedPreds != nil {
				// Take intersection of both the predicate lists
				intersectPreds := make([]string, 0)
				hashMap := make(map[string]bool)
				for _, allowedPred := range sg.Params.AllowedPreds {
					hashMap[allowedPred] = true
				}
				for _, pred := range preds {
					if _, found := hashMap[pred]; found {
						intersectPreds = append(intersectPreds, pred)
					}
				}
				preds = intersectPreds
			}

		default:
			if len(child.ExpandPreds) > 0 {
				span.Annotate(nil, "expand default")
				// We already have the predicates populated from the var.
				temp := getPredsFromVals(child.ExpandPreds)
				for _, pred := range temp {
					preds = append(preds, x.NamespaceAttr(namespace, pred))
				}
			} else {
				typeNames := strings.Split(child.Params.Expand, ",")
				preds = getPredicatesFromTypes(namespace, typeNames)
			}
		}
		preds = uniquePreds(preds)

		// There's a types filter at this level so filter out any non-uid predicates
		// since only uid nodes can have a type.
		if len(child.Filters) > 0 {
			preds, err = filterUidPredicates(ctx, preds)
			if err != nil {
				return out, err
			}
		}

		for _, pred := range preds {
			// Convert attribute name for the given namespace.
			temp := &SubGraph{
				ReadTs: sg.ReadTs,
				Attr:   x.ParseAttr(pred),
			}
			temp.Params = child.Params
			// TODO(martinmr): simplify this condition once _reverse_ and _forward_
			// are removed
			temp.Params.ExpandAll = child.Params.Expand != "_reverse_" &&
				child.Params.Expand != "_forward_"
			temp.Params.ParentVars = make(map[string]varValue)
			for k, v := range child.Params.ParentVars {
				temp.Params.ParentVars[k] = v
			}
			temp.Params.IsInternal = false
			temp.Params.Expand = ""
			temp.Params.Facet = &pb.FacetParams{AllKeys: true}
			for _, cf := range child.Filters {
				s := &SubGraph{}
				recursiveCopy(s, cf)
				temp.Filters = append(temp.Filters, s)
			}

			// Go through each child, create a copy and attach to temp.Children.
			for _, cc := range child.Children {
				s := &SubGraph{}
				recursiveCopy(s, cc)
				temp.Children = append(temp.Children, s)
			}

			for _, ch := range sg.Children {
				if ch.isSimilar(temp) {
					return out, errors.Errorf("Repeated subgraph: [%s] while using expand()",
						ch.Attr)
				}
			}
			out = append(out, temp)
		}
	}
	return out, nil
}

// ProcessGraph processes the SubGraph instance accumulating result for the query
// from different instances. Note: taskQuery is nil for root node.
func ProcessGraph(ctx context.Context, sg, parent *SubGraph, rch chan error) {
	var suffix string
	if len(sg.Params.Alias) > 0 {
		suffix += "." + sg.Params.Alias
	}
	if len(sg.Attr) > 0 {
		suffix += "." + sg.Attr
	}
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "query.ProcessGraph"+suffix)
	defer stop()

	if sg.Attr == "uid" {
		// We dont need to call ProcessGraph for uid, as we already have uids
		// populated from parent and there is nothing to process but uidMatrix
		// and values need to have the right sizes so that preTraverse works.
		sg.appendDummyValues()
		rch <- nil
		return
	}
	var err error
	switch {
	case parent == nil && sg.SrcFunc != nil && sg.SrcFunc.Name == "uid":
		// I'm root and I'm using some variable that has been populated.
		// Retain the actual order in uidMatrix. But sort the destUids.
		if sg.SrcUIDs != nil && len(sg.SrcUIDs.Uids) != 0 {
			// I am root. I don't have any function to execute, and my
			// result has been prepared for me already by list passed by the user.
			// uidmatrix retains the order. SrcUids are sorted (in newGraph).
			sg.DestMap = codec.FromList(sg.SrcUIDs)
		} else {
			// Populated variable.
			if sg.OrderedUIDs != nil {
				sg.uidMatrix = []*pb.List{sg.OrderedUIDs}
			} else {
				sg.uidMatrix = []*pb.List{codec.ToList(sg.DestMap)}
			}
		}
		if sg.Params.AfterUID > 0 {
			sg.DestMap.RemoveRange(0, sg.Params.AfterUID+1)
		}

	case sg.Attr == "":
		// This is when we have uid function in children.
		if sg.SrcFunc != nil && sg.SrcFunc.Name == "uid" {
			// If its a uid() filter, we just have to intersect the SrcUIDs with DestUIDs
			// and return.
			if err := sg.fillVars(sg.Params.ParentVars); err != nil {
				rch <- err
				return
			}
			codec.And(sg.DestMap, sg.SrcUIDs)
			rch <- nil
			return
		}

		if sg.SrcUIDs == nil {
			glog.Errorf("SrcUIDs is unexpectedly nil. Subgraph: %+v", sg)
			rch <- errors.Errorf("SrcUIDs shouldn't be nil.")
			return
		}
		// If we have a filter SubGraph which only contains an operator,
		// it won't have any attribute to work on.
		// This is to allow providing SrcUIDs to the filter children.
		// Each filter use it's own (shallow) copy of SrcUIDs, so there is no race conditions,
		// when multiple filters replace their sg.DestUIDs
		sg.DestMap = codec.FromList(sg.SrcUIDs)
	default:
		isInequalityFn := sg.SrcFunc != nil && isInequalityFn(sg.SrcFunc.Name)
		switch {
		case isInequalityFn && sg.SrcFunc.IsValueVar:
			// This is a ineq function which uses a value variable.
			err = sg.applyIneqFunc()
			if parent != nil {
				rch <- err
				return
			}
		case isInequalityFn && sg.SrcFunc.IsLenVar:
			// Safe to access 0th element here because if no variable was given, parser would throw
			// an error.
			val := sg.SrcFunc.Args[0].Value
			src := types.Val{Tid: types.StringID, Value: []byte(val)}
			dst, err := types.Convert(src, types.IntID)
			if err != nil {
				// TODO(Aman): needs to do parent check?
				rch <- errors.Wrapf(err, "invalid argument %v. Comparing with different type", val)
				return
			}

			curVal := types.Val{Tid: types.IntID, Value: int64(sg.DestMap.GetCardinality())}
			if types.CompareVals(sg.SrcFunc.Name, curVal, dst) {
				sg.DestMap = codec.FromList(sg.SrcUIDs)
			} else {
				sg.DestMap = nil
			}
		default:
			taskQuery, err := createTaskQuery(ctx, sg)
			if err != nil {
				rch <- err
				return
			}
			result, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
			switch {
			case err != nil && strings.Contains(err.Error(), worker.ErrNonExistentTabletMessage):
				sg.UnknownAttr = true
				// Create an empty result because the code below depends on it.
				result = &pb.Result{}
			case err != nil:
				rch <- err
				return
			}

			sg.uidMatrix = result.UidMatrix
			sg.valueMatrix = result.ValueMatrix
			sg.facetsMatrix = result.FacetMatrix
			sg.counts = result.Counts
			sg.LangTags = result.LangMatrix
			sg.List = result.List

			if sg.Params.DoCount {
				if len(sg.Filters) == 0 {
					// If there is a filter, we need to do more work to get the actual count.
					rch <- nil
					return
				}
				sg.counts = make([]uint32, len(sg.uidMatrix))
			}

			if result.IntersectDest {
				sg.DestMap = codec.Intersect(result.UidMatrix)
			} else {
				sg.DestMap = codec.Merge(result.UidMatrix)
			}

			if parent == nil {
				// I'm root. We reach here if root had a function.
				sg.uidMatrix = []*pb.List{codec.ToList(sg.DestMap)}
			}
		}
	}

	// Run filters if any.
	if len(sg.Filters) > 0 {
		// Run all filters in parallel.
		filterChan := make(chan error, len(sg.Filters))
		for _, filter := range sg.Filters {
			isUidFuncWithoutVar := filter.SrcFunc != nil && filter.SrcFunc.Name == "uid" &&
				len(filter.Params.NeedsVar) == 0
			// For uid function filter, no need for processing. User already gave us the
			// list. Lets just update DestUIDs.
			if isUidFuncWithoutVar {
				filter.DestMap = codec.FromList(filter.SrcUIDs)
				filterChan <- nil
				continue
			}

			filter.SrcUIDs = codec.ToList(sg.DestMap)
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
			}
		}

		if filterErr != nil {
			rch <- filterErr
			return
		}

		hasNils := false
		// Now apply the results from filter.
		var bitmaps []*sroar.Bitmap
		for _, filter := range sg.Filters {
			if filter.DestMap == nil {
				hasNils = true
				break
			}
			bitmaps = append(bitmaps, filter.DestMap)
		}

		switch {
		case sg.FilterOp == "or":
			sg.DestMap = sroar.FastParOr(4, bitmaps...)
		case sg.FilterOp == "not":
			x.AssertTrue(len(sg.Filters) == 1)
			if sg.Filters[0].DestMap == nil {
				sg.DestMap.AndNot(sroar.NewBitmap())
			} else {
				sg.DestMap.AndNot(sg.Filters[0].DestMap)
			}
		case sg.FilterOp == "and":
			if hasNils {
				sg.DestMap = sroar.NewBitmap()
			} else {
				sg.DestMap = sroar.FastAnd(bitmaps...)
			}
		default:
			// We need to also intersect the original dest uids in this case to get the final
			// DestUIDs.
			// me(func: eq(key, "key1")) @filter(eq(key, "key2"))

			// TODO - See if the server performing the filter can intersect with the srcUIDs before
			// returning them in this case.
			if hasNils {
				sg.DestMap = sroar.NewBitmap()
			} else {
				r := sroar.FastAnd(bitmaps...)
				sg.DestMap.And(r)
			}
		}
	}

	if len(sg.Params.Order) == 0 && len(sg.Params.FacetsOrder) == 0 {
		// for `has` function when there is no filtering and ordering, we fetch
		// correct paginated results so no need to apply pagination here.
		if !(len(sg.Filters) == 0 && sg.SrcFunc != nil && sg.SrcFunc.Name == "has") {
			// There is no ordering. Just apply pagination and return.
			if err = sg.applyPagination(ctx); err != nil {
				rch <- err
				return
			}
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

	if sg.Params.Random > 0 {
		if err = sg.applyRandom(ctx); err != nil {
			rch <- err
			return
		}
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
		sg.updateUidMatrix()
		for i, ul := range sg.uidMatrix {
			// A possible optimization is to return the size of the intersection
			// without forming the intersection.
			sg.counts[i] = uint32(len(ul.Uids))
		}
		rch <- nil
		return
	}

	if sg.Children, err = expandSubgraph(ctx, sg); err != nil {
		rch <- err
		return
	}

	if sg.IsGroupBy() {
		// Add the attrs required by groupby nodes
		for _, it := range sg.Params.GroupbyAttrs {
			// TODO - Throw error if Attr is of list type.
			sg.Children = append(sg.Children, &SubGraph{
				Attr:   it.Attr,
				ReadTs: sg.ReadTs,
				Params: params{
					Alias:        it.Alias,
					IgnoreResult: true,
					Langs:        it.Langs,
				},
			})
		}
	}

	if len(sg.Children) > 0 {
		// We store any variable defined by this node in the map and pass it on
		// to the children which might depend on it. We only need to do this if the SubGraph
		// has children.
		if err = sg.updateVars(sg.Params.ParentVars, []*SubGraph{}); err != nil {
			rch <- err
			return
		}
	}

	childChan := make(chan error, len(sg.Children))
	for i := 0; i < len(sg.Children); i++ {
		child := sg.Children[i]
		child.Params.ParentVars = make(map[string]varValue)
		for k, v := range sg.Params.ParentVars {
			child.Params.ParentVars[k] = v
		}

		child.SrcUIDs = codec.ToList(sg.DestMap) // Make the connection.
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
		}
	}

	if sg.DestMap == nil || sg.DestMap.GetCardinality() == 0 {
		// Looks like we're done here. Be careful with nil srcUIDs!
		if span != nil {
			span.Annotatef(nil, "Zero uids for %q", sg.Attr)
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

	rch <- childErr
}

// stores index of a uid as the index in the uidMatrix (x)
// and index in the corresponding list of the uidMatrix (y)
type UidKey struct {
	x int
	y int
}

// applies "random" to lists inside uidMatrix
// sg.Params.Random number of nodes are selected in each uid list
// duplicates are avoided (random selection without replacement)
// if sg.Params.Random is more than the number of available nodes
// all nodes are returned
func (sg *SubGraph) applyRandom(ctx context.Context) error {
	sg.updateUidMatrix()

	for i := 0; i < len(sg.uidMatrix); i++ {
		// shuffle the uid list and select the
		// first sg.Params.Random uids

		uidList := sg.uidMatrix[i].Uids

		rand.Shuffle(len(uidList), func(i, j int) {
			uidList[i], uidList[j] = uidList[j], uidList[i]
		})

		numRandom := sg.Params.Random
		if sg.Params.Random > len(uidList) {
			numRandom = len(uidList)
		}

		sg.uidMatrix[i].Uids = uidList[:numRandom]
	}

	sg.DestMap = codec.Merge(sg.uidMatrix)
	return nil
}

// applyPagination applies count and offset to lists inside uidMatrix.
func (sg *SubGraph) applyPagination(ctx context.Context) error {
	if sg.Params.Count == 0 && sg.Params.Offset == 0 { // No pagination.
		return nil
	}

	sg.updateUidMatrix()
	for i := 0; i < len(sg.uidMatrix); i++ {
		// Apply the offsets.
		start, end := x.PageRange(sg.Params.Count, sg.Params.Offset, len(sg.uidMatrix[i].Uids))
		sg.uidMatrix[i].Uids = sg.uidMatrix[i].Uids[start:end]
	}
	// Re-merge the UID matrix.
	sg.DestMap = codec.Merge(sg.uidMatrix)
	return nil
}

// applyOrderAndPagination orders each posting list by a given attribute
// before applying pagination.
func (sg *SubGraph) applyOrderAndPagination(ctx context.Context) error {
	if len(sg.Params.Order) == 0 && len(sg.Params.FacetsOrder) == 0 {
		return nil
	}

	sg.updateUidMatrix()

	// See if we need to apply order based on facet.
	if len(sg.Params.FacetsOrder) != 0 {
		return sg.sortAndPaginateUsingFacet(ctx)
	}

	for _, it := range sg.Params.NeedsVar {
		// TODO(pawan) - Return error if user uses var order with predicates.
		if len(sg.Params.Order) > 0 && it.Name == sg.Params.Order[0].Attr &&
			(it.Typ == gql.ValueVar) {
			// If the Order name is same as var name and it's a value variable, we sort using that variable.
			return sg.sortAndPaginateUsingVar(ctx)
		}
	}

	// Todo: fix offset for cascade queries.
	if sg.Params.Count == 0 {
		// Only retrieve up to 1000 results by default.
		sg.Params.Count = 1000
	}

	x.AssertTrue(len(sg.Params.Order) > 0)

	ns, err := x.ExtractNamespace(ctx)
	if err != nil {
		return errors.Wrapf(err, "While ordering and paginating")
	}
	order := sg.createOrderForTask(ns)
	sortMsg := &pb.SortMessage{
		Order:     order,
		UidMatrix: sg.uidMatrix,
		Offset:    int32(sg.Params.Offset),
		Count:     int32(sg.Params.Count),
		ReadTs:    sg.ReadTs,
	}
	result, err := worker.SortOverNetwork(ctx, sortMsg)
	if err != nil {
		return err
	}

	x.AssertTrue(len(result.UidMatrix) == len(sg.uidMatrix))
	if sg.facetsMatrix != nil {
		// The order of uids in the lists which are part of the uidMatrix would have been changed
		// after sort. We want to update the order of lists in the facetMatrix accordingly.
		for idx, rl := range result.UidMatrix {
			fl := make([]*pb.Facets, 0, len(sg.facetsMatrix[idx].FacetsList))
			for _, uid := range rl.Uids {
				// Find index of this uid in original sorted uid list.
				oidx := algo.IndexOf(sg.uidMatrix[idx], uid)
				// Find corresponding facet.
				fl = append(fl, sg.facetsMatrix[idx].FacetsList[oidx])
			}
			sg.facetsMatrix[idx].FacetsList = fl
		}
	}

	sg.uidMatrix = result.UidMatrix
	// Update the destUids as we might have removed some UIDs for which we didn't find any values
	// while sorting.
	sg.updateDestUids()
	return nil
}

// createOrderForTask creates namespaced aware order for the task.
func (sg *SubGraph) createOrderForTask(ns uint64) []*pb.Order {
	out := []*pb.Order{}
	for _, o := range sg.Params.Order {
		oc := &pb.Order{
			Attr:  x.NamespaceAttr(ns, o.Attr),
			Desc:  o.Desc,
			Langs: o.Langs,
		}
		out = append(out, oc)
	}
	return out
}

func (sg *SubGraph) updateDestUids() {
	// Update sg.destUID. Iterate over the UID matrix (which is not sorted by
	// UID). For each element in UID matrix, we do a binary search in the
	// current destUID and mark it. Then we scan over this bool array and
	// rebuild destUIDs.
	sg.DestMap = codec.Merge(sg.uidMatrix)
}

func (sg *SubGraph) sortAndPaginateUsingFacet(ctx context.Context) error {
	if len(sg.facetsMatrix) == 0 {
		return nil
	}
	if len(sg.facetsMatrix) != len(sg.uidMatrix) {
		return errors.Errorf("Facet matrix and UID matrix mismatch: %d vs %d",
			len(sg.facetsMatrix), len(sg.uidMatrix))
	}

	orderbyKeys := make(map[string]int)
	var orderDesc []bool
	for i, order := range sg.Params.FacetsOrder {
		orderbyKeys[order.Key] = i
		orderDesc = append(orderDesc, order.Desc)
	}

	for i := 0; i < len(sg.uidMatrix); i++ {
		ul := sg.uidMatrix[i]
		fl := sg.facetsMatrix[i]
		uids := ul.Uids[:0]
		facetList := fl.FacetsList[:0]

		values := make([][]types.Val, len(ul.Uids))
		for i := 0; i < len(values); i++ {
			values[i] = make([]types.Val, len(sg.Params.FacetsOrder))
		}

		for j := 0; j < len(ul.Uids); j++ {
			uid := ul.Uids[j]
			f := fl.FacetsList[j]
			uids = append(uids, uid)
			facetList = append(facetList, f)

			// Since any facet can come only once in f.Facets, we can have counter to check if we
			// have populated all facets or not. Once we are done populating all facets
			// we can break out of below loop.
			remainingFacets := len(orderbyKeys)
			// TODO: We are searching sequentially, explore if binary search is useful here.
			for _, it := range f.Facets {
				idx, ok := orderbyKeys[it.Key]
				if !ok {
					continue
				}

				fVal, err := facets.ValFor(it)
				if err != nil {
					return err
				}
				// If type is not sortable, we are ignoring it.
				if types.IsSortable(fVal.Tid) {
					values[j][idx] = fVal
				}

				remainingFacets--
				if remainingFacets == 0 {
					break
				}
			}
		}
		if len(values) == 0 {
			continue
		}

		if err := types.SortWithFacet(values, &uids, facetList, orderDesc, ""); err != nil {
			return err
		}
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
	sg.updateDestUids()
	return nil
}

func (sg *SubGraph) sortAndPaginateUsingVar(ctx context.Context) error {
	// nil has a different meaning from an initialized map of zero length here. If the variable
	// didn't return any values then UidToVal would be an empty with zero length. If the variable
	// was used before definition, UidToVal would be nil.
	if sg.Params.UidToVal == nil {
		return errors.Errorf("Variable: [%s] used before definition.", sg.Params.Order[0].Attr)
	}

	for i := 0; i < len(sg.uidMatrix); i++ {
		ul := sg.uidMatrix[i]
		uids := make([]uint64, 0, len(ul.Uids))
		values := make([][]types.Val, 0, len(ul.Uids))
		for _, uid := range ul.Uids {
			v, ok := sg.Params.UidToVal[uid]
			if !ok {
				// We skip the UIDs which don't have a value.
				continue
			}
			values = append(values, []types.Val{v})
			uids = append(uids, uid)
		}
		if len(values) == 0 {
			continue
		}
		if err := types.Sort(values, &uids, []bool{sg.Params.Order[0].Desc}, ""); err != nil {
			return err
		}
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
	sg.updateDestUids()
	return nil
}

// isValidArg checks if arg passed is valid keyword.
func isValidArg(a string) bool {
	switch a {
	case "numpaths", "from", "to", "orderasc", "orderdesc", "first", "offset", "after", "depth",
		"minweight", "maxweight", "random":
		return true
	}
	return false
}

// isValidFuncName checks if fn passed is valid keyword.
func isValidFuncName(f string) bool {
	switch f {
	case "anyofterms", "allofterms", "val", "regexp", "anyoftext", "alloftext",
		"has", "uid", "uid_in", "anyof", "allof", "type", "match":
		return true
	}
	return isInequalityFn(f) || types.IsGeoFunc(f)
}

func isInequalityFn(f string) bool {
	switch f {
	case "eq", "le", "ge", "gt", "lt", "between":
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
	return f != nil && f.Name == "uid" && len(f.NeedsVar) == 0
}

func getNodeTypes(ctx context.Context, sg *SubGraph) ([]string, error) {
	temp := &SubGraph{
		Attr:    "dgraph.type",
		SrcUIDs: codec.ToList(sg.DestMap),
		ReadTs:  sg.ReadTs,
	}
	taskQuery, err := createTaskQuery(ctx, temp)
	if err != nil {
		return nil, err
	}
	result, err := worker.ProcessTaskOverNetwork(ctx, taskQuery)
	if err != nil {
		return nil, err
	}
	return getPredsFromVals(result.ValueMatrix), nil
}

// getPredicatesFromTypes returns the list of preds contained in the given types.
func getPredicatesFromTypes(namespace uint64, typeNames []string) []string {
	var preds []string

	for _, typeName := range typeNames {
		typeDef, ok := schema.State().GetType(x.NamespaceAttr(namespace, typeName))
		if !ok {
			continue
		}

		for _, field := range typeDef.Fields {
			preds = append(preds, field.Predicate)
		}
	}
	return preds
}

// filterUidPredicates takes a list of predicates and returns a list of the predicates
// that are of type uid or [uid].
func filterUidPredicates(ctx context.Context, preds []string) ([]string, error) {
	schs, err := worker.GetSchemaOverNetwork(ctx, &pb.SchemaRequest{Predicates: preds})
	if err != nil {
		return nil, err
	}

	filteredPreds := make([]string, 0)
	for _, sch := range schs {
		if sch.GetType() != "uid" {
			continue
		}
		filteredPreds = append(filteredPreds, sch.GetPredicate())
	}
	return filteredPreds, nil
}

// UidsToHex converts the new UIDs to hex string.
func UidsToHex(m map[string]uint64) map[string]string {
	res := make(map[string]string)
	for k, v := range m {
		res[k] = UidToHex(v)
	}
	return res
}

func UidToHex(uid uint64) string {
	return fmt.Sprintf("%#x", uid)
}

// Request wraps the state that is used when executing query.
// Initially ReadTs, Cache and GqlQuery are set.
// Subgraphs, Vars and Latency are filled when processing query.
type Request struct {
	ReadTs   uint64 // ReadTs for the transaction.
	Cache    int    // 0 represents use txn cache, 1 represents not to use cache.
	Latency  *Latency
	GqlQuery *gql.Result

	Subgraphs []*SubGraph

	Vars map[string]varValue
}

// ProcessQuery processes query part of the request (without mutations).
// Fills Subgraphs and Vars.
// It can process multiple query blocks that are part of the query..
func (req *Request) ProcessQuery(ctx context.Context) (err error) {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "query.ProcessQuery")
	defer stop()

	// Vars stores the processed variables.
	req.Vars = make(map[string]varValue)
	loopStart := time.Now()
	queries := req.GqlQuery.Query
	// first loop converts queries to SubGraph representation and populates ReadTs And Cache.
	for i := 0; i < len(queries); i++ {
		gq := queries[i]

		if gq == nil || (len(gq.UID) == 0 && gq.Func == nil && len(gq.NeedsVar) == 0 &&
			gq.Alias != "shortest" && !gq.IsEmpty) {
			return errors.Errorf("Invalid query. No function used at root and no aggregation" +
				" or math variables found in the body.")
		}
		sg, err := ToSubGraph(ctx, gq)
		if err != nil {
			return errors.Wrapf(err, "while converting to subgraph")
		}
		sg.recurse(func(sg *SubGraph) {
			sg.ReadTs = req.ReadTs
			sg.Cache = req.Cache
		})
		span.Annotate(nil, "Query parsed")
		req.Subgraphs = append(req.Subgraphs, sg)
	}
	req.Latency.Parsing += time.Since(loopStart)

	execStart := time.Now()
	hasExecuted := make([]bool, len(req.Subgraphs))
	numQueriesDone := 0

	// canExecute returns true if a query block is ready to execute with all the variables
	// that it depends on are already populated or are defined in the same block.
	canExecute := func(idx int) bool {
		queryVars := req.GqlQuery.QueryVars[idx]
		for _, v := range queryVars.Needs {
			// here we check if this block defines the variable v.
			var selfDep bool
			for _, vd := range queryVars.Defines {
				if v == vd {
					selfDep = true
					break
				}
			}
			// The variable should be defined in this block or should have already been
			// populated by some other block, otherwise we are not ready to execute yet.
			_, ok := req.Vars[v]
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

			err = sg.recursiveFillVars(req.Vars)
			if err != nil {
				return err
			}
			hasExecuted[idx] = true
			numQueriesDone++
			idxList = append(idxList, idx)
			// A query doesn't need to be executed if
			// 1. It just does aggregation and math functions which is when sg.Params.IsEmpty is true.
			// 2. Its has an inequality fn at root without any args which can happen when it uses
			// value variables for args which don't expand to any value.
			if sg.Params.IsEmpty || isEmptyIneqFnWithVar(sg) {
				errChan <- nil
				continue
			}

			switch {
			case sg.Params.Alias == "shortest":
				// We allow only one shortest path block per query.
				go func() {
					shortestSg, err = shortestPath(ctx, sg)
					errChan <- err
				}()
			case sg.Params.Recurse:
				go func() {
					errChan <- recurse(ctx, sg)
				}()
			default:
				go ProcessGraph(ctx, sg, nil, errChan)
			}
		}

		var ferr error
		// Wait for the execution that was started in this iteration.
		for i := 0; i < len(idxList); i++ {
			if err = <-errChan; err != nil {
				ferr = err
				continue
			}
		}
		if ferr != nil {
			return ferr
		}

		// If the executed subgraph had some variable defined in it, Populate it in the map.
		for _, idx := range idxList {
			sg := req.Subgraphs[idx]

			var sgPath []*SubGraph
			if err := sg.populateVarMap(req.Vars, sgPath); err != nil {
				return err
			}
			// first time at the root here.

			// Apply pagination at the root after @cascade.
			if len(sg.Params.Cascade.Fields) > 0 && (sg.Params.Cascade.First != 0 || sg.Params.Cascade.Offset != 0) {
				sg.updateUidMatrix()
				for i := 0; i < len(sg.uidMatrix); i++ {
					start, end := x.PageRange(sg.Params.Cascade.First, sg.Params.Cascade.Offset, len(sg.uidMatrix[i].Uids))
					sg.uidMatrix[i].Uids = sg.uidMatrix[i].Uids[start:end]
				}
			}

			if err := sg.populatePostAggregation(req.Vars, []*SubGraph{}, nil); err != nil {
				return err
			}
		}
	}

	// Ensure all the queries are executed.
	for _, it := range hasExecuted {
		if !it {
			return errors.Errorf("Query couldn't be executed")
		}
	}
	req.Latency.Processing += time.Since(execStart)

	// If we had a shortestPath SG, append it to the result.
	if len(shortestSg) != 0 {
		req.Subgraphs = append(req.Subgraphs, shortestSg...)
	}
	return nil
}

// ExecutionResult holds the result of running a query.
type ExecutionResult struct {
	Subgraphs  []*SubGraph
	SchemaNode []*pb.SchemaNode
	Types      []*pb.TypeUpdate
	Metrics    map[string]uint64
}

// Process handles a query request.
func (req *Request) Process(ctx context.Context) (er ExecutionResult, err error) {
	err = req.ProcessQuery(ctx)
	if err != nil {
		return er, err
	}
	er.Subgraphs = req.Subgraphs
	// calculate metrics.
	metrics := make(map[string]uint64)
	for _, sg := range er.Subgraphs {
		calculateMetrics(sg, metrics)
	}
	er.Metrics = metrics
	namespace, err := x.ExtractNamespace(ctx)
	if err != nil {
		return er, errors.Wrapf(err, "While processing query")
	}
	schemaProcessingStart := time.Now()
	if req.GqlQuery.Schema != nil {
		preds := x.NamespaceAttrList(namespace, req.GqlQuery.Schema.Predicates)
		req.GqlQuery.Schema.Predicates = preds
		if er.SchemaNode, err = worker.GetSchemaOverNetwork(ctx, req.GqlQuery.Schema); err != nil {
			return er, errors.Wrapf(err, "while fetching schema")
		}
		typeNames := x.NamespaceAttrList(namespace, req.GqlQuery.Schema.Types)
		req.GqlQuery.Schema.Types = typeNames
		if er.Types, err = worker.GetTypes(ctx, req.GqlQuery.Schema); err != nil {
			return er, errors.Wrapf(err, "while fetching types")
		}
	}

	if !x.IsGalaxyOperation(ctx) {
		// Filter the schema nodes for the given namespace.
		er.SchemaNode = filterSchemaNodeForNamespace(namespace, er.SchemaNode)
		// Filter the types for the given namespace.
		er.Types = filterTypesForNamespace(namespace, er.Types)
	}
	req.Latency.Processing += time.Since(schemaProcessingStart)

	return er, nil
}

// filterTypesForNamespace filters types for the given namespace.
func filterTypesForNamespace(namespace uint64, types []*pb.TypeUpdate) []*pb.TypeUpdate {
	out := []*pb.TypeUpdate{}
	for _, update := range types {
		// Type name doesn't have reverse.
		typeNamespace, typeName := x.ParseNamespaceAttr(update.TypeName)
		if typeNamespace != namespace {
			continue
		}
		update.TypeName = typeName
		fields := []*pb.SchemaUpdate{}
		// Convert field name for the current namespace.
		for _, field := range update.Fields {
			_, fieldName := x.ParseNamespaceAttr(field.Predicate)
			field.Predicate = fieldName
			fields = append(fields, field)
		}
		update.Fields = fields
		out = append(out, update)
	}
	return out
}

// filterSchemaNodeForNamespace filters schema nodes for the given namespace.
func filterSchemaNodeForNamespace(namespace uint64, nodes []*pb.SchemaNode) []*pb.SchemaNode {
	out := []*pb.SchemaNode{}

	for _, node := range nodes {
		nodeNamespace, attrName := x.ParseNamespaceAttr(node.Predicate)
		if nodeNamespace != namespace {
			continue
		}
		node.Predicate = attrName
		out = append(out, node)
	}
	return out
}

// StripBlankNode returns a copy of the map where all the keys have the blank node prefix removed.
func StripBlankNode(mp map[string]uint64) map[string]uint64 {
	temp := make(map[string]uint64)
	for k, v := range mp {
		if strings.HasPrefix(k, "_:") {
			temp[k[2:]] = v
		}
	}
	return temp
}

// calculateMetrics populates the given map with the number of UIDs that were seen
// for each predicate.
func calculateMetrics(sg *SubGraph, metrics map[string]uint64) {
	// Skip internal nodes.
	if !sg.IsInternal() {
		// Add the number of SrcUIDs. This is the number of uids processed by this attribute.
		metrics[sg.Attr] += uint64(len(sg.SrcUIDs.GetUids()))
	}
	// Add all the uids gathered by filters.
	for _, filter := range sg.Filters {
		calculateMetrics(filter, metrics)
	}
	// Calculate metrics for the children as well.
	for _, child := range sg.Children {
		calculateMetrics(child, metrics)
	}
}
