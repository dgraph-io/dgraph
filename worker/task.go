/*
 * Copyright 2016 DGraph Labs, Inc.
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

package worker

import (
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/types/facets/utils"
	"github.com/dgraph-io/dgraph/x"
)

var (
	emptyUIDList task.List
	emptyResult  task.Result
)

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, q *task.Query) (*task.Result, error) {
	attr := q.Attr
	gid := group.BelongsTo(attr)
	x.Trace(ctx, "attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processTask(q, gid)
	}

	// Send this over the network.
	// TODO: Send the request to multiple servers as described in Jeff Dean's talk.
	addr := groups().AnyServer(gid)
	pl := pools().get(addr)

	conn, err := pl.Get()
	if err != nil {
		return &emptyResult, x.Wrapf(err, "ProcessTaskOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := NewWorkerClient(conn)
	reply, err := c.ServeTask(ctx, q)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.ServeTask"))
		return &emptyResult, err
	}

	x.Trace(ctx, "Reply from server. length: %v Addr: %v Attr: %v",
		len(reply.UidMatrix), addr, attr)
	return reply, nil
}

// convertValue converts the data to the schema.State() type of predicate.
func convertValue(attr, data string) (types.Val, error) {
	// Parse given value and get token. There should be only one token.
	t, err := schema.State().TypeOf(attr)
	if err != nil || !t.IsScalar() {
		return types.Val{}, x.Errorf("Attribute %s is not valid scalar type", attr)
	}

	src := types.Val{types.StringID, []byte(data)}
	dst, err := types.Convert(src, t)
	return dst, err
}

type FuncType int

const (
	NotFn FuncType = iota
	AggregatorFn
	CompareAttrFn
	CompareScalarFn
	GeoFn
	PasswordFn
	StandardFn = 100
)

func parseFuncType(arr []string) (FuncType, string) {
	if len(arr) == 0 {
		return NotFn, ""
	}
	f := strings.ToLower(arr[0])
	switch f {
	case "leq", "geq", "lt", "gt", "eq":
		// gt(release_date, "1990") is 'CompareAttr' which
		//    takes advantage of indexed-attr
		// gt(count(films), 0) is 'CompareScalar', we first do
		//    counting on attr, then compare the result as scalar with int
		if len(arr) > 2 && arr[1] == "count" {
			return CompareScalarFn, f
		}
		return CompareAttrFn, f
	case "min", "max", "sum":
		return AggregatorFn, f
	case "checkpwd":
		return PasswordFn, f
	default:
		if types.IsGeoFunc(f) {
			return GeoFn, f
		}
		return StandardFn, f
	}
}

// processTask processes the query, accumulates and returns the result.
func processTask(q *task.Query, gid uint32) (*task.Result, error) {
	attr := q.Attr
	resParseFn, err := getSrcFnContext(q)
	if err != nil {
		return nil, err
	}

	fnType, f := resParseFn.fnType, resParseFn.fname
	tokens := resParseFn.tokens
	geoQuery := resParseFn.geoQuery
	intersectDest := resParseFn.intersectDest
	ineqValue := resParseFn.ineqValue
	ineqValueToken := resParseFn.ineqValueToken
	n := resParseFn.n
	threshold := resParseFn.threshold

	var out task.Result
	it := algo.NewListIterator(q.Uids)
	opts := posting.ListOptions{
		AfterUID: uint64(q.AfterUid),
	}
	// If we have srcFunc and Uids, it means its a filter. So we intersect.
	if fnType != NotFn && algo.ListLen(q.Uids) > 0 {
		opts.Intersect = q.Uids
	}

	for i := 0; i < n; i++ {
		var key []byte
		var uid uint64
		if it.Valid() {
			uid = it.Val()
		}
		if fnType == AggregatorFn || fnType == CompareScalarFn || fnType == PasswordFn {
			key = x.DataKey(attr, it.Val())
			it.Next()
		} else if fnType != NotFn {
			key = x.IndexKey(attr, tokens[i])
		} else if q.Reverse {
			key = x.ReverseKey(attr, it.Val())
			it.Next()
		} else {
			key = x.DataKey(attr, it.Val())
			it.Next()
		}
		// Get or create the posting list for an entity, attribute combination.
		pl, decr := posting.GetOrCreate(key, gid)
		defer decr()
		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		val, err := pl.ValueFor(q.Langs)
		isValueEdge := err == nil
		newValue := &task.Value{ValType: int32(val.Tid)}
		if err == nil {
			newValue.Val = val.Value.([]byte)
		} else {
			newValue.Val = x.Nilbyte
		}
		out.Values = append(out.Values, newValue)

		// get filtered uids and facets.
		type result struct {
			uid    uint64
			facets []*facets.Facet
		}
		var filteredRes []*result
		if !isValueEdge { // for uid edge.. get postings
			var perr error
			// Get postings and filter based on facetFilterTree.
			pl.Postings(opts, func(p *types.Posting) bool {
				res := true
				res, perr = applyFacetFilter(p.Facets, q.FacetsFilter)
				if perr != nil {
					return false // break loop.
				}
				if res {
					filteredRes = append(filteredRes, &result{
						uid:    p.Uid,
						facets: utils.CopyFacets(p.Facets, q.FacetParam)})
				}
				return true // continue iteration.
			})
			if perr != nil {
				return nil, perr
			}
		} else if q.FacetsFilter != nil { // else part means isValueEdge
			// This is Value edge and we are asked to do facet filtering. Not supported.
			return nil, x.Errorf("Facet filtering is not supported on values.")
		}

		// add facets to result.
		if q.FacetParam != nil {
			if isValueEdge {
				fs, err := pl.Facets(q.FacetParam)
				if err != nil {
					fs = []*facets.Facet{}
				}
				out.FacetMatrix = append(out.FacetMatrix,
					&facets.List{[]*facets.Facets{&facets.Facets{fs}}})
			} else {
				var fcsList []*facets.Facets
				for _, fres := range filteredRes {
					fcsList = append(fcsList, &facets.Facets{fres.facets})
				}
				out.FacetMatrix = append(out.FacetMatrix, &facets.List{fcsList})
			}
		}

		// add uids to uidmatrix..
		if q.DoCount || fnType == AggregatorFn {
			if q.DoCount {
				out.Counts = append(out.Counts, uint32(pl.Length(0)))
			}
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			continue
		}

		if fnType == PasswordFn {
			lastPos := len(out.Values) - 1
			if len(newValue.Val) == 0 {
				out.Values[lastPos] = task.FalseVal
			}
			pwd := q.SrcFunc[1]
			err = types.VerifyPassword(pwd, string(newValue.Val))
			if err != nil {
				out.Values[lastPos] = task.FalseVal
			} else {
				out.Values[lastPos] = task.TrueVal
			}
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			continue
		}

		if fnType == CompareScalarFn {
			count := int64(pl.Length(0))
			if EvalCompare(f, count, threshold) {
				tlist := algo.SortedListToBlock([]uint64{uid})
				out.UidMatrix = append(out.UidMatrix, tlist)
			}
			continue
		}

		// The more usual case: Getting the UIDs.
		uidList := new(task.List)
		uidw := algo.NewWriteIterator(uidList, 0)
		for _, fres := range filteredRes {
			uidw.Append(fres.uid)
		}
		uidw.End()
		out.UidMatrix = append(out.UidMatrix, uidList)
	}

	// aggregate on the collection out.Values[]
	if fnType == AggregatorFn && len(out.Values) > 0 {
		var err error
		typ, _ := schema.State().TypeOf(attr)
		out.Values[0], err = Aggregate(f, out.Values, typ)
		if err != nil {
			return nil, err
		}
		out.Values = out.Values[:1] // trim length to 1
	}

	if fnType == CompareAttrFn && len(tokens) > 0 && ineqValueToken == tokens[0] {
		// Need to evaluate inequality for entries in the first bucket.
		typ, err := schema.State().TypeOf(attr)
		if err != nil || !typ.IsScalar() {
			return nil, x.Errorf("Attribute not scalar: %s %v", attr, typ)
		}

		x.AssertTrue(len(out.UidMatrix) > 0)
		// Filter the first row of UidMatrix. Since ineqValue != nil, we may
		// assume that ineqValue is equal to the first token found in TokensTable.
		algo.ApplyFilter(out.UidMatrix[0], func(uid uint64, i int) bool {
			sv, err := fetchValue(uid, attr, q.Langs, typ)
			if sv.Value == nil || err != nil {
				return false
			}
			return compareTypeVals(q.SrcFunc[0], sv, ineqValue)
		})
	}

	// If geo filter, do value check for correctness.
	var values []*task.Value
	if geoQuery != nil {
		uids := algo.MergeSorted(out.UidMatrix)
		it := algo.NewListIterator(uids)
		for ; it.Valid(); it.Next() {
			uid := it.Val()
			key := x.DataKey(attr, uid)
			pl, decr := posting.GetOrCreate(key, gid)
			val, err := pl.Value()
			newValue := &task.Value{ValType: int32(val.Tid)}
			if err == nil {
				newValue.Val = val.Value.([]byte)
			} else {
				newValue.Val = x.Nilbyte
			}
			values = append(values, newValue)
			decr() // Decrement the reference count of the pl.
		}

		filtered := types.FilterGeoUids(uids, values, geoQuery)
		for i := 0; i < len(out.UidMatrix); i++ {
			algo.IntersectWith(out.UidMatrix[i], filtered)
		}
	}
	out.IntersectDest = intersectDest
	return &out, nil
}

type functionContext struct {
	tokens         []string
	geoQuery       *types.GeoQueryData
	intersectDest  bool
	ineqValue      types.Val
	ineqValueToken string
	n              int
	threshold      int64
	fname          string
	fnType         FuncType
}

func getSrcFnContext(q *task.Query) (*functionContext, error) {
	fnType, f := parseFuncType(q.SrcFunc)
	attr := q.Attr
	res := &functionContext{fnType: fnType, fname: f}
	var err error

	switch fnType {
	case AggregatorFn:
		// confirm agrregator could apply on the attributes
		typ, err := schema.State().TypeOf(attr)
		if err != nil {
			return nil, x.Errorf("Attribute %q is not scalar-type", attr)
		}
		if !CouldApplyAggregatorOn(f, typ) {
			return nil, x.Errorf("Aggregator %q could not apply on %v",
				f, attr)
		}
		res.n = algo.ListLen(q.Uids)
		return res, nil
	case CompareAttrFn:
		if len(q.SrcFunc) != 2 {
			return nil, x.Errorf("Function requires 2 arguments, but got %d %v",
				len(q.SrcFunc), q.SrcFunc)
		}
		res.ineqValue, err = convertValue(attr, q.SrcFunc[1])
		if err != nil {
			return nil, err
		}
		// Tokenizing RHS value of inequality.
		// TODO(kg): more comments about why we convert to types.BinaryID, and
		// then convert it back to attr type in IndexTokens.
		// the point is IndexTokens need BinaryID type to be passed in
		v := types.ValueForType(types.BinaryID)
		err = types.Marshal(res.ineqValue, &v)
		if err != nil {
			return nil, err
		}
		ineqTokens, err := posting.IndexTokens(attr,
			types.Val{res.ineqValue.Tid, v.Value.([]byte)})
		if err != nil {
			return nil, err
		}
		if len(ineqTokens) != 1 {
			return nil, x.Errorf("Expected only 1 token but got: %v", ineqTokens)
		}
		res.ineqValueToken = ineqTokens[0]
		// Get tokens geq / leq ineqValueToken.
		res.tokens, err = getInequalityTokens(attr, res.ineqValueToken, f)
		if err != nil {
			return nil, err
		}
		res.n = len(res.tokens)
		return res, nil

	case CompareScalarFn:
		if len(q.SrcFunc) != 3 {
			return nil, x.Errorf("Function requires 3 arguments, but got %d %v",
				len(q.SrcFunc), q.SrcFunc)
		}
		res.threshold, err = strconv.ParseInt(q.SrcFunc[2], 10, 64)
		if err != nil {
			return nil, x.Wrapf(err, "Compare %v(%v) require digits, but got invalid num",
				q.SrcFunc[0], q.SrcFunc[1])
		}
		res.n = algo.ListLen(q.Uids)
		return res, nil

	case GeoFn:
		// For geo functions, we get extra information used for filtering.
		res.tokens, res.geoQuery, err = types.GetGeoTokens(q.SrcFunc)
		if err != nil {
			return nil, err
		}
		res.n = len(res.tokens)
		return res, nil

	case PasswordFn:
		// confirm agrregator could apply on the attributes
		if len(q.SrcFunc) != 2 {
			return nil, x.Errorf("Function requires 2 arguments, but got %d %v",
				len(q.SrcFunc), q.SrcFunc)
		}
		res.n = algo.ListLen(q.Uids)
		return res, nil

	case StandardFn:
		// srcfunc 0th val is func name and and [1:] are args.
		// we tokenize the arguments of the query.
		res.tokens, err = tok.GetTokens(q.SrcFunc[1:])
		if err != nil {
			return nil, err
		}
		res.intersectDest = (strings.ToLower(q.SrcFunc[0]) == "allof")
		res.n = len(res.tokens)
		return res, nil

	case NotFn:
		res.n = algo.ListLen(q.Uids)
		return res, nil
	}
	return nil, x.Errorf("FnType %d not handled in numFnAttrs.", fnType)
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, q *task.Query) (*task.Result, error) {
	if ctx.Err() != nil {
		return &emptyResult, ctx.Err()
	}

	gid := group.BelongsTo(q.Attr)
	x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr, algo.ListLen(q.Uids), gid)

	var reply *task.Result
	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", q.Attr, gid)

	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = processTask(q, gid)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}

// applyFacetFilter : we return error only when query has some problems.
// like Or has 3 arguments, argument facet val overflows integer.
// returns true if postingFacets can be included.
func applyFacetFilter(postingFacets []*facets.Facet, tree *facets.FilterTree) (bool, error) {
	ftree, err := preprocessFilter(tree)
	if err != nil {
		return false, err
	}
	return applyFacetsTree(postingFacets, ftree)
}

func applyFacetsTree(postingFacets []*facets.Facet, ftree *facetsTree) (bool, error) {
	if ftree == nil {
		return true, nil
	}
	if ftree.function != nil {
		fname := strings.ToLower(ftree.function.name)
		var fc *facets.Facet
		for _, fci := range postingFacets {
			if fci.Key == ftree.function.key {
				fc = fci
				break
			}
		}
		if fc == nil { // facet is not there
			return false, nil
		}
		fnType, fname := parseFuncType([]string{fname})
		if len(ftree.function.args) != 1 {
			return false, x.Errorf("Only one argument expected in %s, but got %d.",
				fname, len(ftree.function.args))
		}

		switch fnType {
		case CompareAttrFn: // lt, gt, le, ge, eq
			return compareTypeVals(fname, types.ValFor(fc), ftree.function.val), nil

		case StandardFn: // allof, anyof
			if facets.TypeIDForValType(fc.ValType) != facets.StringID {
				return false, nil
			}
			return filterOnStandardFn(fname, fc.Tokens, ftree.function.tokens)
		}
		return false, x.Errorf("Fn %s not supported in facets filtering.", fname)
	}

	var res []bool
	for _, c := range ftree.children {
		if r, err := applyFacetsTree(postingFacets, c); err != nil {
			return false, err
		} else {
			res = append(res, r)
		}
	}

	switch strings.ToLower(ftree.op) {
	case "not":
		return !res[0], nil
	case "and":
		return res[0] && res[1], nil
	case "or":
		return res[0] || res[1], nil
	}
	return false, x.Errorf("Unexpected behavior in applyFacetsTree.")
}

// Should be used only in filtering arg1 by comparing with arg2.
// arg2 is reference Val to which arg1 is compared.
func compareTypeVals(op string, arg1, arg2 types.Val) bool {
	revRes := func(b bool, e error) (bool, error) { // reverses result
		return !b, e
	}
	noError := func(b bool, e error) bool {
		return b && e == nil
	}
	switch op {
	case "geq":
		return noError(revRes(types.Less(arg1, arg2)))
	case "gt":
		return noError(types.Less(arg2, arg1))
	case "leq":
		return noError(revRes(types.Less(arg2, arg1)))
	case "lt":
		return noError(types.Less(arg1, arg2))
	case "eq":
		return noError(types.Equal(arg1, arg2))
	default:
		// should have been checked at query level.
		x.Fatalf("Unknown ineqType %v", op)
	}
	return false
}

// filterOnStandardFn : tells whether facet corresponding to fcTokens can be taken or not.
// fcTokens and argTokens should be sorted.
func filterOnStandardFn(fname string, fcTokens []string, argTokens []string) (bool, error) {
	switch fname {
	case "allof":
		// allof argTokens should be in fcTokens
		if len(argTokens) > len(fcTokens) {
			return false, nil
		}
		aidx := 0
		for fidx := 0; aidx < len(argTokens) && fidx < len(fcTokens); {
			if fcTokens[fidx] < argTokens[aidx] {
				fidx++
			} else if fcTokens[fidx] == argTokens[aidx] {
				fidx++
				aidx++
			} else {
				// as all of argTokens should match
				// which is not possible now.
				break
			}
		}
		return aidx == len(argTokens), nil
	case "anyof":
		for aidx, fidx := 0, 0; aidx < len(argTokens) && fidx < len(fcTokens); {
			if fcTokens[fidx] < argTokens[aidx] {
				fidx++
			} else if fcTokens[fidx] == argTokens[aidx] {
				return true, nil
			} else {
				aidx++
			}
		}
		return false, nil
	}
	return false, x.Errorf("Fn %s not supported in facets filtering.", fname)
}

type facetsFunc struct {
	name   string
	key    string
	args   []string
	tokens []string
	val    types.Val
}
type facetsTree struct {
	op       string
	children []*facetsTree
	function *facetsFunc
}

func preprocessFilter(tree *facets.FilterTree) (*facetsTree, error) {
	if tree == nil {
		return nil, nil
	}
	ftree := &facetsTree{}
	ftree.op = tree.Op
	if tree.Func != nil {
		ftree.function = &facetsFunc{}
		ftree.function.name = tree.Func.Name
		ftree.function.key = tree.Func.Key
		ftree.function.args = tree.Func.Args

		fnType, fname := parseFuncType([]string{ftree.function.name})
		switch fnType {
		case CompareAttrFn:
			argf, err := utils.FacetFor(tree.Func.Key, tree.Func.Args[0])
			if err != nil {
				return nil, err // stop processing as this is query error
			}
			ftree.function.val = types.ValFor(argf)
		case StandardFn:
			argTokens, aerr := tok.GetTokens(tree.Func.Args)
			if aerr != nil { // query error ; stop processing.
				return nil, aerr
			}
			sort.Strings(argTokens)
			ftree.function.tokens = argTokens
		default:
			return nil, x.Errorf("Fn %s not supported in preprocessFilter.", fname)
		}
		return ftree, nil
	}

	for _, c := range tree.Children {
		ftreec, err := preprocessFilter(c)
		if err != nil {
			return nil, err
		}
		ftree.children = append(ftree.children, ftreec)
	}

	numChild := len(tree.Children)
	switch strings.ToLower(tree.Op) {
	case "not":
		if numChild != 1 {
			return nil, x.Errorf("Expected 1 child for not but got %d.", numChild)
		}
	case "and":
		if numChild != 2 {
			return nil, x.Errorf("Expected 2 child for not but got %d.", numChild)
		}
	case "or":
		if numChild != 2 {
			return nil, x.Errorf("Expected 2 child for not but got %d.", numChild)
		}
	default:
		return nil, x.Errorf("Unsupported operation in facet filtering: %s.", tree.Op)
	}
	return ftree, nil
}
