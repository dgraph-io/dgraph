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

package worker

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/facetsp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"

	cindex "github.com/google/codesearch/index"
	cregexp "github.com/google/codesearch/regexp"
)

var (
	emptyUIDList taskp.List
	emptyResult  taskp.Result
	regexTok     tok.ExactTokenizer
)

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, q *taskp.Query) (*taskp.Result, error) {
	attr := q.Attr
	gid := group.BelongsTo(attr)
	x.Trace(ctx, "attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processTask(ctx, q, gid)
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

	c := workerp.NewWorkerClient(conn)
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
	if err != nil {
		return types.Val{}, err
	}
	if !t.IsScalar() {
		return types.Val{}, x.Errorf("Attribute %s is not valid scalar type", attr)
	}

	src := types.Val{types.StringID, []byte(data)}
	dst, err := types.Convert(src, t)
	return dst, err
}

// Returns nil byte on error
func convertToType(v types.Val, typ types.TypeID) (*taskp.Value, error) {
	result := &taskp.Value{ValType: int32(typ), Val: x.Nilbyte}
	if v.Tid == typ {
		result.Val = v.Value.([]byte)
		return result, nil
	}

	// conver data from binary to appropriate format
	val, err := types.Convert(v, typ)
	if err != nil {
		return result, err
	}
	// Marshal
	data := types.ValueForType(types.BinaryID)
	err = types.Marshal(val, &data)
	if err != nil {
		return result, x.Errorf("Failed convertToType during Marshal")
	}
	result.Val = data.Value.([]byte)
	return result, nil
}

type FuncType int

const (
	NotAFunction FuncType = iota
	AggregatorFn
	CompareAttrFn
	CompareScalarFn
	GeoFn
	PasswordFn
	RegexFn
	FullTextSearchFn
	StandardFn = 100
)

const numPart = uint64(32)

func parseFuncType(arr []string) (FuncType, string) {
	if len(arr) == 0 {
		return NotAFunction, ""
	}
	f := strings.ToLower(arr[0])
	switch f {
	case "le", "ge", "lt", "gt", "eq":
		// gt(release_date, "1990") is 'CompareAttr' which
		//    takes advantage of indexed-attr
		// gt(count(films), 0) is 'CompareScalar', we first do
		//    counting on attr, then compare the result as scalar with int
		if len(arr) > 3 && arr[2] == "count" {
			return CompareScalarFn, f
		}
		return CompareAttrFn, f
	case "min", "max", "sum", "avg":
		return AggregatorFn, f
	case "checkpwd":
		return PasswordFn, f
	case "regexp":
		return RegexFn, f
	case "alloftext", "anyoftext":
		return FullTextSearchFn, f
	default:
		if types.IsGeoFunc(f) {
			return GeoFn, f
		}
		return StandardFn, f
	}
}

func needsIndex(fnType FuncType) bool {
	switch fnType {
	case CompareAttrFn, GeoFn, RegexFn, FullTextSearchFn, StandardFn:
		return true
	default:
		return false
	}
}

// processTask processes the query, accumulates and returns the result.
func processTask(ctx context.Context, q *taskp.Query, gid uint32) (*taskp.Result, error) {
	attr := q.Attr
	srcFn, err := parseSrcFn(q)
	if err != nil {
		return nil, err
	}

	if q.Reverse && !schema.State().IsReversed(attr) {
		return nil, x.Errorf("Predicate %s doesn't have reverse edge", attr)
	}
	if needsIndex(srcFn.fnType) && !schema.State().IsIndexed(q.Attr) {
		return nil, x.Errorf("Predicate %s is not indexed", q.Attr)
	}

	var out taskp.Result
	opts := posting.ListOptions{
		AfterUID: uint64(q.AfterUid),
	}
	// If we have srcFunc and Uids, it means its a filter. So we intersect.
	if srcFn.fnType != NotAFunction && q.UidList != nil && len(q.UidList.Uids) > 0 {
		opts.Intersect = q.UidList
	}
	facetsTree, err := preprocessFilter(q.FacetsFilter)
	if err != nil {
		return nil, err
	}

	for i := 0; i < srcFn.n; i++ {
		var key []byte
		if srcFn.fnType == NotAFunction || srcFn.fnType == CompareScalarFn {
			if q.Reverse {
				key = x.ReverseKey(attr, q.UidList.Uids[i])
			} else {
				key = x.DataKey(attr, q.UidList.Uids[i])
			}
		} else if srcFn.fnType == AggregatorFn || srcFn.fnType == PasswordFn {
			key = x.DataKey(attr, q.UidList.Uids[i])
		} else {
			key = x.IndexKey(attr, srcFn.tokens[i])
		}
		// Get or create the posting list for an entity, attribute combination.
		pl, decr := posting.GetOrCreate(key, gid)
		defer decr()
		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		val, err := pl.ValueFor(q.Langs)
		isValueEdge := err == nil
		if val.Tid == types.PasswordID && srcFn.fnType != PasswordFn {
			return nil, x.Errorf("Attribute `%s` of type password cannot be fetched", attr)
		}
		newValue := &taskp.Value{ValType: int32(val.Tid), Val: x.Nilbyte}
		if isValueEdge {
			if typ, err := schema.State().TypeOf(attr); err == nil {
				newValue, err = convertToType(val, typ)
			} else if err != nil {
				// Ideally Schema should be present for already inserted mutation
				// x.Checkf(err, "Schema not defined for attribute %s", attr)
				// Converting to stored type for backward compatiblity of old inserted data
				newValue, err = convertToType(val, val.Tid)
			}
		}
		out.Values = append(out.Values, newValue)

		// get filtered uids and facets.
		type result struct {
			uid    uint64
			facets []*facetsp.Facet
		}
		var filteredRes []*result
		if !isValueEdge { // for uid edge.. get postings
			var perr error
			pl.Postings(opts, func(p *typesp.Posting) bool {
				res := true
				res, perr = applyFacetsTree(p.Facets, facetsTree)
				if perr != nil {
					return false // break loop.
				}
				if res {
					filteredRes = append(filteredRes, &result{
						uid:    p.Uid,
						facets: facets.CopyFacets(p.Facets, q.FacetParam)})
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
					fs = []*facetsp.Facet{}
				}
				out.FacetMatrix = append(out.FacetMatrix,
					&facetsp.List{[]*facetsp.Facets{{fs}}})
			} else {
				var fcsList []*facetsp.Facets
				for _, fres := range filteredRes {
					fcsList = append(fcsList, &facetsp.Facets{fres.facets})
				}
				out.FacetMatrix = append(out.FacetMatrix, &facetsp.List{fcsList})
			}
		}

		// add uids to uidmatrix..
		if q.DoCount || srcFn.fnType == AggregatorFn {
			if q.DoCount {
				out.Counts = append(out.Counts, uint32(pl.Length(0)))
			}
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			continue
		}

		if srcFn.fnType == PasswordFn {
			lastPos := len(out.Values) - 1
			if len(newValue.Val) == 0 {
				out.Values[lastPos] = task.FalseVal
			}
			pwd := q.SrcFunc[2]
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

		if srcFn.fnType == CompareScalarFn {
			count := int64(pl.Length(0))
			if EvalCompare(srcFn.fname, count, srcFn.threshold) {
				tlist := &taskp.List{[]uint64{q.UidList.Uids[i]}}
				out.UidMatrix = append(out.UidMatrix, tlist)
			}
			continue
		}

		// The more usual case: Getting the UIDs.
		uidList := new(taskp.List)
		for _, fres := range filteredRes {
			uidList.Uids = append(uidList.Uids, fres.uid)
		}
		out.UidMatrix = append(out.UidMatrix, uidList)
	}

	if srcFn.fnType == CompareScalarFn && srcFn.isCompareAtRoot {
		f := func(key, val []byte, mu sync.Mutex) {
			pl, decr := posting.GetOrUnmarshal(key, val, gid)
			count := int64(pl.Length(0))
			decr()
			if EvalCompare(srcFn.fname, count, srcFn.threshold) {
				pk := x.Parse(key)
				// TODO: Look if we want to put these UIDs in one list before
				// passing it back to query package.
				tlist := &taskp.List{[]uint64{pk.Uid}}
				mu.Lock()
				out.UidMatrix = append(out.UidMatrix, tlist)
				mu.Unlock()
			}
		}
		iterateParallel(ctx, q, f)
	}

	if srcFn.fnType == RegexFn {
		// Go through the indexkeys for the predicate and match them with
		// the regex matcher.
		typ, err := schema.State().TypeOf(attr)
		if err != nil || !typ.IsScalar() {
			return nil, x.Errorf("Attribute not scalar: %s %v", attr, typ)
		}
		if typ != types.StringID {
			return nil,
				x.Errorf("Got non-string type. Regex match is allowed only on string type.")
		}
		tokenizers := schema.State().TokenizerNames(q.Attr)
		var found bool
		for _, t := range tokenizers {
			if t == "trigram" { // TODO(tzdybal) - maybe just rename to 'regex' tokenizer?
				found = true
			}
		}
		if !found {
			return nil,
				x.Errorf("Attribute %v does not have trigram index for regex matching.", q.Attr)
		}

		query := cindex.RegexpQuery(srcFn.regex.Syntax)
		empty := taskp.List{}
		uids, err := uidsForRegex(attr, gid, query, &empty)
		if uids != nil {
			out.UidMatrix = append(out.UidMatrix, uids)

			var values []types.Val
			for _, uid := range uids.Uids {
				key := x.DataKey(attr, uid)
				pl, decr := posting.GetOrCreate(key, gid)

				var val types.Val
				if len(srcFn.lang) > 0 {
					val, err = pl.ValueForTag(srcFn.lang)
				} else {
					val, err = pl.Value()
				}

				if err != nil {
					decr()
					continue
				}
				// conver data from binary to appropriate format
				strVal, err := types.Convert(val, types.StringID)
				if err == nil {
					values = append(values, strVal)
				}
				decr() // Decrement the reference count of the pl.
			}

			filtered := matchRegex(uids, values, srcFn.regex)
			for i := 0; i < len(out.UidMatrix); i++ {
				algo.IntersectWith(out.UidMatrix[i], filtered, out.UidMatrix[i])
			}
		} else {
			return nil, err
		}
	}

	if srcFn.fnType == CompareAttrFn && len(srcFn.tokens) > 0 &&
		srcFn.ineqValueToken == srcFn.tokens[0] {
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
			return compareTypeVals(q.SrcFunc[0], sv, srcFn.ineqValue)
		})
	}

	// If geo filter, do value check for correctness.
	var values []*taskp.Value
	if srcFn.geoQuery != nil {
		uids := algo.MergeSorted(out.UidMatrix)
		for _, uid := range uids.Uids {
			key := x.DataKey(attr, uid)
			pl, decr := posting.GetOrCreate(key, gid)

			val, err := pl.Value()
			newValue := &taskp.Value{ValType: int32(val.Tid)}
			if err == nil {
				newValue.Val = val.Value.([]byte)
			} else {
				newValue.Val = x.Nilbyte
			}
			values = append(values, newValue)
			decr() // Decrement the reference count of the pl.
		}

		filtered := types.FilterGeoUids(uids, values, srcFn.geoQuery)
		for i := 0; i < len(out.UidMatrix); i++ {
			algo.IntersectWith(out.UidMatrix[i], filtered, out.UidMatrix[i])
		}
	}

	// For string matching functions, check the language.
	if (srcFn.fnType == StandardFn || srcFn.fnType == FullTextSearchFn) && len(srcFn.lang) > 0 {
		uids := algo.MergeSorted(out.UidMatrix)
		var values []types.Val
		for _, uid := range uids.Uids {
			key := x.DataKey(attr, uid)
			pl, decr := posting.GetOrCreate(key, gid)

			val, err := pl.ValueForTag(srcFn.lang)
			if err != nil {
				decr()
				continue
			}
			// conver data from binary to appropriate format
			strVal, err := types.Convert(val, types.StringID)
			if err == nil {
				values = append(values, strVal)
			}
			decr() // Decrement the reference count of the pl.
		}

		filter := stringFilter{
			funcName: srcFn.fname,
			funcType: srcFn.fnType,
			lang:     srcFn.lang,
			tokenMap: map[string]bool{},
		}
		for _, t := range srcFn.tokens {
			filter.tokenMap[t] = false
		}
		filtered := matchStrings(uids, values, filter)
		for i := 0; i < len(out.UidMatrix); i++ {
			algo.IntersectWith(out.UidMatrix[i], filtered, out.UidMatrix[i])
		}
	}

	out.IntersectDest = srcFn.intersectDest
	return &out, nil
}

func matchRegex(uids *taskp.List, values []types.Val, regex *cregexp.Regexp) *taskp.List {
	rv := &taskp.List{}
	for i := 0; i < len(values); i++ {
		if len(values[i].Value.(string)) == 0 {
			continue
		}

		if regex.MatchString(values[i].Value.(string), true, true) > 0 {
			rv.Uids = append(rv.Uids, uids.Uids[i])
		}
	}

	return rv
}

type functionContext struct {
	tokens          []string
	geoQuery        *types.GeoQueryData
	intersectDest   bool
	ineqValue       types.Val
	ineqValueToken  string
	n               int
	threshold       int64
	fname           string
	lang            string
	fnType          FuncType
	regex           *cregexp.Regexp
	isCompareAtRoot bool
}

func ensureArgsCount(funcStr []string, expected int) error {
	actual := len(funcStr) - 2
	switch {
	case actual == 0:
		return x.Errorf("No arguments passed to function '%s'", funcStr[0])
	case actual != expected:
		return x.Errorf("Function '%s' requires %d arguments, but got %d (%v)",
			funcStr[0], expected, actual, funcStr[2:])
	default:
		return nil
	}
}

func parseSrcFn(q *taskp.Query) (*functionContext, error) {
	fnType, f := parseFuncType(q.SrcFunc)
	attr := q.Attr
	fc := &functionContext{fnType: fnType, fname: f}
	var err error

	switch fnType {
	case NotAFunction:
		fc.n = len(q.UidList.Uids)
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
		fc.n = len(q.UidList.Uids)
	case CompareAttrFn:
		err = ensureArgsCount(q.SrcFunc, 1)
		if err != nil {
			return nil, err
		}
		typ, err := schema.State().TypeOf(attr)
		if typ == types.BoolID && fc.fname != "eq" {
			return nil, x.Errorf("Only eq operator defined for type bool. Got: %v", fc.fname)
		}
		fc.ineqValue, err = convertValue(attr, q.SrcFunc[2])
		if err != nil {
			return nil, x.Errorf("Got error: %v while running: %v", err.Error(), q.SrcFunc)
		}
		// Get tokens ge / le ineqValueToken.
		fc.tokens, fc.ineqValueToken, err = getInequalityTokens(attr, f, fc.ineqValue)
		if err != nil {
			return nil, err
		}
		fc.n = len(fc.tokens)
	case CompareScalarFn:
		err = ensureArgsCount(q.SrcFunc, 2)
		if err != nil {
			return nil, err
		}
		fc.threshold, err = strconv.ParseInt(q.SrcFunc[3], 10, 64)
		if err != nil {
			return nil, x.Wrapf(err, "Compare %v(%v) require digits, but got invalid num",
				q.SrcFunc[0], q.SrcFunc[2])
		}
		if q.UidList == nil {
			// Fetch Uids from Store and populate in q.UidList.
			fc.n = 0
			fc.isCompareAtRoot = true
		} else {
			fc.n = len(q.UidList.Uids)
		}
	case GeoFn:
		// For geo functions, we get extra information used for filtering.
		fc.tokens, fc.geoQuery, err = types.GetGeoTokens(q.SrcFunc)
		tok.EncodeGeoTokens(fc.tokens)
		if err != nil {
			return nil, err
		}
		fc.n = len(fc.tokens)
	case PasswordFn:
		err = ensureArgsCount(q.SrcFunc, 2)
		if err != nil {
			return nil, err
		}
		fc.n = len(q.UidList.Uids)
	case StandardFn, FullTextSearchFn:
		// srcfunc 0th val is func name and and [2:] are args.
		// we tokenize the arguments of the query.
		err = ensureArgsCount(q.SrcFunc, 1)
		if err != nil {
			return nil, err
		}
		required, found := verifyStringIndex(attr, fnType)
		if !found {
			return nil, x.Errorf("Attribute %s is not indexed with type %s", attr, required)
		}
		fc.tokens, err = getStringTokens(q.SrcFunc[2:], q.SrcFunc[1], fnType)
		if err != nil {
			return nil, err
		}
		fnName := strings.ToLower(q.SrcFunc[0])
		fc.lang = q.SrcFunc[1]
		fc.intersectDest = strings.HasPrefix(fnName, "allof") // allofterms and alloftext
		fc.n = len(fc.tokens)
	case RegexFn:
		err = ensureArgsCount(q.SrcFunc, 2)
		if err != nil {
			return nil, err
		}
		ignoreCase := q.SrcFunc[3] == "i"
		matchType := "(?m)" // this is cregexp library specific
		if ignoreCase {
			matchType = "(?i)" + matchType
		}
		fc.regex, err = cregexp.Compile(matchType + q.SrcFunc[2])
		if err != nil {
			return nil, err
		}
		fc.n = 0
		fc.lang = q.SrcFunc[1]
	default:
		return nil, x.Errorf("FnType %d not handled in numFnAttrs.", fnType)
	}
	return fc, nil
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, q *taskp.Query) (*taskp.Result, error) {
	if ctx.Err() != nil {
		return &emptyResult, ctx.Err()
	}

	gid := group.BelongsTo(q.Attr)
	x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr, len(q.UidList.Uids), gid)

	var reply *taskp.Result
	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", q.Attr, gid)

	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = processTask(ctx, q, gid)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}

// applyFacetsTree : we return error only when query has some problems.
// like Or has 3 arguments, argument facet val overflows integer.
// returns true if postingFacets can be included.
func applyFacetsTree(postingFacets []*facetsp.Facet, ftree *facetsTree) (bool, error) {
	if ftree == nil {
		return true, nil
	}
	if ftree.function != nil {
		fname := strings.ToLower(ftree.function.name)
		var fc *facetsp.Facet
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
		switch fnType {
		case CompareAttrFn: // lt, gt, le, ge, eq
			return compareTypeVals(fname, facets.ValFor(fc), ftree.function.val), nil

		case StandardFn: // allofterms, anyofterms
			if facets.TypeIDForValType(fc.ValType) != facets.StringID {
				return false, nil
			}
			return filterOnStandardFn(fname, fc.Tokens, ftree.function.tokens)
		}
		return false, x.Errorf("Fn %s not supported in facets filtering.", fname)
	}

	var res []bool
	for _, c := range ftree.children {
		r, err := applyFacetsTree(postingFacets, c)
		if err != nil {
			return false, err
		}
		res = append(res, r)
	}

	// we have already checked for number of children in preprocessFilter
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
	case "ge":
		return noError(revRes(types.Less(arg1, arg2)))
	case "gt":
		return noError(types.Less(arg2, arg1))
	case "le":
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
	case "allofterms":
		// allofterms argTokens should be in fcTokens
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
	case "anyofterms":
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

func preprocessFilter(tree *facetsp.FilterTree) (*facetsTree, error) {
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
		if len(tree.Func.Args) != 1 {
			return nil, x.Errorf("One argument expected in %s, but got %d.",
				fname, len(tree.Func.Args))
		}

		switch fnType {
		case CompareAttrFn:
			argf, err := facets.FacetFor(tree.Func.Key, tree.Func.Args[0])
			if err != nil {
				return nil, err // stop processing as this is query error
			}
			ftree.function.val = facets.ValFor(argf)
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

type itkv struct {
	key []byte
	val []byte
}

func iterateParallel(ctx context.Context, q *taskp.Query, f func([]byte, []byte, sync.Mutex)) {
	grpSize := uint64(math.MaxUint64 / uint64(numPart))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := uint64(0); i < numPart; i++ {
		minUid := grpSize*i + 1
		maxUid := grpSize * (i + 1)
		if i == numPart-1 {
			maxUid = math.MaxUint64
		}
		x.Trace(ctx, "Running go-routine %v for iteration", i)
		wg.Add(1)
		go func() {
			it := pstore.NewIterator()
			defer it.Close()
			startKey := x.DataKey(q.Attr, minUid)
			pk := x.Parse(startKey)
			prefix := pk.DataPrefix()
			if q.Reverse {
				startKey = x.ReverseKey(q.Attr, minUid)
				pk = x.Parse(startKey)
				prefix = pk.ReversePrefix()
			}

			w := 0
			for it.Seek(startKey); it.ValidForPrefix(prefix); it.Next() {
				pk := x.Parse(it.Key().Data())
				x.AssertTruef(pk.Attr == q.Attr,
					"Invalid key obtained for comparison")
				if w%1000 == 0 {
					x.Trace(ctx, "iterateParallel: go-routine-id: %v key: %v:%v", i, pk.Attr, pk.Uid)
				}
				w++
				if pk.Uid > maxUid {
					break
				}
				key := it.Key().Data()
				val := it.Value().Data()
				f(key, val, mu)
			}
			wg.Done()
		}()
	}
	wg.Wait()
}
