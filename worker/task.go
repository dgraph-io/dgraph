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
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/coreos/etcd/raft"
	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	ctask "github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"

	cindex "github.com/google/codesearch/index"
	cregexp "github.com/google/codesearch/regexp"
)

var (
	emptyUIDList   protos.List
	emptyResult    protos.Result
	emptyValueList = protos.ValueList{Values: []*protos.TaskValue{&protos.TaskValue{Val: x.Nilbyte}}}
)

func invokeNetworkRequest(
	ctx context.Context, addr string, f func(context.Context, protos.WorkerClient) (interface{}, error)) (interface{}, error) {
	pl, err := conn.Get().Get(addr)
	if err != nil {
		return &emptyResult, x.Wrapf(err, "dispatchTaskOverNetwork: while retrieving connection.")
	}

	conn := pl.Get()
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Sending request to %v", addr)
	}
	c := protos.NewWorkerClient(conn)
	return f(ctx, c)
}

const backupRequestGracePeriod = 10 * time.Millisecond

// TODO: Cross-server cancellation as described in Jeff Dean's talk.
func processWithBackupRequest(
	ctx context.Context,
	gid uint32,
	f func(context.Context, protos.WorkerClient) (interface{}, error)) (interface{}, error) {
	addrs := groups().AnyTwoServers(gid)
	if len(addrs) == 0 {
		return nil, errors.New("no network connection")
	}
	if len(addrs) == 1 {
		reply, err := invokeNetworkRequest(ctx, addrs[0], f)
		return reply, err
	}
	type taskresult struct {
		reply interface{}
		err   error
	}

	chResults := make(chan taskresult, len(addrs))
	ctx0, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		reply, err := invokeNetworkRequest(ctx0, addrs[0], f)
		chResults <- taskresult{reply, err}
	}()
	timer := time.NewTimer(backupRequestGracePeriod)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		go func() {
			reply, err := invokeNetworkRequest(ctx0, addrs[1], f)
			chResults <- taskresult{reply, err}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-chResults:
			if result.err != nil {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case result := <-chResults:
					return result.reply, result.err
				}
			} else {
				return result.reply, nil
			}
		}
	case result := <-chResults:
		if result.err != nil {
			cancel() // Might as well cleanup resources ASAP
			timer.Stop()
			return invokeNetworkRequest(ctx, addrs[1], f)
		}
		return result.reply, nil
	}
}

// ProcessTaskOverNetwork is used to process the query and get the result from
// the instance which stores posting list corresponding to the predicate in the
// query.
func ProcessTaskOverNetwork(ctx context.Context, q *protos.Query) (*protos.Result, error) {
	attr := q.Attr
	gid := groups().BelongsTo(attr)
	if gid == 0 {
		return &protos.Result{}, errUnservedTablet
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("attr: %v groupId: %v", attr, gid)
	}

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processTask(ctx, q, gid)
	}

	result, err := processWithBackupRequest(ctx, gid, func(ctx context.Context, c protos.WorkerClient) (interface{}, error) {
		if tr, ok := trace.FromContext(ctx); ok {
			id := fmt.Sprintf("%d", rand.Int())
			tr.LazyPrintf("Sending request to server, id: %s", id)
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("trace", id))
		}
		return c.ServeTask(ctx, q)
	})
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while worker.ServeTask: %v", err)
		}
		return nil, err
	}
	reply := result.(*protos.Result)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Reply from server. length: %v Group: %v Attr: %v", len(reply.UidMatrix), gid, attr)
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

	// convert data from binary to appropriate format
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

func parseFuncType(srcFunc *protos.SrcFunction) (FuncType, string) {
	if srcFunc == nil {
		return NotAFunction, ""
	}
	ftype, fname := parseFuncTypeHelper(srcFunc.Name)
	if srcFunc.IsCount && ftype == CompareAttrFn {
		// gt(release_date, "1990") is 'CompareAttr' which
		//    takes advantage of indexed-attr
		// gt(count(films), 0) is 'CompareScalar', we first do
		//    counting on attr, then compare the result as scalar with int
		return CompareScalarFn, fname
	}
	return ftype, fname
}

func parseFuncTypeHelper(name string) (FuncType, string) {
	if len(name) == 0 {
		return NotAFunction, ""
	}
	f := strings.ToLower(name)
	switch f {
	case "le", "ge", "lt", "gt", "eq":
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

type result struct {
	uid    uint64
	facets []*protos.Facet
}

type funcArgs struct {
	q     *protos.Query
	gid   uint32
	srcFn *functionContext
	out   *protos.Result
}

// The function tells us whether we want to fetch value posting lists or uid posting lists.
func (srcFn *functionContext) needsValuePostings(typ types.TypeID) (bool, error) {
	switch srcFn.fnType {
	case AggregatorFn, PasswordFn:
		return true, nil
	case CompareAttrFn:
		if len(srcFn.tokens) > 0 {
			return false, nil
		}
		return true, nil
	case GeoFn, RegexFn, FullTextSearchFn, StandardFn, HasFn:
		// All of these require index, hence would require fetching uid postings.
		return false, nil
	case UidInFn, CompareScalarFn:
		// Operate on uid postings
		return false, nil
	case NotAFunction:
		return typ.IsScalar(), nil
	default:
		return false, x.Errorf("Unhandled case in fetchValuePostings for fn: %s", srcFn.fname)
	}
	return true, nil
}

// Handles fetching of value posting lists and filtering of uids based on that.
func handleValuePostings(ctx context.Context, args funcArgs) error {
	srcFn := args.srcFn
	q := args.q
	attr := q.Attr
	out := args.out

	switch srcFn.fnType {
	case NotAFunction, AggregatorFn, PasswordFn, CompareAttrFn:
	default:
		return x.Errorf("Unhandled function in handleValuePostings: %s", srcFn.fname)
	}

	var key []byte
	var err error
	listType := schema.State().IsList(attr)
	for i := 0; i < srcFn.n; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		key = x.DataKey(attr, q.UidList.Uids[i])

		// Get or create the posting list for an entity, attribute combination.
		pl := posting.Get(key)
		var vals []types.Val
		// Even if its a list type and value is asked in a language we return that.
		if listType && len(q.Langs) == 0 {
			vals, err = pl.AllValues()
		} else {
			var val types.Val
			val, err = pl.ValueFor(q.Langs)
			if val.Tid == types.PasswordID && srcFn.fnType != PasswordFn {
				return x.Errorf("Attribute `%s` of type password cannot be fetched", attr)
			}
			vals = append(vals, val)
		}

		if err != nil || len(vals) == 0 {
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			out.ValueMatrix = append(out.ValueMatrix, &emptyValueList)
			continue
		}

		valTid := vals[0].Tid
		newValue := &protos.TaskValue{ValType: int32(valTid), Val: x.Nilbyte}
		uidList := new(protos.List)
		var vl protos.ValueList
		for _, val := range vals {
			newValue, err = convertToType(val, srcFn.atype)

			// This means we fetched the value directly instead of fetching index key and intersecting.
			// Lets compare the value and add filter the uid.
			if srcFn.fnType == CompareAttrFn {
				// Lets convert the val to its type.
				if val, err = types.Convert(val, srcFn.atype); err != nil {
					return err
				}
				if types.CompareVals(srcFn.fname, val, srcFn.ineqValue) {
					uidList.Uids = append(uidList.Uids, q.UidList.Uids[i])
					break
				}
			} else {
				vl.Values = append(vl.Values, newValue)
			}
		}
		out.ValueMatrix = append(out.ValueMatrix, &vl)

		if q.FacetsFilter != nil { // else part means isValueEdge
			// This is Value edge and we are asked to do facet filtering. Not supported.
			return x.Errorf("Facet filtering is not supported on values.")
		}

		// add facets to result.
		if q.FacetParam != nil {
			fs, err := pl.Facets(q.FacetParam, q.Langs)
			if err != nil {
				fs = []*protos.Facet{}
			}
			out.FacetMatrix = append(out.FacetMatrix,
				&protos.FacetsList{[]*protos.Facets{{fs}}})
		}

		switch {
		case q.DoCount:
			out.Counts = append(out.Counts, uint32(pl.Length(0)))
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
		case srcFn.fnType == AggregatorFn:
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
		case srcFn.fnType == PasswordFn:
			lastPos := len(out.ValueMatrix) - 1
			newValue := out.ValueMatrix[lastPos].Values[0]
			if len(newValue.Val) == 0 {
				// TODO - Check that this is safe.
				out.ValueMatrix[lastPos].Values[0] = ctask.FalseVal
			}
			pwd := q.SrcFunc.Args[0]
			err = types.VerifyPassword(pwd, string(newValue.Val))
			if err != nil {
				out.ValueMatrix[lastPos].Values[0] = ctask.FalseVal
			} else {
				out.ValueMatrix[lastPos].Values[0] = ctask.TrueVal
			}
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
		default:
			out.UidMatrix = append(out.UidMatrix, uidList)
		}
	}
	return nil
}

// This function handles operations on uid posting lists. Index keys, reverse keys and some data
// keys store uid posting lists.
func handleUidPostings(ctx context.Context, args funcArgs, opts posting.ListOptions) error {
	srcFn := args.srcFn
	q := args.q
	attr := q.Attr
	out := args.out

	facetsTree, err := preprocessFilter(q.FacetsFilter)
	if err != nil {
		return err
	}

	for i := 0; i < srcFn.n; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var key []byte
		switch srcFn.fnType {
		case NotAFunction, CompareScalarFn, HasFn, UidInFn:
			if q.Reverse {
				key = x.ReverseKey(attr, q.UidList.Uids[i])
			} else {
				key = x.DataKey(attr, q.UidList.Uids[i])
			}
		case GeoFn, RegexFn, FullTextSearchFn, StandardFn:
			key = x.IndexKey(attr, srcFn.tokens[i])
		case CompareAttrFn:
			key = x.IndexKey(attr, srcFn.tokens[i])
		default:
			return x.Errorf("Unhandled function in handleUidPostings: %s", srcFn.fname)
		}

		// Get or create the posting list for an entity, attribute combination.
		pl := posting.Get(key)

		// get filtered uids and facets.
		var filteredRes []*result
		out.ValueMatrix = append(out.ValueMatrix, &emptyValueList)

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
			return perr
		}

		// add facets to result.
		if q.FacetParam != nil {
			var fcsList []*protos.Facets
			for _, fres := range filteredRes {
				fcsList = append(fcsList, &protos.Facets{fres.facets})
			}
			out.FacetMatrix = append(out.FacetMatrix, &protos.FacetsList{fcsList})
		}

		switch {
		case q.DoCount:
			out.Counts = append(out.Counts, uint32(pl.Length(0)))
			// Add an empty UID list to make later processing consistent
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
		case srcFn.fnType == CompareScalarFn:
			count := int64(pl.Length(0))
			if EvalCompare(srcFn.fname, count, srcFn.threshold) {
				tlist := &protos.List{[]uint64{q.UidList.Uids[i]}}
				out.UidMatrix = append(out.UidMatrix, tlist)
			}
		case srcFn.fnType == HasFn:
			count := int64(pl.Length(0))
			if EvalCompare("gt", count, 0) {
				tlist := &protos.List{[]uint64{q.UidList.Uids[i]}}
				out.UidMatrix = append(out.UidMatrix, tlist)
			}
		case srcFn.fnType == UidInFn:
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
		default:
			// The more usual case: Getting the UIDs.
			uidList := new(protos.List)
			for _, fres := range filteredRes {
				uidList.Uids = append(uidList.Uids, fres.uid)
			}
			out.UidMatrix = append(out.UidMatrix, uidList)
		}
	}
	return nil
}

// This function should only be used by upsert. Upsert mutation also does a query which will wait
// for the mutation to complete and hence would get stuck. Therefore, we only need to wait till
// index - 1.
func processUpsertTask(ctx context.Context, q *protos.Query, gid uint32) (*protos.Result, error) {
	// This code is copied from waitLinearizableRead only difference being that we wait till
	// index-1.
	n := groups().Node
	replyCh, err := n.ReadIndex(ctx)
	if err != nil {
		return nil, err
	}
	select {
	case index := <-replyCh:
		if index == raft.None {
			return nil, x.Errorf("cannot get linearized read (time expired or no configured leader)")
		}
		if err := n.Applied.WaitForMark(ctx, index-1); err != nil {
			return nil, err
		}
		return helpProcessTask(ctx, q, gid)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// processTask processes the query, accumulates and returns the result.
func processTask(ctx context.Context, q *protos.Query, gid uint32) (*protos.Result, error) {
	if err := waitLinearizableRead(ctx); err != nil {
		return &emptyResult, err
	}
	return helpProcessTask(ctx, q, gid)
}

func helpProcessTask(ctx context.Context, q *protos.Query, gid uint32) (*protos.Result, error) {
	out := new(protos.Result)
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

	typ, err := schema.State().TypeOf(attr)
	if err != nil {
		if q.UidList == nil {
			return out, nil
		}
		// Schema not defined, so lets add dummy values and return.
		// Adding dummy values is important because the code assumes that len(SrcUids) equals
		// len(uidMatrix)
		for i := 0; i < len(q.UidList.Uids); i++ {
			out.UidMatrix = append(out.UidMatrix, &emptyUIDList)
			out.ValueMatrix = append(out.ValueMatrix, &emptyValueList)
		}
		return out, nil
	}
	srcFn.atype = typ

	opts := posting.ListOptions{
		AfterUID: uint64(q.AfterUid),
	}
	// If we have srcFunc and Uids, it means its a filter. So we intersect.
	if srcFn.fnType != NotAFunction && q.UidList != nil && len(q.UidList.Uids) > 0 {
		opts.Intersect = q.UidList
	}

	args := funcArgs{q, gid, srcFn, out}
	needsValPostings, err := srcFn.needsValuePostings(typ)
	if err != nil {
		return nil, err
	}
	if needsValPostings {
		if err = handleValuePostings(ctx, args); err != nil {
			return nil, err
		}
	} else {
		if err = handleUidPostings(ctx, args, opts); err != nil {
			return nil, err
		}
	}

	if srcFn.fnType == HasFn && srcFn.isFuncAtRoot {
		if err := handleHasFunction(funcArgs{q, gid, srcFn, out}); err != nil {
			return nil, err
		}
	}

	if srcFn.fnType == CompareScalarFn && srcFn.isFuncAtRoot {
		if err := handleCompareScalarFunction(funcArgs{q, gid, srcFn, out}); err != nil {
			return nil, err
		}
	}

	if srcFn.fnType == RegexFn {
		// Go through the indexkeys for the predicate and match them with
		// the regex matcher.
		if err := handleRegexFunction(ctx, funcArgs{q, gid, srcFn, out}); err != nil {
			return nil, err
		}
	}

	// We fetch the actual value for the uids, compare them to the value in the
	// request and filter the uids only if the tokenizer IsLossy.
	if srcFn.fnType == CompareAttrFn && len(srcFn.tokens) > 0 {
		if err := handleCompareFunction(ctx, funcArgs{q, gid, srcFn, out}); err != nil {
			return nil, err
		}
	}

	// If geo filter, do value check for correctness.
	if srcFn.geoQuery != nil {
		filterGeoFunction(funcArgs{q, gid, srcFn, out})
	}

	// For string matching functions, check the language.
	if needsStringFiltering(srcFn, q.Langs) {
		filterStringFunction(funcArgs{q, gid, srcFn, out})
	}

	out.IntersectDest = srcFn.intersectDest
	return out, nil
}

func needsStringFiltering(srcFn *functionContext, langs []string) bool {
	return srcFn.isStringFn && langForFunc(langs) != "." &&
		(srcFn.fnType == StandardFn || srcFn.fnType == HasFn ||
			srcFn.fnType == FullTextSearchFn || srcFn.fnType == CompareAttrFn)
}

func handleHasFunction(arg funcArgs) error {
	attr := arg.q.Attr
	if ok := schema.State().HasCount(attr); !ok {
		return x.Errorf("Need @count directive in schema for attr: %s for fn: %s at root.",
			attr, arg.srcFn.fname)
	}
	cp := countParams{
		count:   0,
		fn:      "gt",
		attr:    attr,
		gid:     arg.gid,
		reverse: arg.q.Reverse,
	}
	cp.evaluate(arg.out)
	return nil
}

func handleCompareScalarFunction(arg funcArgs) error {
	attr := arg.q.Attr
	if ok := schema.State().HasCount(attr); !ok {
		return x.Errorf("Need @count directive in schema for attr: %s for fn: %s at root",
			attr, arg.srcFn.fname)
	}
	count := arg.srcFn.threshold
	cp := countParams{
		fn:      arg.srcFn.fname,
		count:   count,
		attr:    attr,
		gid:     arg.gid,
		reverse: arg.q.Reverse,
	}
	cp.evaluate(arg.out)
	return nil
}

func handleRegexFunction(ctx context.Context, arg funcArgs) error {
	attr := arg.q.Attr
	typ, err := schema.State().TypeOf(attr)
	if err != nil || !typ.IsScalar() {
		return x.Errorf("Attribute not scalar: %s %v", attr, typ)
	}
	if typ != types.StringID {
		return x.Errorf("Got non-string type. Regex match is allowed only on string type.")
	}
	tokenizers := schema.State().TokenizerNames(attr)
	var found bool
	for _, t := range tokenizers {
		if t == "trigram" { // TODO(tzdybal) - maybe just rename to 'regex' tokenizer?
			found = true
		}
	}
	if !found {
		return x.Errorf("Attribute %v does not have trigram index for regex matching.", attr)
	}

	query := cindex.RegexpQuery(arg.srcFn.regex.Syntax)
	empty := protos.List{}
	uids, err := uidsForRegex(attr, arg.gid, query, &empty)
	lang := langForFunc(arg.q.Langs)
	if uids != nil {
		arg.out.UidMatrix = append(arg.out.UidMatrix, uids)

		var values []types.Val
		for _, uid := range uids.Uids {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			key := x.DataKey(attr, uid)
			pl := posting.Get(key)

			var val types.Val
			if lang != "" {
				val, err = pl.ValueForTag(lang)
			} else {
				val, err = pl.Value()
			}

			if err != nil {
				continue
			}
			// conver data from binary to appropriate format
			strVal, err := types.Convert(val, types.StringID)
			if err == nil {
				values = append(values, strVal)
			}
		}

		filtered := matchRegex(uids, values, arg.srcFn.regex)
		for i := 0; i < len(arg.out.UidMatrix); i++ {
			algo.IntersectWith(arg.out.UidMatrix[i], filtered, arg.out.UidMatrix[i])
		}
	} else {
		return err
	}
	return nil
}

func handleCompareFunction(ctx context.Context, arg funcArgs) error {
	attr := arg.q.Attr
	tokenizer, err := pickTokenizer(attr, arg.srcFn.fname)
	// We should already have checked this in getInequalityTokens.
	x.Check(err)
	// Only if the tokenizer that we used IsLossy, then we need to fetch
	// and compare the actual values.
	if tokenizer.IsLossy() {
		// Need to evaluate inequality for entries in the first bucket.
		typ, err := schema.State().TypeOf(attr)
		if err != nil || !typ.IsScalar() {
			return x.Errorf("Attribute not scalar: %s %v", attr, typ)
		}

		x.AssertTrue(len(arg.out.UidMatrix) > 0)
		rowsToFilter := 0
		if arg.srcFn.fname == eq {
			// If fn is eq, we could have multiple arguments and hence multiple rows
			// to filter.
			rowsToFilter = len(arg.srcFn.tokens)
		} else if arg.srcFn.tokens[0] == arg.srcFn.ineqValueToken {
			// If operation is not eq and ineqValueToken equals first token,
			// then we need to filter first row..
			rowsToFilter = 1
		}
		lang := langForFunc(arg.q.Langs)
		for row := 0; row < rowsToFilter; row++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			algo.ApplyFilter(arg.out.UidMatrix[row], func(uid uint64, i int) bool {
				switch lang {
				case "":
					pl := posting.GetNoStore(x.DataKey(attr, uid))
					sv, err := pl.Value()
					if err == nil {
						dst, err := types.Convert(sv, typ)
						return err == nil &&
							types.CompareVals(arg.q.SrcFunc.Name, dst, arg.srcFn.eqTokens[row])
					}
					return false
				case ".":
					pl := posting.GetNoStore(x.DataKey(attr, uid))
					values, _ := pl.AllValues()
					for _, sv := range values {
						dst, err := types.Convert(sv, typ)
						if err == nil &&
							types.CompareVals(arg.q.SrcFunc.Name, dst, arg.srcFn.eqTokens[row]) {
							return true
						}
					}
					return false
				default:
					sv, err := fetchValue(uid, attr, arg.q.Langs, typ)
					if sv.Value == nil || err != nil {
						return false
					}
					return types.CompareVals(arg.q.SrcFunc.Name, sv, arg.srcFn.eqTokens[row])
				}
			})
		}
	}
	return nil
}

func filterGeoFunction(arg funcArgs) {
	attr := arg.q.Attr
	var values []*protos.TaskValue
	uids := algo.MergeSorted(arg.out.UidMatrix)
	for _, uid := range uids.Uids {
		key := x.DataKey(attr, uid)
		pl := posting.Get(key)

		val, err := pl.Value()
		newValue := &protos.TaskValue{ValType: int32(val.Tid)}
		if err == nil {
			newValue.Val = val.Value.([]byte)
		} else {
			newValue.Val = x.Nilbyte
		}
		values = append(values, newValue)
	}

	filtered := types.FilterGeoUids(uids, values, arg.srcFn.geoQuery)
	for i := 0; i < len(arg.out.UidMatrix); i++ {
		algo.IntersectWith(arg.out.UidMatrix[i], filtered, arg.out.UidMatrix[i])
	}
}

func filterStringFunction(arg funcArgs) {
	attr := arg.q.Attr
	uids := algo.MergeSorted(arg.out.UidMatrix)
	var values []types.Val
	filteredUids := make([]uint64, 0, len(uids.Uids))
	lang := langForFunc(arg.q.Langs)
	for _, uid := range uids.Uids {
		key := x.DataKey(attr, uid)
		pl := posting.Get(key)

		var val types.Val
		var err error
		if lang == "" {
			val, err = pl.Value()
		} else {
			val, err = pl.ValueForTag(lang)
		}
		if err != nil {
			continue
		}
		// convert data from binary to appropriate format
		strVal, err := types.Convert(val, types.StringID)
		if err == nil {
			values = append(values, strVal)
		}
		filteredUids = append(filteredUids, uid)
	}

	filtered := &protos.List{Uids: filteredUids}
	filter := stringFilter{
		funcName: arg.srcFn.fname,
		funcType: arg.srcFn.fnType,
		lang:     lang,
	}

	switch arg.srcFn.fnType {
	case HasFn:
		// Dont do anything, as filtering based on lang is already
		// done above.
	case FullTextSearchFn, StandardFn:
		filter.tokens = arg.srcFn.tokens
		filter.match = defaultMatch
		filtered = matchStrings(filtered, values, filter)
	case CompareAttrFn:
		filter.ineqValue = arg.srcFn.ineqValue
		filter.eqVals = arg.srcFn.eqTokens
		filter.match = ineqMatch
		filtered = matchStrings(uids, values, filter)
	}

	for i := 0; i < len(arg.out.UidMatrix); i++ {
		algo.IntersectWith(arg.out.UidMatrix[i], filtered, arg.out.UidMatrix[i])
	}
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
	fnType         FuncType
	regex          *cregexp.Regexp
	isFuncAtRoot   bool
	isStringFn     bool
	atype          types.TypeID
}

const (
	eq = "eq" // equal
)

func ensureArgsCount(srcFunc *protos.SrcFunction, expected int) error {
	if len(srcFunc.Args) != expected {
		return x.Errorf("Function '%s' requires %d arguments, but got %d (%v)",
			srcFunc.Name, expected, len(srcFunc.Args), srcFunc.Args)
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

// We allow atmost one lang in functions. We can inline in 1.9.
func langForFunc(langs []string) string {
	x.AssertTrue(len(langs) <= 1)
	if len(langs) == 0 {
		return ""
	}
	return langs[0]
}

func parseSrcFn(q *protos.Query) (*functionContext, error) {
	fnType, f := parseFuncType(q.SrcFunc)
	attr := q.Attr
	fc := &functionContext{fnType: fnType, fname: f}
	var err error

	t, err := schema.State().TypeOf(attr)
	if err == nil && fnType != NotAFunction && t.Name() == types.StringID.Name() {
		fc.isStringFn = true
	}

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
		args := q.SrcFunc.Args
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

		// Number of index keys is more than no. of uids to filter, so its better to fetch data keys
		// directly and compare. Lets make tokens empty.
		// We don't do this for eq because eq could have multiple arguments and we would have to
		// compare the value with all of them. Also eq would usually have less arguments, hence we
		// won't be fetching many index keys.
		if q.UidList != nil && len(fc.tokens) > len(q.UidList.Uids) && fc.fname != eq {
			fc.tokens = fc.tokens[:0]
			fc.n = len(q.UidList.Uids)
		} else {
			fc.n = len(fc.tokens)
		}
	case CompareScalarFn:
		if err = ensureArgsCount(q.SrcFunc, 1); err != nil {
			return nil, err
		}
		if fc.threshold, err = strconv.ParseInt(q.SrcFunc.Args[0], 0, 64); err != nil {
			return nil, x.Wrapf(err, "Compare %v(%v) require digits, but got invalid num",
				q.SrcFunc.Name, q.SrcFunc.Args[0])
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
		if fc.tokens, err = getStringTokens(q.SrcFunc.Args, langForFunc(q.Langs), fnType); err != nil {
			return nil, err
		}
		fnName := strings.ToLower(q.SrcFunc.Name)
		fc.intersectDest = strings.HasPrefix(fnName, "allof") // allofterms and alloftext
		fc.n = len(fc.tokens)
	case RegexFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		ignoreCase := false
		modifiers := q.SrcFunc.Args[1]
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
		if fc.regex, err = cregexp.Compile(matchType + q.SrcFunc.Args[0]); err != nil {
			return nil, err
		}
		fc.n = 0
	case HasFn:
		if err = ensureArgsCount(q.SrcFunc, 0); err != nil {
			return nil, err
		}
		checkRoot(q, fc)
	case UidInFn:
		if err = ensureArgsCount(q.SrcFunc, 1); err != nil {
			return nil, err
		}
		if fc.uidPresent, err = strconv.ParseUint(q.SrcFunc.Args[0], 0, 64); err != nil {
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

	gid := groups().BelongsTo(q.Attr)
	var numUids int
	if q.UidList != nil {
		numUids = len(q.UidList.Uids)
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr, numUids, gid)
	}

	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", q.Attr, gid)

	type reply struct {
		result *protos.Result
		err    error
	}
	c := make(chan reply, 1)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		// md is a map[string][]string
		if v, ok := md["trace"]; ok && len(v) > 0 {
			var tr trace.Trace
			tr, ctx = x.NewTrace("GrpcQuery", ctx)
			defer tr.Finish()
			tr.LazyPrintf("Trace id %s", v[0])
		}
	}
	go func() {
		result, err := processTask(ctx, q, gid)
		c <- reply{result, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case reply := <-c:
		return reply.result, reply.err
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
		fnType, fname := parseFuncTypeHelper(fname)
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

		fnType, fname := parseFuncTypeHelper(ftree.function.name)
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
		pl := posting.Get(countKey)
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
	itOpt.PrefetchValues = false
	itOpt.Reverse = cp.fn == "le" || cp.fn == "lt"
	it := pstore.NewIterator(itOpt)
	defer it.Close()
	pk := x.ParsedKey{
		Attr: cp.attr,
	}
	countPrefix := pk.CountPrefix(cp.reverse)

	for it.Seek(countKey); it.ValidForPrefix(countPrefix); it.Next() {
		key := it.Item().Key()
		pl := posting.Get(key)
		out.UidMatrix = append(out.UidMatrix, pl.Uids(posting.ListOptions{}))
	}
}
