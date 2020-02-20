/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"bytes"
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	ctask "github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"

	"github.com/golang/protobuf/proto"
	cindex "github.com/google/codesearch/index"
	cregexp "github.com/google/codesearch/regexp"
	"github.com/pkg/errors"
)

func invokeNetworkRequest(ctx context.Context, addr string,
	f func(context.Context, pb.WorkerClient) (interface{}, error)) (interface{}, error) {
	pl, err := conn.GetPools().Get(addr)
	if err != nil {
		return &pb.Result{}, errors.Wrapf(err, "dispatchTaskOverNetwork: while retrieving connection.")
	}

	con := pl.Get()
	if span := otrace.FromContext(ctx); span != nil {
		span.Annotatef(nil, "invokeNetworkRequest: Sending request to %v", addr)
	}
	c := pb.NewWorkerClient(con)
	return f(ctx, c)
}

const backupRequestGracePeriod = time.Second

// TODO: Cross-server cancellation as described in Jeff Dean's talk.
func processWithBackupRequest(
	ctx context.Context,
	gid uint32,
	f func(context.Context, pb.WorkerClient) (interface{}, error)) (interface{}, error) {
	addrs := groups().AnyTwoServers(gid)
	if len(addrs) == 0 {
		return nil, errors.New("No network connection")
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
func ProcessTaskOverNetwork(ctx context.Context, q *pb.Query) (*pb.Result, error) {
	attr := q.Attr
	gid, err := groups().BelongsToReadOnly(attr, q.ReadTs)
	switch {
	case err != nil:
		return &pb.Result{}, err
	case gid == 0:
		return &pb.Result{}, errNonExistentTablet
	}

	span := otrace.FromContext(ctx)
	if span != nil {
		span.Annotatef(nil, "ProcessTaskOverNetwork. attr: %v gid: %v, readTs: %d, node id: %d",
			attr, gid, q.ReadTs, groups().Node.Id)
	}

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processTask(ctx, q, gid)
	}

	result, err := processWithBackupRequest(ctx, gid,
		func(ctx context.Context, c pb.WorkerClient) (interface{}, error) {
			return c.ServeTask(ctx, q)
		})
	if err != nil {
		return &pb.Result{}, err
	}

	reply := result.(*pb.Result)
	if span != nil {
		span.Annotatef(nil, "Reply from server. len: %v gid: %v Attr: %v",
			len(reply.UidMatrix), gid, attr)
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
		return types.Val{}, errors.Errorf("Attribute %s is not valid scalar type", attr)
	}
	src := types.Val{Tid: types.StringID, Value: []byte(data)}
	dst, err := types.Convert(src, t)
	return dst, err
}

// Returns nil byte on error
func convertToType(v types.Val, typ types.TypeID) (*pb.TaskValue, error) {
	result := &pb.TaskValue{ValType: typ.Enum(), Val: x.Nilbyte}
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
		return result, errors.Errorf("Failed convertToType during Marshal")
	}
	result.Val = data.Value.([]byte)
	return result, nil
}

// FuncType represents the type of a query function (aggregation, has, etc).
type FuncType int

const (
	notAFunction FuncType = iota
	aggregatorFn
	compareAttrFn
	compareScalarFn
	geoFn
	passwordFn
	regexFn
	fullTextSearchFn
	hasFn
	uidInFn
	customIndexFn
	matchFn
	standardFn = 100
)

func parseFuncType(srcFunc *pb.SrcFunction) (FuncType, string) {
	if srcFunc == nil {
		return notAFunction, ""
	}
	ftype, fname := parseFuncTypeHelper(srcFunc.Name)
	if srcFunc.IsCount && ftype == compareAttrFn {
		// gt(release_date, "1990") is 'CompareAttr' which
		//    takes advantage of indexed-attr
		// gt(count(films), 0) is 'CompareScalar', we first do
		//    counting on attr, then compare the result as scalar with int
		return compareScalarFn, fname
	}
	return ftype, fname
}

func parseFuncTypeHelper(name string) (FuncType, string) {
	if len(name) == 0 {
		return notAFunction, ""
	}
	f := strings.ToLower(name)
	switch f {
	case "le", "ge", "lt", "gt", "eq":
		return compareAttrFn, f
	case "min", "max", "sum", "avg":
		return aggregatorFn, f
	case "checkpwd":
		return passwordFn, f
	case "regexp":
		return regexFn, f
	case "alloftext", "anyoftext":
		return fullTextSearchFn, f
	case "has":
		return hasFn, f
	case "uid_in":
		return uidInFn, f
	case "anyof", "allof":
		return customIndexFn, f
	case "match":
		return matchFn, f
	default:
		if types.IsGeoFunc(f) {
			return geoFn, f
		}
		return standardFn, f
	}
}

func needsIndex(fnType FuncType, uidList *pb.List) bool {
	switch fnType {
	case compareAttrFn:
		if uidList != nil {
			// UidList is not nil means this is a filter. Filter predicate is not indexed, so
			// instead of fetching values by index key, we will fetch value by data key
			// (from uid and predicate) and apply filter on values.
			return false
		}
		return true
	case geoFn, fullTextSearchFn, standardFn, matchFn:
		return true
	}
	return false
}

// needsIntersect checks if the function type needs algo.IntersectSorted() after the results
// are collected. This is needed for functions that require all values to  match, like
// "allofterms", "alloftext", and custom functions with "allof".
// Returns true if function results need intersect, false otherwise.
func needsIntersect(fnName string) bool {
	return strings.HasPrefix(fnName, "allof") || strings.HasSuffix(fnName, "allof")
}

type funcArgs struct {
	q     *pb.Query
	gid   uint32
	srcFn *functionContext
	out   *pb.Result
}

// The function tells us whether we want to fetch value posting lists or uid posting lists.
func (srcFn *functionContext) needsValuePostings(typ types.TypeID) (bool, error) {
	switch srcFn.fnType {
	case aggregatorFn, passwordFn:
		return true, nil
	case compareAttrFn:
		if len(srcFn.tokens) > 0 {
			return false, nil
		}
		return true, nil
	case geoFn, regexFn, fullTextSearchFn, standardFn, hasFn, customIndexFn, matchFn:
		// All of these require an index, hence would require fetching uid postings.
		return false, nil
	case uidInFn, compareScalarFn:
		// Operate on uid postings
		return false, nil
	case notAFunction:
		return typ.IsScalar(), nil
	}
	return false, errors.Errorf("Unhandled case in fetchValuePostings for fn: %s", srcFn.fname)
}

// Handles fetching of value posting lists and filtering of uids based on that.
func (qs *queryState) handleValuePostings(ctx context.Context, args funcArgs) error {
	srcFn := args.srcFn
	q := args.q

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "handleValuePostings")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "Number of uids: %d. args.srcFn: %+v", srcFn.n, args.srcFn)
	}

	switch srcFn.fnType {
	case notAFunction, aggregatorFn, passwordFn, compareAttrFn:
	default:
		return errors.Errorf("Unhandled function in handleValuePostings: %s", srcFn.fname)
	}

	if srcFn.atype == types.PasswordID && srcFn.fnType != passwordFn {
		// Silently skip if the user is trying to fetch an attribute of type password.
		return nil
	}
	if srcFn.fnType == passwordFn && srcFn.atype != types.PasswordID {
		return errors.Errorf("checkpwd fn can only be used on attr: [%s] with schema type "+
			"password. Got type: %s", q.Attr, types.TypeID(srcFn.atype).Name())
	}
	if srcFn.n == 0 {
		return nil
	}

	// This function has small boilerplate as handleUidPostings, around how the code gets
	// concurrently executed. I didn't see much value in trying to separate it out, because the core
	// logic constitutes most of the code volume here.
	numGo, width := x.DivideAndRule(srcFn.n)
	x.AssertTrue(width > 0)
	span.Annotatef(nil, "Width: %d. NumGo: %d", width, numGo)

	errCh := make(chan error, numGo)
	outputs := make([]*pb.Result, numGo)
	listType := schema.State().IsList(q.Attr)

	calculate := func(start, end int) error {
		x.AssertTrue(start%width == 0)
		out := &pb.Result{}
		outputs[start/width] = out

		for i := start; i < end; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			key := x.DataKey(q.Attr, q.UidList.Uids[i])

			// Get or create the posting list for an entity, attribute combination.
			pl, err := qs.cache.Get(key)
			if err != nil {
				return err
			}

			vals, fcs, err := retrieveValuesAndFacets(args, pl, listType)
			switch {
			case err == posting.ErrNoValue || len(vals) == 0:
				out.UidMatrix = append(out.UidMatrix, &pb.List{})
				out.FacetMatrix = append(out.FacetMatrix, &pb.FacetsList{})
				if q.DoCount {
					out.Counts = append(out.Counts, 0)
				} else {
					out.ValueMatrix = append(out.ValueMatrix,
						&pb.ValueList{Values: []*pb.TaskValue{}})
					if q.ExpandAll {
						// To keep the cardinality same as that of ValueMatrix.
						out.LangMatrix = append(out.LangMatrix, &pb.LangList{})
					}
				}
				continue
			case err != nil:
				return err
			}

			if q.ExpandAll {
				langTags, err := pl.GetLangTags(args.q.ReadTs)
				if err != nil {
					return err
				}
				out.LangMatrix = append(out.LangMatrix, &pb.LangList{Lang: langTags})
			}

			uidList := new(pb.List)
			var vl pb.ValueList
			for _, val := range vals {
				newValue, err := convertToType(val, srcFn.atype)
				if err != nil {
					return err
				}

				// This means we fetched the value directly instead of fetching index key and intersecting.
				// Lets compare the value and add filter the uid.
				if srcFn.fnType == compareAttrFn {
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

			// Add facets to result.
			out.FacetMatrix = append(out.FacetMatrix, fcs)

			switch {
			case q.DoCount:
				len := pl.Length(args.q.ReadTs, 0)
				if len == -1 {
					return posting.ErrTsTooOld
				}
				out.Counts = append(out.Counts, uint32(len))
				// Add an empty UID list to make later processing consistent
				out.UidMatrix = append(out.UidMatrix, &pb.List{})
			case srcFn.fnType == aggregatorFn:
				// Add an empty UID list to make later processing consistent
				out.UidMatrix = append(out.UidMatrix, &pb.List{})
			case srcFn.fnType == passwordFn:
				lastPos := len(out.ValueMatrix) - 1
				if len(out.ValueMatrix[lastPos].Values) == 0 {
					continue
				}
				newValue := out.ValueMatrix[lastPos].Values[0]
				if len(newValue.Val) == 0 {
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
				out.UidMatrix = append(out.UidMatrix, &pb.List{})
			default:
				out.UidMatrix = append(out.UidMatrix, uidList)
			}
		}
		return nil
	} // End of calculate function.

	for i := 0; i < numGo; i++ {
		start := i * width
		end := start + width
		if end > srcFn.n {
			end = srcFn.n
		}
		go func(start, end int) {
			errCh <- calculate(start, end)
		}(start, end)
	}
	for i := 0; i < numGo; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	// All goroutines are done. Now attach their results.
	out := args.out
	for _, chunk := range outputs {
		out.UidMatrix = append(out.UidMatrix, chunk.UidMatrix...)
		out.Counts = append(out.Counts, chunk.Counts...)
		out.ValueMatrix = append(out.ValueMatrix, chunk.ValueMatrix...)
		out.FacetMatrix = append(out.FacetMatrix, chunk.FacetMatrix...)
		out.LangMatrix = append(out.LangMatrix, chunk.LangMatrix...)
	}
	return nil
}

func retrieveValuesAndFacets(args funcArgs, pl *posting.List, listType bool) (
	[]types.Val, *pb.FacetsList, error) {
	q := args.q
	var err error
	var vals []types.Val
	var fcs []*pb.Facets

	// Retrieve values when facet filtering is not being requested.
	if q.FacetsFilter == nil {
		// Retrieve values.
		switch {
		case q.ExpandAll:
			vals, err = pl.AllValues(args.q.ReadTs)
		case listType && len(q.Langs) == 0:
			vals, err = pl.AllUntaggedValues(args.q.ReadTs)
		default:
			var val types.Val
			val, err = pl.ValueFor(args.q.ReadTs, q.Langs)
			vals = append(vals, val)
		}
		if err != nil {
			return nil, nil, err
		}

		// Retrieve facets.
		if q.FacetParam != nil {
			fcs, err = pl.Facets(args.q.ReadTs, q.FacetParam, q.Langs, listType)
		}
		if err != nil {
			return nil, nil, err
		}

		return vals, &pb.FacetsList{FacetsList: fcs}, nil
	}

	// Retrieve values when facet filtering is being requested.
	facetsTree, err := preprocessFilter(q.FacetsFilter)
	if err != nil {
		return nil, nil, err
	}

	// Retrieve the posting that matches the language preferences.
	langMatch, err := pl.PostingFor(q.ReadTs, q.Langs)
	if err != nil && err != posting.ErrNoValue {
		return nil, nil, err
	}
	err = pl.Iterate(q.ReadTs, 0, func(p *pb.Posting) error {
		if listType && len(q.Langs) == 0 {
			// Don't retrieve tagged values unless explicitly asked.
			if len(p.LangTag) > 0 {
				return nil
			}
		} else {
			// Only consider the posting that matches our language preferences.
			if !proto.Equal(p, langMatch) {
				return nil
			}
		}

		picked, err := applyFacetsTree(p.Facets, facetsTree)
		if err != nil {
			return err
		}
		if picked {
			vals = append(vals, types.Val{
				Tid:   types.TypeID(p.ValType),
				Value: p.Value,
			})
			if q.FacetParam != nil {
				fcs = append(fcs, &pb.Facets{Facets: facets.CopyFacets(p.Facets, q.FacetParam)})
			}
		}
		return nil // continue iteration.
	})
	if err != nil {
		return nil, nil, err
	}

	return vals, &pb.FacetsList{FacetsList: fcs}, nil
}

// This function handles operations on uid posting lists. Index keys, reverse keys and some data
// keys store uid posting lists.
func (qs *queryState) handleUidPostings(
	ctx context.Context, args funcArgs, opts posting.ListOptions) error {
	srcFn := args.srcFn
	q := args.q

	facetsTree, err := preprocessFilter(q.FacetsFilter)
	if err != nil {
		return err
	}

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "handleUidPostings")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "Number of uids: %d. args.srcFn: %+v", srcFn.n, args.srcFn)
	}
	if srcFn.n == 0 {
		return nil
	}

	// Divide the task into many goroutines.
	numGo, width := x.DivideAndRule(srcFn.n)
	x.AssertTrue(width > 0)
	span.Annotatef(nil, "Width: %d. NumGo: %d", width, numGo)

	errCh := make(chan error, numGo)
	outputs := make([]*pb.Result, numGo)

	calculate := func(start, end int) error {
		x.AssertTrue(start%width == 0)
		out := &pb.Result{}
		outputs[start/width] = out

		for i := start; i < end; i++ {
			if i%100 == 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
			}
			var key []byte
			switch srcFn.fnType {
			case notAFunction, compareScalarFn, hasFn, uidInFn:
				if q.Reverse {
					key = x.ReverseKey(q.Attr, q.UidList.Uids[i])
				} else {
					key = x.DataKey(q.Attr, q.UidList.Uids[i])
				}
			case geoFn, regexFn, fullTextSearchFn, standardFn, customIndexFn, matchFn,
				compareAttrFn:
				key = x.IndexKey(q.Attr, srcFn.tokens[i])
			default:
				return errors.Errorf("Unhandled function in handleUidPostings: %s", srcFn.fname)
			}

			// Get or create the posting list for an entity, attribute combination.
			pl, err := qs.cache.Get(key)
			if err != nil {
				return err
			}

			switch {
			case q.DoCount:
				if i == 0 {
					span.Annotate(nil, "DoCount")
				}
				len := pl.Length(args.q.ReadTs, 0)
				if len == -1 {
					return posting.ErrTsTooOld
				}
				out.Counts = append(out.Counts, uint32(len))
				// Add an empty UID list to make later processing consistent
				out.UidMatrix = append(out.UidMatrix, &pb.List{})
			case srcFn.fnType == compareScalarFn:
				if i == 0 {
					span.Annotate(nil, "CompareScalarFn")
				}
				len := pl.Length(args.q.ReadTs, 0)
				if len == -1 {
					return posting.ErrTsTooOld
				}
				count := int64(len)
				if evalCompare(srcFn.fname, count, srcFn.threshold) {
					tlist := &pb.List{Uids: []uint64{q.UidList.Uids[i]}}
					out.UidMatrix = append(out.UidMatrix, tlist)
				}
			case srcFn.fnType == hasFn:
				if i == 0 {
					span.Annotate(nil, "HasFn")
				}
				empty, err := pl.IsEmpty(args.q.ReadTs, 0)
				if err != nil {
					return err
				}
				if !empty {
					tlist := &pb.List{Uids: []uint64{q.UidList.Uids[i]}}
					out.UidMatrix = append(out.UidMatrix, tlist)
				}
			case srcFn.fnType == uidInFn:
				if i == 0 {
					span.Annotate(nil, "UidInFn")
				}
				reqList := &pb.List{Uids: []uint64{srcFn.uidPresent}}
				topts := posting.ListOptions{
					ReadTs:    args.q.ReadTs,
					AfterUid:  0,
					Intersect: reqList,
				}
				plist, err := pl.Uids(topts)
				if err != nil {
					return err
				}
				if len(plist.Uids) > 0 {
					tlist := &pb.List{Uids: []uint64{q.UidList.Uids[i]}}
					out.UidMatrix = append(out.UidMatrix, tlist)
				}
			case q.FacetParam != nil || facetsTree != nil:
				if i == 0 {
					span.Annotate(nil, "default with facets")
				}
				uidList := &pb.List{
					Uids: make([]uint64, 0, pl.ApproxLen()),
				}

				var fcsList []*pb.Facets
				err = pl.Postings(opts, func(p *pb.Posting) error {
					pick, err := applyFacetsTree(p.Facets, facetsTree)
					if err != nil {
						return err
					}
					if pick {
						// TODO: This way of picking Uids differs from how
						// pl.Uids works. So, have a look to see if we're
						// catching all the edge cases here.
						uidList.Uids = append(uidList.Uids, p.Uid)
						if q.FacetParam != nil {
							fcsList = append(fcsList, &pb.Facets{
								Facets: facets.CopyFacets(p.Facets, q.FacetParam),
							})
						}
					}
					return nil // continue iteration.
				})
				if err != nil {
					return err
				}

				out.UidMatrix = append(out.UidMatrix, uidList)
				if q.FacetParam != nil {
					out.FacetMatrix = append(out.FacetMatrix, &pb.FacetsList{FacetsList: fcsList})
				}
			default:
				if i == 0 {
					span.Annotate(nil, "default no facets")
				}
				uidList, err := pl.Uids(opts)
				if err != nil {
					return err
				}
				out.UidMatrix = append(out.UidMatrix, uidList)
			}
		}
		return nil
	} // End of calculate function.

	for i := 0; i < numGo; i++ {
		start := i * width
		end := start + width
		if end > srcFn.n {
			end = srcFn.n
		}
		go func(start, end int) {
			errCh <- calculate(start, end)
		}(start, end)
	}
	for i := 0; i < numGo; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	// All goroutines are done. Now attach their results.
	out := args.out
	for _, chunk := range outputs {
		out.FacetMatrix = append(out.FacetMatrix, chunk.FacetMatrix...)
		out.Counts = append(out.Counts, chunk.Counts...)
		out.UidMatrix = append(out.UidMatrix, chunk.UidMatrix...)
	}
	var total int
	for _, list := range out.UidMatrix {
		total += len(list.Uids)
	}
	span.Annotatef(nil, "Total number of elements in matrix: %d", total)
	return nil
}

const (
	// UseTxnCache indicates the transaction cache should be used.
	UseTxnCache = iota
	// NoCache indicates no caches should be used.
	NoCache
)

// processTask processes the query, accumulates and returns the result.
func processTask(ctx context.Context, q *pb.Query, gid uint32) (*pb.Result, error) {
	ctx, span := otrace.StartSpan(ctx, "processTask."+q.Attr)
	defer span.End()

	stop := x.SpanTimer(span, "processTask"+q.Attr)
	defer stop()

	span.Annotatef(nil, "Waiting for startTs: %d at node: %d, gid: %d",
		q.ReadTs, groups().Node.Id, gid)
	if err := posting.Oracle().WaitForTs(ctx, q.ReadTs); err != nil {
		return &pb.Result{}, err
	}
	if span != nil {
		maxAssigned := posting.Oracle().MaxAssigned()
		span.Annotatef(nil, "Done waiting for maxAssigned. Attr: %q ReadTs: %d Max: %d",
			q.Attr, q.ReadTs, maxAssigned)
	}
	if err := groups().ChecksumsMatch(ctx); err != nil {
		return &pb.Result{}, err
	}
	span.Annotatef(nil, "Done waiting for checksum match")

	// If a group stops serving tablet and it gets partitioned away from group
	// zero, then it wouldn't know that this group is no longer serving this
	// predicate. There's no issue if a we are serving a particular tablet and
	// we get partitioned away from group zero as long as it's not removed.
	// BelongsToReadOnly is called instead of BelongsTo to prevent this alpha
	// from requesting to serve this tablet.
	knownGid, err := groups().BelongsToReadOnly(q.Attr, q.ReadTs)
	switch {
	case err != nil:
		return &pb.Result{}, err
	case knownGid == 0:
		return &pb.Result{}, errNonExistentTablet
	case knownGid != groups().groupId():
		return &pb.Result{}, errUnservedTablet
	}

	var qs queryState
	if q.Cache == UseTxnCache {
		qs.cache = posting.Oracle().CacheAt(q.ReadTs)
	}
	// For now, remove the query level cache. It is causing contention for queries with high
	// fan-out.

	out, err := qs.helpProcessTask(ctx, q, gid)
	if err != nil {
		return &pb.Result{}, err
	}
	return out, nil
}

type queryState struct {
	cache *posting.LocalCache
}

func (qs *queryState) helpProcessTask(ctx context.Context, q *pb.Query, gid uint32) (
	*pb.Result, error) {

	span := otrace.FromContext(ctx)
	out := new(pb.Result)
	attr := q.Attr

	srcFn, err := parseSrcFn(q)
	if err != nil {
		return nil, err
	}

	if q.Reverse && !schema.State().IsReversed(schema.ReadCtx, attr) {
		return nil, errors.Errorf("Predicate %s doesn't have reverse edge", attr)
	}

	if needsIndex(srcFn.fnType, q.UidList) && !schema.State().IsIndexed(schema.ReadCtx, q.Attr) {
		return nil, errors.Errorf("Predicate %s is not indexed", q.Attr)
	}

	if len(q.Langs) > 0 && !schema.State().HasLang(attr) {
		return nil, errors.Errorf("Language tags can only be used with predicates of string type"+
			" having @lang directive in schema. Got: [%v]", attr)
	}

	typ, err := schema.State().TypeOf(attr)
	if err != nil {
		// All schema checks are done before this, this type is only used to
		// convert it to schema type before returning.
		// Schema type won't be present only if there is no data for that predicate
		// or if we load through bulk loader.
		typ = types.DefaultID
	}
	out.List = schema.State().IsList(attr)
	srcFn.atype = typ

	// Reverse attributes might have more than 1 results even if the original attribute
	// is not a list.
	if q.Reverse {
		out.List = true
	}

	opts := posting.ListOptions{
		ReadTs:   q.ReadTs,
		AfterUid: q.AfterUid,
	}
	// If we have srcFunc and Uids, it means its a filter. So we intersect.
	if srcFn.fnType != notAFunction && q.UidList != nil && len(q.UidList.Uids) > 0 {
		opts.Intersect = q.UidList
	}

	args := funcArgs{q, gid, srcFn, out}
	needsValPostings, err := srcFn.needsValuePostings(typ)
	if err != nil {
		return nil, err
	}
	if needsValPostings {
		span.Annotate(nil, "handleValuePostings")
		if err = qs.handleValuePostings(ctx, args); err != nil {
			return nil, err
		}
	} else {
		span.Annotate(nil, "handleUidPostings")
		if err = qs.handleUidPostings(ctx, args, opts); err != nil {
			return nil, err
		}
	}

	if srcFn.fnType == hasFn && srcFn.isFuncAtRoot {
		span.Annotate(nil, "handleHasFunction")
		if err := qs.handleHasFunction(ctx, q, out, srcFn); err != nil {
			return nil, err
		}
	}

	if srcFn.fnType == compareScalarFn && srcFn.isFuncAtRoot {
		span.Annotate(nil, "handleCompareScalarFunction")
		if err := qs.handleCompareScalarFunction(args); err != nil {
			return nil, err
		}
	}

	if srcFn.fnType == regexFn {
		span.Annotate(nil, "handleRegexFunction")
		if err := qs.handleRegexFunction(ctx, args); err != nil {
			return nil, err
		}
	}

	if srcFn.fnType == matchFn {
		span.Annotate(nil, "handleMatchFunction")
		if err := qs.handleMatchFunction(ctx, args); err != nil {
			return nil, err
		}
	}

	// We fetch the actual value for the uids, compare them to the value in the
	// request and filter the uids only if the tokenizer IsLossy.
	if srcFn.fnType == compareAttrFn && len(srcFn.tokens) > 0 {
		span.Annotate(nil, "handleCompareFunction")
		if err := qs.handleCompareFunction(ctx, args); err != nil {
			return nil, err
		}
	}

	// If geo filter, do value check for correctness.
	if srcFn.geoQuery != nil {
		span.Annotate(nil, "handleGeoFunction")
		if err := qs.filterGeoFunction(ctx, args); err != nil {
			return nil, err
		}
	}

	// For string matching functions, check the language. We are not checking here
	// for hasFn as filtering for it has already been done in handleHasFunction.
	if srcFn.fnType != hasFn && needsStringFiltering(srcFn, q.Langs, attr) {
		span.Annotate(nil, "filterStringFunction")
		if err := qs.filterStringFunction(args); err != nil {
			return nil, err
		}
	}

	out.IntersectDest = srcFn.intersectDest
	return out, nil
}

func needsStringFiltering(srcFn *functionContext, langs []string, attr string) bool {
	if !srcFn.isStringFn {
		return false
	}

	// If a predicate doesn't have @lang directive in schema, we don't need to do any string
	// filtering.
	if !schema.State().HasLang(attr) {
		return false
	}

	return langForFunc(langs) != "." &&
		(srcFn.fnType == standardFn || srcFn.fnType == hasFn ||
			srcFn.fnType == fullTextSearchFn || srcFn.fnType == compareAttrFn ||
			srcFn.fnType == customIndexFn)
}

func (qs *queryState) handleCompareScalarFunction(arg funcArgs) error {
	attr := arg.q.Attr
	if ok := schema.State().HasCount(schema.ReadCtx, attr); !ok {
		return errors.Errorf("Need @count directive in schema for attr: %s for fn: %s at root",
			attr, arg.srcFn.fname)
	}
	count := arg.srcFn.threshold
	cp := countParams{
		fn:      arg.srcFn.fname,
		count:   count,
		attr:    attr,
		gid:     arg.gid,
		readTs:  arg.q.ReadTs,
		reverse: arg.q.Reverse,
	}
	return qs.evaluate(cp, arg.out)
}

func (qs *queryState) handleRegexFunction(ctx context.Context, arg funcArgs) error {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "handleRegexFunction")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "Number of uids: %d. args.srcFn: %+v", arg.srcFn.n, arg.srcFn)
	}

	attr := arg.q.Attr
	typ, err := schema.State().TypeOf(attr)
	span.Annotatef(nil, "Attr: %s. Type: %s", attr, typ.Name())
	if err != nil || !typ.IsScalar() {
		return errors.Errorf("Attribute not scalar: %s %v", attr, typ)
	}
	if typ != types.StringID {
		return errors.Errorf("Got non-string type. Regex match is allowed only on string type.")
	}
	useIndex := schema.State().HasTokenizer(schema.ReadCtx, tok.IdentTrigram, attr)
	span.Annotatef(nil, "Trigram index found: %t, func at root: %t",
		useIndex, arg.srcFn.isFuncAtRoot)

	query := cindex.RegexpQuery(arg.srcFn.regex.Syntax)
	empty := pb.List{}
	var uids *pb.List

	// Here we determine the list of uids to match.
	switch {
	// If this is a filter eval, use the given uid list (good)
	case arg.q.UidList != nil:
		// These UIDs are copied into arg.out.UidMatrix which is later updated while
		// processing the query. The below trick makes a copy of the list to avoid the
		// race conditions later. I (Aman) did a race condition tests to ensure that we
		// do not have more race condition in similar code in the rest of the file.
		// The race condition was found only here because in filter condition, even when
		// predicates do not have indexes, we allow regexp queries (for example, we do
		// not support eq/gt/lt/le in @filter, see #4077), and this was new code that
		// was added just to support the aforementioned case, the race condition is only
		// in this part of the code.
		uids = &pb.List{}
		uids.Uids = append(arg.q.UidList.Uids[:0:0], arg.q.UidList.Uids...)

	// Prefer to use an index (fast)
	case useIndex:
		uids, err = uidsForRegex(attr, arg, query, &empty)
		if err != nil {
			return err
		}

	// No index and at root, return error instructing user to use `has` or index.
	default:
		return errors.Errorf(
			"Attribute %v does not have trigram index for regex matching. "+
				"Please add a trigram index or use has/uid function with regexp() as filter.",
			attr)
	}

	arg.out.UidMatrix = append(arg.out.UidMatrix, uids)
	isList := schema.State().IsList(attr)
	lang := langForFunc(arg.q.Langs)

	span.Annotatef(nil, "Total uids: %d, list: %t lang: %v", len(uids.Uids), isList, lang)

	filtered := &pb.List{}
	for _, uid := range uids.Uids {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pl, err := qs.cache.Get(x.DataKey(attr, uid))
		if err != nil {
			return err
		}

		vals := make([]types.Val, 1)
		switch {
		case lang != "":
			vals[0], err = pl.ValueForTag(arg.q.ReadTs, lang)

		case isList:
			vals, err = pl.AllUntaggedValues(arg.q.ReadTs)

		default:
			vals[0], err = pl.Value(arg.q.ReadTs)
		}
		if err != nil {
			if err == posting.ErrNoValue {
				continue
			}
			return err
		}

		for _, val := range vals {
			// convert data from binary to appropriate format
			strVal, err := types.Convert(val, types.StringID)
			if err == nil && matchRegex(strVal, arg.srcFn.regex) {
				filtered.Uids = append(filtered.Uids, uid)
				// NOTE: We only add the uid once.
				break
			}
		}
	}

	for i := 0; i < len(arg.out.UidMatrix); i++ {
		algo.IntersectWith(arg.out.UidMatrix[i], filtered, arg.out.UidMatrix[i])
	}

	return nil
}

func (qs *queryState) handleCompareFunction(ctx context.Context, arg funcArgs) error {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "handleCompareFunction")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "Number of uids: %d. args.srcFn: %+v", arg.srcFn.n, arg.srcFn)
	}

	attr := arg.q.Attr
	span.Annotatef(nil, "Attr: %s. Fname: %s", attr, arg.srcFn.fname)
	tokenizer, err := pickTokenizer(attr, arg.srcFn.fname)
	if err != nil {
		return err
	}

	// Only if the tokenizer that we used IsLossy
	// then we need to fetch and compare the actual values.
	span.Annotatef(nil, "Tokenizer: %s, Lossy: %t", tokenizer.Name(), tokenizer.IsLossy())
	if tokenizer.IsLossy() {
		// Need to evaluate inequality for entries in the first bucket.
		typ, err := schema.State().TypeOf(attr)
		if err != nil || !typ.IsScalar() {
			return errors.Errorf("Attribute not scalar: %s %v", attr, typ)
		}

		x.AssertTrue(len(arg.out.UidMatrix) > 0)
		rowsToFilter := 0
		switch {
		case arg.srcFn.fname == eq:
			// If fn is eq, we could have multiple arguments and hence multiple rows to filter.
			rowsToFilter = len(arg.srcFn.tokens)
		case arg.srcFn.tokens[0] == arg.srcFn.ineqValueToken:
			// If operation is not eq and ineqValueToken equals first token,
			// then we need to filter first row.
			rowsToFilter = 1
		}
		isList := schema.State().IsList(attr)
		lang := langForFunc(arg.q.Langs)
		for row := 0; row < rowsToFilter; row++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			var filterErr error
			algo.ApplyFilter(arg.out.UidMatrix[row], func(uid uint64, i int) bool {
				switch lang {
				case "":
					if isList {
						pl, err := posting.GetNoStore(x.DataKey(attr, uid))
						if err != nil {
							filterErr = err
							return false
						}
						svs, err := pl.AllUntaggedValues(arg.q.ReadTs)
						if err != nil {
							if err != posting.ErrNoValue {
								filterErr = err
							}
							return false
						}
						for _, sv := range svs {
							dst, err := types.Convert(sv, typ)
							if err == nil && types.CompareVals(arg.q.SrcFunc.Name, dst, arg.srcFn.eqTokens[row]) {
								return true
							}
						}

						return false
					}

					pl, err := posting.GetNoStore(x.DataKey(attr, uid))
					if err != nil {
						filterErr = err
						return false
					}
					sv, err := pl.Value(arg.q.ReadTs)
					if err != nil {
						if err != posting.ErrNoValue {
							filterErr = err
						}
						return false
					}
					dst, err := types.Convert(sv, typ)
					return err == nil &&
						types.CompareVals(arg.q.SrcFunc.Name, dst, arg.srcFn.eqTokens[row])
				case ".":
					pl, err := posting.GetNoStore(x.DataKey(attr, uid))
					if err != nil {
						filterErr = err
						return false
					}
					values, err := pl.AllValues(arg.q.ReadTs) // does not return ErrNoValue
					if err != nil {
						filterErr = err
						return false
					}
					for _, sv := range values {
						dst, err := types.Convert(sv, typ)
						if err == nil &&
							types.CompareVals(arg.q.SrcFunc.Name, dst, arg.srcFn.eqTokens[row]) {
							return true
						}
					}
					return false
				default:
					sv, err := fetchValue(uid, attr, arg.q.Langs, typ, arg.q.ReadTs)
					if err != nil {
						if err != posting.ErrNoValue {
							filterErr = err
						}
						return false
					}
					if sv.Value == nil {
						return false
					}
					return types.CompareVals(arg.q.SrcFunc.Name, sv, arg.srcFn.eqTokens[row])
				}
			})
			if filterErr != nil {
				return err
			}
		}
	}
	return nil
}

func (qs *queryState) handleMatchFunction(ctx context.Context, arg funcArgs) error {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "handleMatchFunction")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "Number of uids: %d. args.srcFn: %+v", arg.srcFn.n, arg.srcFn)
	}

	attr := arg.q.Attr
	typ := arg.srcFn.atype
	span.Annotatef(nil, "Attr: %s. Type: %s", attr, typ.Name())
	var uids *pb.List
	switch {
	case !typ.IsScalar():
		return errors.Errorf("Attribute not scalar: %s %v", attr, typ)

	case typ != types.StringID:
		return errors.Errorf("Got non-string type. Fuzzy match is allowed only on string type.")

	case arg.q.UidList != nil && len(arg.q.UidList.Uids) != 0:
		uids = arg.q.UidList

	case schema.State().HasTokenizer(schema.ReadCtx, tok.IdentTrigram, attr):
		var err error
		uids, err = uidsForMatch(attr, arg)
		if err != nil {
			return err
		}

	default:
		return errors.Errorf(
			"Attribute %v does not have trigram index for fuzzy matching. "+
				"Please add a trigram index or use has/uid function with match() as filter.",
			attr)
	}

	isList := schema.State().IsList(attr)
	lang := langForFunc(arg.q.Langs)
	span.Annotatef(nil, "Total uids: %d, list: %t lang: %v", len(uids.Uids), isList, lang)
	arg.out.UidMatrix = append(arg.out.UidMatrix, uids)

	matchQuery := strings.Join(arg.srcFn.tokens, "")
	filtered := &pb.List{}
	for _, uid := range uids.Uids {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pl, err := qs.cache.Get(x.DataKey(attr, uid))
		if err != nil {
			return err
		}

		vals := make([]types.Val, 1)
		switch {
		case lang != "":
			vals[0], err = pl.ValueForTag(arg.q.ReadTs, lang)

		case isList:
			vals, err = pl.AllUntaggedValues(arg.q.ReadTs)

		default:
			vals[0], err = pl.Value(arg.q.ReadTs)
		}
		if err != nil {
			if err == posting.ErrNoValue {
				continue
			}
			return err
		}

		max := int(arg.srcFn.threshold)
		for _, val := range vals {
			// convert data from binary to appropriate format
			strVal, err := types.Convert(val, types.StringID)
			if err == nil && matchFuzzy(matchQuery, strVal.Value.(string), max) {
				filtered.Uids = append(filtered.Uids, uid)
				// NOTE: We only add the uid once.
				break
			}
		}
	}

	for i := 0; i < len(arg.out.UidMatrix); i++ {
		algo.IntersectWith(arg.out.UidMatrix[i], filtered, arg.out.UidMatrix[i])
	}

	return nil
}

func (qs *queryState) filterGeoFunction(ctx context.Context, arg funcArgs) error {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "filterGeoFunction")
	defer stop()

	attr := arg.q.Attr
	uids := algo.MergeSorted(arg.out.UidMatrix)
	numGo, width := x.DivideAndRule(len(uids.Uids))
	if span != nil && numGo > 1 {
		span.Annotatef(nil, "Number of uids: %d. NumGo: %d. Width: %d\n",
			len(uids.Uids), numGo, width)
	}

	filtered := make([]*pb.List, numGo)
	filter := func(idx, start, end int) error {
		filtered[idx] = &pb.List{}
		out := filtered[idx]
		for _, uid := range uids.Uids[start:end] {
			pl, err := qs.cache.Get(x.DataKey(attr, uid))
			if err != nil {
				return err
			}
			var tv pb.TaskValue
			err = pl.Iterate(arg.q.ReadTs, 0, func(p *pb.Posting) error {
				tv.ValType = p.ValType
				tv.Val = p.Value
				if types.MatchGeo(&tv, arg.srcFn.geoQuery) {
					out.Uids = append(out.Uids, uid)
					return posting.ErrStopIteration
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	}

	errCh := make(chan error, numGo)
	for i := 0; i < numGo; i++ {
		start := i * width
		end := start + width
		if end > len(uids.Uids) {
			end = len(uids.Uids)
		}
		go func(idx, start, end int) {
			errCh <- filter(idx, start, end)
		}(i, start, end)
	}
	for i := 0; i < numGo; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	final := &pb.List{}
	for _, out := range filtered {
		final.Uids = append(final.Uids, out.Uids...)
	}
	if span != nil && numGo > 1 {
		span.Annotatef(nil, "Total uids after filtering geo: %d", len(final.Uids))
	}
	for i := 0; i < len(arg.out.UidMatrix); i++ {
		algo.IntersectWith(arg.out.UidMatrix[i], final, arg.out.UidMatrix[i])
	}
	return nil
}

// TODO: This function is really slow when there are a lot of UIDs to filter, for e.g. when used in
// `has(name)`. We could potentially have a query level cache, which can be used to speed things up
// a bit. Or, try to reduce the number of UIDs which make it here.
func (qs *queryState) filterStringFunction(arg funcArgs) error {
	if glog.V(3) {
		glog.Infof("filterStringFunction. arg: %+v\n", arg.q)
		defer glog.Infof("Done filterStringFunction")
	}
	attr := arg.q.Attr
	uids := algo.MergeSorted(arg.out.UidMatrix)
	var values [][]types.Val
	filteredUids := make([]uint64, 0, len(uids.Uids))
	lang := langForFunc(arg.q.Langs)

	// This iteration must be done in a serial order, because we're also storing the values in a
	// matrix, to check it later.
	// TODO: This function can be optimized by having a query specific cache, which can be populated
	// by the handleHasFunction for e.g. for a `has(name)` query.
	for _, uid := range uids.Uids {
		vals, err := qs.getValsForUID(attr, lang, uid, arg.q.ReadTs)
		switch {
		case err == posting.ErrNoValue:
			continue
		case err != nil:
			return err
		}

		var strVals []types.Val
		for _, v := range vals {
			// convert data from binary to appropriate format
			strVal, err := types.Convert(v, types.StringID)
			if err != nil {
				continue
			}
			strVals = append(strVals, strVal)
		}
		if len(strVals) > 0 {
			values = append(values, strVals)
			filteredUids = append(filteredUids, uid)
		}
	}

	filtered := &pb.List{Uids: filteredUids}
	filter := stringFilter{
		funcName: arg.srcFn.fname,
		funcType: arg.srcFn.fnType,
		lang:     lang,
	}

	switch arg.srcFn.fnType {
	case hasFn:
		// Dont do anything, as filtering based on lang is already
		// done above.
	case fullTextSearchFn:
		filter.tokens = arg.srcFn.tokens
		filter.match = defaultMatch
		filter.tokName = "fulltext"
		filtered = matchStrings(filtered, values, &filter)
	case standardFn:
		filter.tokens = arg.srcFn.tokens
		filter.match = defaultMatch
		filter.tokName = "term"
		filtered = matchStrings(filtered, values, &filter)
	case customIndexFn:
		filter.tokens = arg.srcFn.tokens
		filter.match = defaultMatch
		filter.tokName = arg.q.SrcFunc.Args[0]
		filtered = matchStrings(filtered, values, &filter)
	case compareAttrFn:
		filter.ineqValue = arg.srcFn.ineqValue
		filter.eqVals = arg.srcFn.eqTokens
		filter.match = ineqMatch
		filtered = matchStrings(filtered, values, &filter)
	}

	for i := 0; i < len(arg.out.UidMatrix); i++ {
		algo.IntersectWith(arg.out.UidMatrix[i], filtered, arg.out.UidMatrix[i])
	}
	return nil
}

func (qs *queryState) getValsForUID(attr, lang string, uid, ReadTs uint64) ([]types.Val, error) {
	key := x.DataKey(attr, uid)
	pl, err := qs.cache.Get(key)
	if err != nil {
		return nil, err
	}

	var vals []types.Val
	var val types.Val
	if lang == "" {
		if schema.State().IsList(attr) {
			// NOTE: we will never reach here if this function is called from handleHasFunction, as
			// @lang is not allowed for list predicates.
			vals, err = pl.AllValues(ReadTs)
		} else {
			val, err = pl.Value(ReadTs)
			vals = append(vals, val)
		}
	} else {
		val, err = pl.ValueForTag(ReadTs, lang)
		vals = append(vals, val)
	}

	return vals, err
}

func matchRegex(value types.Val, regex *cregexp.Regexp) bool {
	return len(value.Value.(string)) > 0 && regex.MatchString(value.Value.(string), true, true) > 0
}

type functionContext struct {
	tokens        []string
	geoQuery      *types.GeoQueryData
	intersectDest bool
	ineqValue     types.Val
	// eqTokens is used by compareAttr functions. It stores values corresponding to each
	// function argument. There could be multiple arguments to `eq` function but only one for
	// other compareAttr functions.
	// TODO(@Animesh): change field names which could explain their uses better. Check if we
	// really need all of ineqValue, eqTokens, tokens
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

func ensureArgsCount(srcFunc *pb.SrcFunction, expected int) error {
	if len(srcFunc.Args) != expected {
		return errors.Errorf("Function '%s' requires %d arguments, but got %d (%v)",
			srcFunc.Name, expected, len(srcFunc.Args), srcFunc.Args)
	}
	return nil
}

func checkRoot(q *pb.Query, fc *functionContext) {
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

func parseSrcFn(q *pb.Query) (*functionContext, error) {
	fnType, f := parseFuncType(q.SrcFunc)
	attr := q.Attr
	fc := &functionContext{fnType: fnType, fname: f}
	isIndexedAttr := schema.State().IsIndexed(schema.ReadCtx, attr)
	var err error

	t, err := schema.State().TypeOf(attr)
	if err == nil && fnType != notAFunction && t.Name() == types.StringID.Name() {
		fc.isStringFn = true
	}

	switch fnType {
	case notAFunction:
		fc.n = len(q.UidList.Uids)
	case aggregatorFn:
		// confirm aggregator could apply on the attributes
		typ, err := schema.State().TypeOf(attr)
		if err != nil {
			return nil, errors.Errorf("Attribute %q is not scalar-type", attr)
		}
		if !couldApplyAggregatorOn(f, typ) {
			return nil, errors.Errorf("Aggregator %q could not apply on %v",
				f, attr)
		}
		fc.n = len(q.UidList.Uids)
	case compareAttrFn:
		args := q.SrcFunc.Args
		// Only eq can have multiple args. It should have atleast one.
		if fc.fname == eq {
			if len(args) < 1 {
				return nil, errors.Errorf("eq expects atleast 1 argument.")
			}
		} else { // Others can have only 1 arg.
			if len(args) != 1 {
				return nil, errors.Errorf("%+v expects only 1 argument. Got: %+v",
					fc.fname, args)
			}
		}

		var tokens []string
		// eq can have multiple args.
		for _, arg := range args {
			if fc.ineqValue, err = convertValue(attr, arg); err != nil {
				return nil, errors.Errorf("Got error: %v while running: %v", err,
					q.SrcFunc)
			}
			fc.eqTokens = append(fc.eqTokens, fc.ineqValue)
			if !isIndexedAttr {
				// In case of non-indexed predicate we won't have any tokens.
				continue
			}

			var lang string
			if len(q.Langs) > 0 {
				// Only one language is allowed.
				lang = q.Langs[0]
			}

			// Get tokens ge / le ineqValueToken.
			if tokens, fc.ineqValueToken, err = getInequalityTokens(q.ReadTs, attr, f, lang,
				fc.ineqValue); err != nil {
				return nil, err
			}
			if len(tokens) == 0 {
				continue
			}
			fc.tokens = append(fc.tokens, tokens...)
		}

		// In case of non-indexed predicate, there won't be any tokens. We will fetch value
		// from data keys.
		// If number of index keys is more than no. of uids to filter, so its better to fetch values
		// from data keys directly and compare. Lets make tokens empty.
		// We don't do this for eq because eq could have multiple arguments and we would have to
		// compare the value with all of them. Also eq would usually have less arguments, hence we
		// won't be fetching many index keys.
		switch {
		case q.UidList != nil && !isIndexedAttr:
			fc.n = len(q.UidList.Uids)
		case q.UidList != nil && len(fc.tokens) > len(q.UidList.Uids) && fc.fname != eq:
			fc.tokens = fc.tokens[:0]
			fc.n = len(q.UidList.Uids)
		default:
			fc.n = len(fc.tokens)
		}
	case compareScalarFn:
		if err = ensureArgsCount(q.SrcFunc, 1); err != nil {
			return nil, err
		}
		if fc.threshold, err = strconv.ParseInt(q.SrcFunc.Args[0], 0, 64); err != nil {
			return nil, errors.Wrapf(err, "Compare %v(%v) require digits, but got invalid num",
				q.SrcFunc.Name, q.SrcFunc.Args[0])
		}
		checkRoot(q, fc)
	case geoFn:
		// For geo functions, we get extra information used for filtering.
		fc.tokens, fc.geoQuery, err = types.GetGeoTokens(q.SrcFunc)
		tok.EncodeGeoTokens(fc.tokens)
		if err != nil {
			return nil, err
		}
		fc.n = len(fc.tokens)
	case passwordFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		fc.n = len(q.UidList.Uids)
	case standardFn, fullTextSearchFn:
		// srcfunc 0th val is func name and and [2:] are args.
		// we tokenize the arguments of the query.
		if err = ensureArgsCount(q.SrcFunc, 1); err != nil {
			return nil, err
		}
		required, found := verifyStringIndex(attr, fnType)
		if !found {
			return nil, errors.Errorf("Attribute %s is not indexed with type %s", attr, required)
		}
		if fc.tokens, err = getStringTokens(q.SrcFunc.Args, langForFunc(q.Langs), fnType); err != nil {
			return nil, err
		}
		fc.intersectDest = needsIntersect(f)
		fc.n = len(fc.tokens)
	case matchFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		required, found := verifyStringIndex(attr, fnType)
		if !found {
			return nil, errors.Errorf("Attribute %s is not indexed with type %s", attr, required)
		}
		fc.intersectDest = needsIntersect(f)
		// Max Levenshtein distance
		var s string
		s, q.SrcFunc.Args = q.SrcFunc.Args[1], q.SrcFunc.Args[:1]
		max, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return nil, errors.Errorf("Levenshtein distance value must be an int, got %v", s)
		}
		if max < 0 {
			return nil, errors.Errorf("Levenshtein distance value must be greater than 0, got %v", s)
		}
		fc.threshold = int64(max)
		fc.tokens = q.SrcFunc.Args
		fc.n = len(fc.tokens)
	case customIndexFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		tokerName := q.SrcFunc.Args[0]
		if !verifyCustomIndex(q.Attr, tokerName) {
			return nil, errors.Errorf("Attribute %s is not indexed with custom tokenizer %s",
				q.Attr, tokerName)
		}
		valToTok, err := convertValue(q.Attr, q.SrcFunc.Args[1])
		if err != nil {
			return nil, err
		}
		tokenizer, ok := tok.GetTokenizer(tokerName)
		if !ok {
			return nil, errors.Errorf("Could not find tokenizer with name %q", tokerName)
		}
		fc.tokens, _ = tok.BuildTokens(valToTok.Value,
			tok.GetTokenizerForLang(tokenizer, langForFunc(q.Langs)))
		fc.intersectDest = needsIntersect(f)
		fc.n = len(fc.tokens)
	case regexFn:
		if err = ensureArgsCount(q.SrcFunc, 2); err != nil {
			return nil, err
		}
		ignoreCase := false
		modifiers := q.SrcFunc.Args[1]
		if len(modifiers) > 0 {
			if modifiers == "i" {
				ignoreCase = true
			} else {
				return nil, errors.Errorf("Invalid regexp modifier: %s", modifiers)
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
	case hasFn:
		if err = ensureArgsCount(q.SrcFunc, 0); err != nil {
			return nil, err
		}
		checkRoot(q, fc)
	case uidInFn:
		if err = ensureArgsCount(q.SrcFunc, 1); err != nil {
			return nil, err
		}
		fc.uidPresent, err = strconv.ParseUint(q.SrcFunc.Args[0], 0, 64)
		if err != nil {
			if e, ok := err.(*strconv.NumError); ok && e.Err == strconv.ErrSyntax {
				return nil, errors.Errorf("Value %q in %s is not a number",
					q.SrcFunc.Args[0], q.SrcFunc.Name)
			}
			return nil, err
		}
		checkRoot(q, fc)
		if fc.isFuncAtRoot {
			return nil, errors.Errorf("uid_in function not allowed at root")
		}
	default:
		return nil, errors.Errorf("FnType %d not handled in numFnAttrs.", fnType)
	}
	return fc, nil
}

// ServeTask is used to respond to a query.
func (w *grpcWorker) ServeTask(ctx context.Context, q *pb.Query) (*pb.Result, error) {
	ctx, span := otrace.StartSpan(ctx, "worker.ServeTask")
	defer span.End()

	if ctx.Err() != nil {
		return &pb.Result{}, ctx.Err()
	}

	gid, err := groups().BelongsToReadOnly(q.Attr, q.ReadTs)
	switch {
	case err != nil:
		return &pb.Result{}, err
	case gid == 0:
		return &pb.Result{}, errNonExistentTablet
	case gid != groups().groupId():
		return &pb.Result{}, errUnservedTablet
	}

	var numUids int
	if q.UidList != nil {
		numUids = len(q.UidList.Uids)
	}
	span.Annotatef(nil, "Attribute: %q NumUids: %v groupId: %v ServeTask", q.Attr, numUids, gid)

	if !groups().ServesGroup(gid) {
		return &pb.Result{}, errors.Errorf(
			"Temporary error, attr: %q groupId: %v Request sent to wrong server", q.Attr, gid)
	}

	type reply struct {
		result *pb.Result
		err    error
	}
	c := make(chan reply, 1)
	go func() {
		result, err := processTask(ctx, q, gid)
		c <- reply{result, err}
	}()

	select {
	case <-ctx.Done():
		return &pb.Result{}, ctx.Err()
	case reply := <-c:
		return reply.result, reply.err
	}
}

// applyFacetsTree : we return error only when query has some problems.
// like Or has 3 arguments, argument facet val overflows integer.
// returns true if postingFacets can be included.
func applyFacetsTree(postingFacets []*api.Facet, ftree *facetsTree) (bool, error) {
	if ftree == nil {
		return true, nil
	}
	if ftree.function != nil {
		fname := strings.ToLower(ftree.function.name)
		var fc *api.Facet
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
		case compareAttrFn: // lt, gt, le, ge, eq
			var err error
			typId, err := facets.TypeIDFor(fc)
			if err != nil {
				return false, err
			}

			v, err := types.Convert(ftree.function.val, typId)
			if err != nil {
				// ignore facet if not of appropriate type
				return false, nil
			}
			fVal, err := facets.ValFor(fc)
			if err != nil {
				return false, err
			}

			return types.CompareVals(fname, fVal, v), nil

		case standardFn: // allofterms, anyofterms
			facetType, err := facets.TypeIDFor(fc)
			if err != nil {
				return false, err
			}
			if facetType != types.StringID {
				return false, nil
			}
			return filterOnStandardFn(fname, fc.Tokens, ftree.function.tokens)
		}
		return false, errors.Errorf("Fn %s not supported in facets filtering.", fname)
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
	return false, errors.Errorf("Unexpected behavior in applyFacetsTree.")
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
	loop:
		for fidx := 0; aidx < len(argTokens) && fidx < len(fcTokens); {
			switch {
			case fcTokens[fidx] < argTokens[aidx]:
				fidx++
			case fcTokens[fidx] == argTokens[aidx]:
				fidx++
				aidx++
			default:
				// as all of argTokens should match
				// which is not possible now.
				break loop
			}
		}
		return aidx == len(argTokens), nil
	case "anyofterms":
		for aidx, fidx := 0, 0; aidx < len(argTokens) && fidx < len(fcTokens); {
			switch {
			case fcTokens[fidx] < argTokens[aidx]:
				fidx++
			case fcTokens[fidx] == argTokens[aidx]:
				return true, nil
			default:
				aidx++
			}
		}
		return false, nil
	}
	return false, errors.Errorf("Fn %s not supported in facets filtering.", fname)
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

func preprocessFilter(tree *pb.FilterTree) (*facetsTree, error) {
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

		fnType, fname := parseFuncTypeHelper(ftree.function.name)
		if len(tree.Func.Args) != 1 {
			return nil, errors.Errorf("One argument expected in %s, but got %d.",
				fname, len(tree.Func.Args))
		}

		switch fnType {
		case compareAttrFn:
			ftree.function.val = types.Val{Tid: types.StringID, Value: []byte(tree.Func.Args[0])}
		case standardFn:
			argTokens, aerr := tok.GetTermTokens(tree.Func.Args)
			if aerr != nil { // query error ; stop processing.
				return nil, aerr
			}
			sort.Strings(argTokens)
			ftree.function.tokens = argTokens
		default:
			return nil, errors.Errorf("Fn %s not supported in preprocessFilter.", fname)
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
			return nil, errors.Errorf("Expected 1 child for not but got %d.", numChild)
		}
	case "and":
		if numChild != 2 {
			return nil, errors.Errorf("Expected 2 child for not but got %d.", numChild)
		}
	case "or":
		if numChild != 2 {
			return nil, errors.Errorf("Expected 2 child for not but got %d.", numChild)
		}
	default:
		return nil, errors.Errorf("Unsupported operation in facet filtering: %s.", tree.Op)
	}
	return ftree, nil
}

type countParams struct {
	readTs  uint64
	count   int64
	attr    string
	gid     uint32
	reverse bool   // If query is asking for ~pred
	fn      string // function name
}

func (qs *queryState) evaluate(cp countParams, out *pb.Result) error {
	count := cp.count
	var illegal bool
	switch cp.fn {
	case "eq":
		illegal = count <= 0
	case "lt":
		illegal = count <= 1
	case "le":
		illegal = count <= 0
	case "gt":
		illegal = count < 0
	case "ge":
		illegal = count <= 0
	default:
		x.AssertTruef(false, "unhandled count comparison fn: %v", cp.fn)
	}
	if illegal {
		return errors.Errorf("count(predicate) cannot be used to search for " +
			"negative counts (nonsensical) or zero counts (not tracked).")
	}

	countKey := x.CountKey(cp.attr, uint32(count), cp.reverse)
	if cp.fn == "eq" {
		pl, err := qs.cache.Get(countKey)
		if err != nil {
			return err
		}
		uids, err := pl.Uids(posting.ListOptions{ReadTs: cp.readTs})
		if err != nil {
			return err
		}
		out.UidMatrix = append(out.UidMatrix, uids)
		return nil
	}

	switch cp.fn {
	case "lt":
		count--
	case "gt":
		count++
	}

	x.AssertTrue(count >= 1)
	countKey = x.CountKey(cp.attr, uint32(count), cp.reverse)

	txn := pstore.NewTransactionAt(cp.readTs, false)
	defer txn.Discard()

	pk := x.ParsedKey{Attr: cp.attr}
	itOpt := badger.DefaultIteratorOptions
	itOpt.PrefetchValues = false
	itOpt.Reverse = cp.fn == "le" || cp.fn == "lt"
	itOpt.Prefix = pk.CountPrefix(cp.reverse)

	itr := txn.NewIterator(itOpt)
	defer itr.Close()

	for itr.Seek(countKey); itr.Valid(); itr.Next() {
		item := itr.Item()
		pl, err := qs.cache.Get(item.Key())
		if err != nil {
			return err
		}
		uids, err := pl.Uids(posting.ListOptions{ReadTs: cp.readTs})
		if err != nil {
			return err
		}
		out.UidMatrix = append(out.UidMatrix, uids)
	}
	return nil
}

func (qs *queryState) handleHasFunction(ctx context.Context, q *pb.Query, out *pb.Result,
	srcFn *functionContext) error {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "handleHasFunction")
	defer stop()
	if glog.V(3) {
		glog.Infof("handleHasFunction query: %+v\n", q)
	}

	txn := pstore.NewTransactionAt(q.ReadTs, false)
	defer txn.Discard()

	initKey := x.ParsedKey{
		Attr: q.Attr,
	}
	startKey := x.DataKey(q.Attr, q.AfterUid+1)
	prefix := initKey.DataPrefix()
	if q.Reverse {
		// Reverse does not mean reverse iteration. It means we're looking for
		// the reverse index.
		startKey = x.ReverseKey(q.Attr, q.AfterUid+1)
		prefix = initKey.ReversePrefix()
	}

	result := &pb.List{}
	var prevKey []byte
	itOpt := badger.DefaultIteratorOptions
	itOpt.PrefetchValues = false
	itOpt.AllVersions = true
	itOpt.Prefix = prefix
	it := txn.NewIterator(itOpt)
	defer it.Close()

	lang := langForFunc(q.Langs)
	needFiltering := needsStringFiltering(srcFn, q.Langs, q.Attr)

	// This function checks if we should include uid in result or not when has is queried with
	// @lang(eg: has(name@en)). We need to do this inside this function to return correct result
	// for first.
	checkInclusion := func(uid uint64) error {
		if !needFiltering {
			return nil
		}

		_, err := qs.getValsForUID(q.Attr, lang, uid, q.ReadTs)
		return err
	}

loop:
	// This function could be switched to the stream.Lists framework, but after the change to use
	// BitCompletePosting, the speed here is already pretty fast. The slowdown for @lang predicates
	// occurs in filterStringFunction (like has(name) queries).
	for it.Seek(startKey); it.Valid(); {
		item := it.Item()
		if bytes.Equal(item.Key(), prevKey) {
			it.Next()
			continue
		}
		prevKey = append(prevKey[:0], item.Key()...)

		// Parse the key upfront, otherwise ReadPostingList would advance the
		// iterator.
		pk, err := x.Parse(item.Key())
		if err != nil {
			return err
		}

		// The following optimization speeds up this iteration considerably, because it avoids
		// the need to run ReadPostingList.
		if item.UserMeta()&posting.BitEmptyPosting > 0 {
			// This is an empty posting list. So, it should not be included.
			continue
		}
		if item.UserMeta()&posting.BitCompletePosting > 0 {
			// This bit would only be set if there are valid uids in UidPack.
			err := checkInclusion(pk.Uid)
			switch {
			case err == posting.ErrNoValue:
				continue
			case err != nil:
				return err
			}
			result.Uids = append(result.Uids, pk.Uid)

			// We'll stop fetching if we fetch the required count.
			if len(result.Uids) >= int(q.First) {
				break
			}
			continue
		}

		// We do need to copy over the key for ReadPostingList.
		l, err := posting.ReadPostingList(item.KeyCopy(nil), it)
		if err != nil {
			return err
		}
		empty, err := l.IsEmpty(q.ReadTs, 0)
		switch {
		case err != nil:
			return err
		case !empty:
			err := checkInclusion(pk.Uid)
			switch {
			case err == posting.ErrNoValue:
				continue
			case err != nil:
				return err
			}
			result.Uids = append(result.Uids, pk.Uid)

			// We'll stop fetching if we fetch the required count.
			if len(result.Uids) >= int(q.First) {
				break loop
			}
		}

		if len(result.Uids)%100000 == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
	}
	if span != nil {
		span.Annotatef(nil, "handleHasFunction found %d uids", len(result.Uids))
	}
	out.UidMatrix = append(out.UidMatrix, result)
	return nil
}
