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
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
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
	emptyUIDList protos.List
	emptyResult  protos.Result
	regexTok     tok.ExactTokenizer
)

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, q *protos.Query) (*protos.Result, error) {
	attr := q.Attr
	gid := group.BelongsTo(attr)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("attr: %v groupId: %v", attr, gid)
	}

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processTask(ctx, q, gid)
	}

	// Send this over the network.
	// TODO: Send the request to multiple servers as described in Jeff Dean's talk.
	addr := groups().AnyServer(gid)
	pl, err := pools().get(addr)
	if err != nil {
		return &emptyResult, x.Wrapf(err, "ProcessTaskOverNetwork: while retrieving connection.")
	}
	defer pools().release(pl)

	conn := pl.Get()
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Sending request to %v", addr)
	}

	c := protos.NewWorkerClient(conn)
	reply, err := c.ServeTask(ctx, q)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while calling Worker.ServeTask: %v", err)
		}
		return &emptyResult, err
	}

	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Reply from server. length: %v Addr: %v Attr: %v",
			len(reply.UidMatrix), addr, attr)
	}
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
func convertToType(v types.Val, typ types.TypeID) (*protos.TaskValue, error) {
	result := &protos.TaskValue{ValType: int32(typ), Val: x.Nilbyte}
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
	HasFn
	UidInFn
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
	case "has":
		return HasFn, f
	case "uid_in":
		return UidInFn, f
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

func getPredList(uid uint64, gid uint32) ([]types.Val, error) {
	key := x.DataKey("_predicate_", uid)
	// Get or create the posting list for an entity, attribute combination.
	pl, decr := posting.GetOrCreate(key, gid)
	defer decr()
	return pl.AllValues()
}

type result struct {
	uid    uint64
	facets []*protos.Facet
}

func addUidToMatrix(key []byte, mu *sync.Mutex, out *protos.Result) {
	pk := x.Parse(key)
	tlist := &protos.List{[]uint64{pk.Uid}}
	mu.Lock()
	out.UidMatrix = append(out.UidMatrix, tlist)
	mu.Unlock()
}

// processTask processes the query, accumulates and returns the result.
func processTask(ctx context.Context, q *protos.Query, gid uint32) (*protos.Result, error) {
	out := new(protos.Result)
	attr := q.Attr

	if attr == "_predicate_" {
		predMap := make(map[string]struct{})
		if q.UidList == nil {
			return out, nil
		}
		for _, uid := range q.UidList.Uids {
			predicates, err := getPredList(uid, gid)
			if err != nil {
				return out, err
			}
			for _, pred := range predicates {
				predMap[string(pred.Value.([]byte))] = struct{}{}
			}
		}
		predList := make([]string, 0, len(predMap))
		for pred := range predMap {
			predList = append(predList, pred)
		}
		sort.Strings(predList)
		for _, pred := range predList {
			// Add it to values.
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			out.Values = append(out.Values, &protos.TaskValue{
				ValType: int32(types.StringID),
				Val:     []byte(pred),
			})
		}
		return out, nil
	}

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

	type plf struct {
		pl   *posting.List
		decr func()
	}
	plch := make(chan plf, 1000)
	stopCh := make(chan struct{}, 1)
	go func(q *protos.Query, srcFn *functionContext) {
	L:
		for i := 0; i < srcFn.n; i++ {
			select {
			case <-stopCh:
				break L
			default:
			}
			var key []byte
			if srcFn.fnType == NotAFunction || srcFn.fnType == CompareScalarFn ||
				srcFn.fnType == HasFn || srcFn.fnType == UidInFn {
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
			plch <- plf{pl, decr}
			//defer decr()
		}
		close(plch)
	}(q, srcFn)

	i := -1
	var lerr error
	var lastDecr func()
BL:
	// NOTE: Never return inside this loop
	for it := range plch {
		i++
		if lastDecr != nil {
			lastDecr()
		}
		pl := it.pl
		lastDecr = it.decr
		// If a posting list contains a value, we store that or else we store a nil
		// byte so that processing is consistent later.
		val, err := pl.ValueFor(q.Langs)
		isValueEdge := err == nil
		if val.Tid == types.PasswordID && srcFn.fnType != PasswordFn {
			lerr = x.Errorf("Attribute `%s` of type password cannot be fetched", attr)
			stopCh <- struct{}{}
			break BL
		}
		newValue := &protos.TaskValue{ValType: int32(val.Tid), Val: x.Nilbyte}
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
		var filteredRes []*result
		if !isValueEdge { // for uid edge.. get postings
			var perr error
			filteredRes = make([]*result, 0, pl.Length(opts.AfterUID))
			pl.Postings(opts, func(p *protos.Posting) bool {
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
				lerr = perr
				stopCh <- struct{}{}
				break BL
			}
		} else if q.FacetsFilter != nil { // else part means isValueEdge
			// This is Value edge and we are asked to do facet filtering. Not supported.
			lerr = x.Errorf("Facet filtering is not supported on values.")
			stopCh <- struct{}{}
			break BL
		}

		// add facets to result.
		if q.FacetParam != nil {
			if isValueEdge {
				fs, err := pl.Facets(q.FacetParam, q.Langs)
				if err != nil {
					fs = []*protos.Facet{}
				}
				out.FacetMatrix = append(out.FacetMatrix,
					&protos.FacetsList{[]*protos.Facets{{fs}}})
			} else {
				var fcsList []*protos.Facets
				for _, fres := range filteredRes {
					fcsList = append(fcsList, &protos.Facets{fres.facets})
				}
				out.FacetMatrix = append(out.FacetMatrix, &protos.FacetsList{fcsList})
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
				tlist := &protos.List{[]uint64{q.UidList.Uids[i]}}
				out.UidMatrix = append(out.UidMatrix, tlist)
			}
			continue
		}

		if srcFn.fnType == HasFn {
			count := int64(pl.Length(0))
			if EvalCompare("gt", count, 0) {
				tlist := &protos.List{[]uint64{q.UidList.Uids[i]}}
				out.UidMatrix = append(out.UidMatrix, tlist)
			}
			continue
		}

		if srcFn.fnType == UidInFn {
			reqList := &protos.List{[]uint64{srcFn.uidPresent}}
			topts := posting.ListOptions{
				AfterUID:  0,
				Intersect: reqList,
			}
			plist := pl.Uids(topts)
			if len(plist.Uids) > 0 {
				tlist := &protos.List{[]uint64{q.UidList.Uids[i]}}
				out.UidMatrix = append(out.UidMatrix, tlist)
			}
			continue
		}

		// The more usual case: Getting the UIDs.
		uidList := new(protos.List)
		for _, fres := range filteredRes {
			uidList.Uids = append(uidList.Uids, fres.uid)
		}
		out.UidMatrix = append(out.UidMatrix, uidList)
	}

	for it := range plch {
		// Some items might be left after error. decr() their reference.
		it.decr()
	}

	if lerr != nil {
		return nil, lerr
	}

	if srcFn.fnType == HasFn && srcFn.isFuncAtRoot {
		if ok := schema.State().HasCount(attr); !ok {
			return nil, x.Errorf("Need @count directive in schema for attr: %s for fn: %s at root.",
				attr, srcFn.fname)
		}
		cp := countParams{
			count:   0,
			fn:      "gt",
			attr:    attr,
			gid:     gid,
			reverse: q.Reverse,
		}
		cp.evaluate(out)
	}

	if srcFn.fnType == CompareScalarFn && srcFn.isFuncAtRoot {
		if ok := schema.State().HasCount(attr); !ok {
			return nil, x.Errorf("Need @count directive in schema for attr: %s for fn: %s at root",
				attr, srcFn.fname)
		}
		count := srcFn.threshold
		cp := countParams{
			fn:      srcFn.fname,
			count:   count,
			attr:    attr,
			gid:     gid,
			reverse: q.Reverse,
		}
		cp.evaluate(out)
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
		empty := protos.List{}
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

	// We fetch the actual value for the uids, compare them to the value in the
	// request and filter the uids only if the tokenizer IsLossy.
	if srcFn.fnType == CompareAttrFn && len(srcFn.tokens) > 0 {
		tokenizer, err := pickTokenizer(attr, srcFn.fname)
		// We should already have checked this in getInequalityTokens.
		x.Check(err)
		// Only if the tokenizer that we used IsLossy, then we need to fetch
		// and compare the actual values.
		if tokenizer.IsLossy() {
			// Need to evaluate inequality for entries in the first bucket.
			typ, err := schema.State().TypeOf(attr)
			if err != nil || !typ.IsScalar() {
				return nil, x.Errorf("Attribute not scalar: %s %v", attr, typ)
			}

			x.AssertTrue(len(out.UidMatrix) > 0)
			rowsToFilter := 0
			if srcFn.fname == eq {
				// If fn is eq, we could have multiple arguments and hence multiple rows
				// to filter.
				rowsToFilter = len(srcFn.tokens)
			} else if srcFn.tokens[0] == srcFn.ineqValueToken {
				// If operation is not eq and ineqValueToken equals first token,
				// then we need to filter first row..
				rowsToFilter = 1
			}
			for row := 0; row < rowsToFilter; row++ {
				algo.ApplyFilter(out.UidMatrix[row], func(uid uint64, i int) bool {
					var langs []string
					if len(srcFn.lang) > 0 {
						langs = append(langs, srcFn.lang)
					}
					sv, err := fetchValue(uid, attr, langs, typ)
					if sv.Value == nil || err != nil {
						return false
					}
					return types.CompareVals(q.SrcFunc[0], sv, srcFn.eqTokens[row])
				})
			}
		}
	}

	// If geo filter, do value check for correctness.
	var values []*protos.TaskValue
	if srcFn.geoQuery != nil {
		uids := algo.MergeSorted(out.UidMatrix)
		for _, uid := range uids.Uids {
			key := x.DataKey(attr, uid)
			pl, decr := posting.GetOrCreate(key, gid)

			val, err := pl.Value()
			newValue := &protos.TaskValue{ValType: int32(val.Tid)}
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
	if (srcFn.fnType == StandardFn || srcFn.fnType == HasFn ||
		srcFn.fnType == FullTextSearchFn || srcFn.fnType == CompareAttrFn) &&
		len(srcFn.lang) > 0 {
		uids := algo.MergeSorted(out.UidMatrix)
		var values []types.Val
		filteredUids := make([]uint64, 0, len(uids.Uids))
		for _, uid := range uids.Uids {
			key := x.DataKey(attr, uid)
			pl, decr := posting.GetOrCreate(key, gid)

			val, err := pl.ValueForTag(srcFn.lang)
			if err != nil {
				decr()
				continue
			}
			// convert data from binary to appropriate format
			strVal, err := types.Convert(val, types.StringID)
			if err == nil {
				values = append(values, strVal)
			}
			filteredUids = append(filteredUids, uid)
			decr() // Decrement the reference count of the pl.
		}

		filtered := &protos.List{Uids: filteredUids}
		filter := stringFilter{
			funcName: srcFn.fname,
			funcType: srcFn.fnType,
			lang:     srcFn.lang,
		}

		switch srcFn.fnType {
		case HasFn:
			// Dont do anything, as filtering based on lang is already
			// done above.
		case FullTextSearchFn, StandardFn:
			filter.tokens = srcFn.tokens
			filter.match = defaultMatch
			filtered = matchStrings(filtered, values, filter)
		case CompareAttrFn:
			filter.ineqValue = srcFn.ineqValue
			filter.match = ineqMatch
			filtered = matchStrings(uids, values, filter)
		}

		for i := 0; i < len(out.UidMatrix); i++ {
			algo.IntersectWith(out.UidMatrix[i], filtered, out.UidMatrix[i])
		}
	}

	out.IntersectDest = srcFn.intersectDest
	return out, nil
}

func matchRegex(uids *protos.List, values []types.Val, regex *cregexp.Regexp) *protos.List {
	rv := &protos.List{}
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
	tokens         []string
	geoQuery       *types.GeoQueryData
	intersectDest  bool
	ineqValue      types.Val
	eqTokens       []types.Val
	ineqValueToken string
	n              int
	threshold      int64
	uidPresent     uint64
	fname          string
	lang           string
	fnType         FuncType
	regex          *cregexp.Regexp
	isFuncAtRoot   bool
}

const (
	eq = "eq" // equal
)

func ensureArgsCount(funcStr []string, expected int) error {
	actual := len(funcStr) - 2
	if actual != expected {
		return x.Errorf("Function '%s' requires %d arguments, but got %d (%v)",
			funcStr[0], expected, actual, funcStr[2:])
	}
	return nil
}

func checkRoot(q *protos.Query, fc *functionContext) {
	if q.UidList == nil {
		// Fetch Uids from Store and populate in q.UidList.
		fc.n = 0
		fc.isFuncAtRoot = true
	} else {
		fc.n = len(q.UidList.Uids)
	}
}

func parseSrcFn(q *protos.Query) (*functionContext, error) {
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
		args := q.SrcFunc[2:]
		// Only eq can have multiple args. It should have atleast one.
		if fc.fname == eq {
			if len(args) <= 0 {
				return nil, x.Errorf("eq expects atleast 1 argument.")
			}
		} else { // Others can have only 1 arg.
			if len(args) != 1 {
				return nil, x.Errorf("%+v expects only 1 argument. Got: %+v",
					fc.fname, args)
			}
		}

		var tokens []string
		// eq can have multiple args.
		for _, arg := range args {
			if fc.ineqValue, err = convertValue(attr, arg); err != nil {
				return nil, x.Errorf("Got error: %v while running: %v", err,
					q.SrcFunc)
			}
			// Get tokens ge / le ineqValueToken.
			if tokens, fc.ineqValueToken, err = getInequalityTokens(attr, f,
				fc.ineqValue); err != nil {
				return nil, err
			}
			if len(tokens) == 0 {
				continue
			}
			fc.tokens = append(fc.tokens, tokens...)
			fc.eqTokens = append(fc.eqTokens, fc.ineqValue)
		}
		fc.n = len(fc.tokens)
		fc.lang = q.SrcFunc[1]
	case CompareScalarFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		if fc.threshold, err = strconv.ParseInt(q.SrcFunc[3], 0, 64); err != nil {
			return nil, x.Wrapf(err, "Compare %v(%v) require digits, but got invalid num",
				q.SrcFunc[0], q.SrcFunc[2])
		}
		checkRoot(q, fc)
	case GeoFn:
		// For geo functions, we get extra information used for filtering.
		fc.tokens, fc.geoQuery, err = types.GetGeoTokens(q.SrcFunc)
		tok.EncodeGeoTokens(fc.tokens)
		if err != nil {
			return nil, err
		}
		fc.n = len(fc.tokens)
	case PasswordFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		fc.n = len(q.UidList.Uids)
	case StandardFn, FullTextSearchFn:
		// srcfunc 0th val is func name and and [2:] are args.
		// we tokenize the arguments of the query.
		if err = ensureArgsCount(q.SrcFunc, 1); err != nil {
			return nil, err
		}
		required, found := verifyStringIndex(attr, fnType)
		if !found {
			return nil, x.Errorf("Attribute %s is not indexed with type %s", attr, required)
		}
		if fc.tokens, err = getStringTokens(q.SrcFunc[2:], q.SrcFunc[1], fnType); err != nil {
			return nil, err
		}
		fnName := strings.ToLower(q.SrcFunc[0])
		fc.lang = q.SrcFunc[1]
		fc.intersectDest = strings.HasPrefix(fnName, "allof") // allofterms and alloftext
		fc.n = len(fc.tokens)
	case RegexFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		ignoreCase := false
		modifiers := q.SrcFunc[3]
		if len(modifiers) > 0 {
			if modifiers == "i" {
				ignoreCase = true
			} else {
				return nil, x.Errorf("Invalid regexp modifier: %s", modifiers)
			}
		}
		matchType := "(?m)" // this is cregexp library specific
		if ignoreCase {
			matchType = "(?i)" + matchType
		}
		if fc.regex, err = cregexp.Compile(matchType + q.SrcFunc[2]); err != nil {
			return nil, err
		}
		fc.n = 0
		fc.lang = q.SrcFunc[1]
	case HasFn:
		if err = ensureArgsCount(q.SrcFunc, 0); err != nil {
			return nil, err
		}
		checkRoot(q, fc)
		fc.lang = q.SrcFunc[1]
	case UidInFn:
		if err = ensureArgsCount(q.SrcFunc, 1); err != nil {
			return nil, err
		}
		if fc.uidPresent, err = strconv.ParseUint(q.SrcFunc[2], 0, 64); err != nil {
			return nil, err
		}
		checkRoot(q, fc)
		if fc.isFuncAtRoot {
			return nil, x.Errorf("uid_in function not allowed at root")
		}
	default:
		return nil, x.Errorf("FnType %d not handled in numFnAttrs.", fnType)
	}
	return fc, nil
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, q *protos.Query) (*protos.Result, error) {
	if ctx.Err() != nil {
		return &emptyResult, ctx.Err()
	}

	gid := group.BelongsTo(q.Attr)
	var numUids int
	if q.UidList != nil {
		numUids = len(q.UidList.Uids)
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr, numUids, gid)
	}

	var reply *protos.Result
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
func applyFacetsTree(postingFacets []*protos.Facet, ftree *facetsTree) (bool, error) {
	if ftree == nil {
		return true, nil
	}
	if ftree.function != nil {
		fname := strings.ToLower(ftree.function.name)
		var fc *protos.Facet
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
			var err error
			typId := facets.TypeIDFor(fc)
			v, has := ftree.function.convertedVal[typId]
			if !has {
				if v, err = types.Convert(ftree.function.val, typId); err != nil {
					// ignore facet if not of appropriate type
					return false, nil
				} else {
					ftree.function.convertedVal[typId] = v
				}
			}
			return types.CompareVals(fname, facets.ValFor(fc), v), nil

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
	// convertedVal is used to cache the converted value of val for each type
	convertedVal map[types.TypeID]types.Val
}
type facetsTree struct {
	op       string
	children []*facetsTree
	function *facetsFunc
}

func preprocessFilter(tree *protos.FilterTree) (*facetsTree, error) {
	if tree == nil {
		return nil, nil
	}
	ftree := &facetsTree{}
	ftree.op = tree.Op
	if tree.Func != nil {
		ftree.function = &facetsFunc{}
		ftree.function.convertedVal = make(map[types.TypeID]types.Val)
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
			ftree.function.val = types.Val{Tid: types.StringID, Value: []byte(tree.Func.Args[0])}
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

type countParams struct {
	count   int64
	attr    string
	gid     uint32
	reverse bool   // If query is asking for ~pred
	fn      string // function name
}

func (cp *countParams) evaluate(out *protos.Result) {
	count := cp.count
	countKey := x.CountKey(cp.attr, uint32(count), cp.reverse)
	if cp.fn == "eq" {
		pl, decr := posting.GetOrCreate(countKey, cp.gid)
		defer decr()
		out.UidMatrix = append(out.UidMatrix, pl.Uids(posting.ListOptions{}))
		return
	}

	if cp.fn == "lt" {
		count -= 1
	} else if cp.fn == "gt" {
		count += 1
	}

	if count < 0 && (cp.fn == "lt" || cp.fn == "le") {
		return
	}

	if count < 0 {
		count = 0
	}
	countKey = x.CountKey(cp.attr, uint32(count), cp.reverse)

	itOpt := badger.DefaultIteratorOptions
	itOpt.FetchValues = false
	itOpt.Reverse = cp.fn == "le" || cp.fn == "lt"
	it := pstore.NewIterator(itOpt)
	defer it.Close()
	pk := x.ParsedKey{
		Attr: cp.attr,
	}
	countPrefix := pk.CountPrefix(cp.reverse)

	for it.Seek(countKey); it.ValidForPrefix(countPrefix); it.Next() {
		key := it.Item().Key()
		pl, decr := posting.GetOrCreate(key, cp.gid)
		defer decr()
		out.UidMatrix = append(out.UidMatrix, pl.Uids(posting.ListOptions{}))
	}
}
