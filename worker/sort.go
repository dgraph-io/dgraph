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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var emptySortResult pb.SortResult

type sortresult struct {
	reply *pb.SortResult
	vals  [][]types.Val
	err   error
}

// SortOverNetwork sends sort query over the network.
func SortOverNetwork(ctx context.Context, q *pb.SortMessage) (*pb.SortResult, error) {
	gid := groups().BelongsTo(q.Order[0].Attr)
	if span := otrace.FromContext(ctx); span != nil {
		span.Annotatef(nil, "worker.SortOverNetwork. Attr: %s. Group: %d", q.Order[0].Attr, gid)
	}

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processSort(ctx, q)
	}

	result, err := processWithBackupRequest(ctx, gid, func(ctx context.Context, c pb.WorkerClient) (interface{}, error) {
		return c.Sort(ctx, q)
	})
	if err != nil {
		return nil, err
	}
	return result.(*pb.SortResult), nil
}

// Sort is used to sort given UID matrix.
func (w *grpcWorker) Sort(ctx context.Context, s *pb.SortMessage) (*pb.SortResult, error) {
	if ctx.Err() != nil {
		return &emptySortResult, ctx.Err()
	}
	ctx, span := otrace.StartSpan(ctx, "worker.Sort")
	defer span.End()

	gid := groups().BelongsTo(s.Order[0].Attr)
	span.Annotatef(nil, "Sorting: Attribute: %q groupId: %v Sort", s.Order[0].Attr, gid)
	if gid != groups().groupId() {
		return nil, x.Errorf("attr: %q groupId: %v Request sent to wrong server.", s.Order[0].Attr, gid)
	}

	var reply *pb.SortResult
	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = processSort(ctx, s)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return &emptySortResult, ctx.Err()
	case err := <-c:
		return reply, err
	}
}

var (
	errContinue = x.Errorf("Continue processing buckets")
	errDone     = x.Errorf("Done processing buckets")
)

func sortWithoutIndex(ctx context.Context, ts *pb.SortMessage) *sortresult {
	span := otrace.FromContext(ctx)
	span.Annotate(nil, "sortWithoutIndex")

	n := len(ts.UidMatrix)
	r := new(pb.SortResult)
	multiSortVals := make([][]types.Val, n)
	// Sort and paginate directly as it'd be expensive to iterate over the index which
	// might have millions of keys just for retrieving some values.
	sType, err := schema.State().TypeOf(ts.Order[0].Attr)
	if err != nil || !sType.IsScalar() {
		return &sortresult{&emptySortResult, nil,
			x.Errorf("Cannot sort attribute %s of type object.", ts.Order[0].Attr)}
	}

	for i := 0; i < n; i++ {
		select {
		case <-ctx.Done():
			return &sortresult{&emptySortResult, nil, ctx.Err()}
		default:
			// Copy, otherwise it'd affect the destUids and hence the srcUids of Next level.
			tempList := &pb.List{Uids: ts.UidMatrix[i].Uids}
			var vals []types.Val
			if vals, err = sortByValue(ctx, ts, tempList, sType); err != nil {
				return &sortresult{&emptySortResult, nil, err}
			}
			start, end, err := paginate(ts, tempList, vals)
			if err != nil {
				return &sortresult{&emptySortResult, nil, err}
			}
			tempList.Uids = tempList.Uids[start:end]
			vals = vals[start:end]
			r.UidMatrix = append(r.UidMatrix, tempList)
			multiSortVals[i] = vals
		}
	}
	return &sortresult{r, multiSortVals, nil}
}

func sortWithIndex(ctx context.Context, ts *pb.SortMessage) *sortresult {
	span := otrace.FromContext(ctx)
	span.Annotate(nil, "sortWithIndex")

	n := len(ts.UidMatrix)
	out := make([]intersectedList, n)
	values := make([][]types.Val, 0, n) // Values corresponding to uids in the uid matrix.
	for i := 0; i < n; i++ {
		// offsets[i] is the offset for i-th posting list. It gets decremented as we
		// iterate over buckets.
		out[i].offset = int(ts.Offset)
		var emptyList pb.List
		out[i].ulist = &emptyList
		out[i].uset = map[uint64]struct{}{}
	}

	order := ts.Order[0]
	typ, err := schema.State().TypeOf(order.Attr)
	if err != nil {
		return &sortresult{&emptySortResult, nil, fmt.Errorf("Attribute %s not defined in schema", order.Attr)}
	}

	// Get the tokenizers and choose the corresponding one.
	if !schema.State().IsIndexed(order.Attr) {
		return &sortresult{&emptySortResult, nil, x.Errorf("Attribute %s is not indexed.", order.Attr)}
	}

	tokenizers := schema.State().Tokenizer(order.Attr)
	var tokenizer tok.Tokenizer
	for _, t := range tokenizers {
		// Get the first sortable index.
		if t.IsSortable() {
			tokenizer = t
			break
		}
	}

	if tokenizer == nil {
		// String type can have multiple tokenizers, only one of which is
		// sortable.
		if typ == types.StringID {
			return &sortresult{&emptySortResult, nil,
				x.Errorf("Attribute:%s does not have exact index for sorting.", order.Attr)}
		}
		// Other types just have one tokenizer, so if we didn't find a
		// sortable tokenizer, then attribute isn't sortable.
		return &sortresult{&emptySortResult, nil, x.Errorf("Attribute:%s is not sortable.", order.Attr)}
	}

	// Iterate over every bucket / token.
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.PrefetchValues = false
	iterOpt.Reverse = order.Desc
	iterOpt.Prefix = x.IndexKey(order.Attr, string(tokenizer.Identifier()))
	txn := pstore.NewTransactionAt(ts.ReadTs, false)
	defer txn.Discard()
	var seekKey []byte
	if !order.Desc {
		// We need to seek to the first key of this index type.
		seekKey = nil // Would automatically seek to iterOpt.Prefix.
	} else {
		// We need to reach the last key of this index type.
		seekKey = x.IndexKey(order.Attr, string(tokenizer.Identifier()+1))
	}
	itr := txn.NewIterator(iterOpt)
	defer itr.Close()

	r := new(pb.SortResult)
BUCKETS:
	// Outermost loop is over index buckets.
	for itr.Seek(seekKey); itr.Valid(); itr.Next() {
		item := itr.Item()
		key := item.Key() // No need to copy.
		select {
		case <-ctx.Done():
			return &sortresult{&emptySortResult, nil, ctx.Err()}
		default:
			k := x.Parse(key)
			if k == nil {
				continue
			}

			x.AssertTrue(k.IsIndex())
			token := k.Term
			// Intersect every UID list with the index bucket, and update their
			// results (in out).
			err := intersectBucket(ctx, ts, token, out)
			switch err {
			case errDone:
				break BUCKETS
			case errContinue:
				// Continue iterating over tokens / index buckets.
			default:
				return &sortresult{&emptySortResult, nil, err}
			}
		}
	}

	for _, il := range out {
		r.UidMatrix = append(r.UidMatrix, il.ulist)
		if len(ts.Order) > 1 {
			// TODO - For lossy tokenizer, no need to pick all values.
			values = append(values, il.values)
		}
	}

	select {
	case <-ctx.Done():
		return &sortresult{&emptySortResult, nil, ctx.Err()}
	default:
		return &sortresult{r, values, nil}
	}
}

type orderResult struct {
	idx int
	r   *pb.Result
	err error
}

func multiSort(ctx context.Context, r *sortresult, ts *pb.SortMessage) error {
	span := otrace.FromContext(ctx)
	span.Annotate(nil, "multiSort")

	// SrcUids for other queries are all the uids present in the response of the first sort.
	dest := destUids(r.reply.UidMatrix)

	// For each uid in dest uids, we have multiple values which belong to different attributes.
	// 1  -> [ "Alice", 23, "1932-01-01"]
	// 10 -> [ "Bob", 35, "1912-02-01" ]
	sortVals := make([][]types.Val, len(dest.Uids))
	for idx := range sortVals {
		sortVals[idx] = make([]types.Val, len(ts.Order))
	}

	seen := make(map[uint64]struct{})
	// Walk through the uidMatrix and put values for this attribute in sortVals.
	for i, ul := range r.reply.UidMatrix {
		x.AssertTrue(len(ul.Uids) == len(r.vals[i]))
		for j, uid := range ul.Uids {
			uidx := algo.IndexOf(dest, uid)
			x.AssertTrue(uidx >= 0)

			if _, ok := seen[uid]; ok {
				// We have already seen this uid.
				continue
			}
			seen[uid] = struct{}{}
			sortVals[uidx][0] = r.vals[i][j]
		}
	}

	// Execute rest of the sorts concurrently.
	och := make(chan orderResult, len(ts.Order)-1)
	for i := 1; i < len(ts.Order); i++ {
		in := &pb.Query{
			Attr:    ts.Order[i].Attr,
			UidList: dest,
			Langs:   ts.Order[i].Langs,
			ReadTs:  ts.ReadTs,
		}
		go fetchValues(ctx, in, i, och)
	}

	var oerr error
	// TODO - Verify behavior with multiple langs.
	for i := 1; i < len(ts.Order); i++ {
		or := <-och
		if or.err != nil {
			if oerr == nil {
				oerr = or.err
			}
			continue
		}

		result := or.r
		x.AssertTrue(len(result.ValueMatrix) == len(dest.Uids))
		for i, _ := range dest.Uids {
			var sv types.Val
			if len(result.ValueMatrix[i].Values) == 0 {
				// Assign nil value which is sorted as greater than all other values.
				sv.Value = nil
			} else {
				v := result.ValueMatrix[i].Values[0]
				val := types.ValueForType(types.TypeID(v.ValType))
				val.Value = v.Val
				var err error
				sv, err = types.Convert(val, val.Tid)
				if err != nil {
					return err
				}
			}
			sortVals[i][or.idx] = sv
		}
	}

	if oerr != nil {
		return oerr
	}

	desc := make([]bool, 0, len(ts.Order))
	for _, o := range ts.Order {
		desc = append(desc, o.Desc)
	}

	// Values have been accumulated, now we do the multisort for each list.
	for i, ul := range r.reply.UidMatrix {
		vals := make([][]types.Val, len(ul.Uids))
		for j, uid := range ul.Uids {
			idx := algo.IndexOf(dest, uid)
			x.AssertTrue(idx >= 0)
			vals[j] = sortVals[idx]
		}
		if err := types.Sort(vals, ul, desc); err != nil {
			return err
		}
		// Paginate
		if len(ul.Uids) > int(ts.Count) {
			ul.Uids = ul.Uids[:ts.Count]
		}
		r.reply.UidMatrix[i] = ul
	}

	return nil
}

// processSort does sorting with pagination. It works by iterating over index
// buckets. As it iterates, it intersects with each UID list of the UID
// matrix. To optimize for pagination, we maintain the "offsets and sizes" or
// pagination window for each UID list. For each UID list, we ignore the
// bucket if we haven't hit the offset. We stop getting results when we got
// enough for our pagination params. When all the UID lists are done, we stop
// iterating over the index.
func processSort(ctx context.Context, ts *pb.SortMessage) (*pb.SortResult, error) {
	span := otrace.FromContext(ctx)
	span.Annotatef(nil, "processSort: Waiting for readTs: %d", ts.ReadTs)
	if err := posting.Oracle().WaitForTs(ctx, ts.ReadTs); err != nil {
		return &emptySortResult, err
	}
	span.Annotate(nil, "Done waiting")

	if ts.Count < 0 {
		return nil, x.Errorf("We do not yet support negative or infinite count with sorting: %s %d. "+
			"Try flipping order and return first few elements instead.", ts.Order[0].Attr, ts.Count)
	}
	if schema.State().IsList(ts.Order[0].Attr) {
		return nil, x.Errorf("Sorting not supported on attr: %s of type: [scalar]", ts.Order[0].Attr)
	}

	cctx, cancel := context.WithCancel(ctx)
	resCh := make(chan *sortresult, 2)
	go func() {
		select {
		case <-time.After(3 * time.Millisecond):
			// Wait between ctx chan and time chan.
		case <-ctx.Done():
			resCh <- &sortresult{err: ctx.Err()}
			return
		}
		r := sortWithoutIndex(cctx, ts)
		resCh <- r
	}()

	go func() {
		sr := sortWithIndex(cctx, ts)
		resCh <- sr
	}()

	r := <-resCh
	if r.err == nil {
		cancel()
		// wait for other goroutine to get cancelled
		<-resCh
	} else {
		span.Annotatef(nil, "processSort error: %v", r.err)
		r = <-resCh
	}

	if r.err != nil {
		return nil, r.err
	}
	// If request didn't have multiple attributes we return.
	if len(ts.Order) <= 1 {
		return r.reply, nil
	}

	err := multiSort(ctx, r, ts)
	return r.reply, err
}

func destUids(uidMatrix []*pb.List) *pb.List {
	included := make(map[uint64]struct{})
	for _, ul := range uidMatrix {
		for _, uid := range ul.Uids {
			included[uid] = struct{}{}
		}
	}

	res := &pb.List{Uids: make([]uint64, 0, len(included))}
	for uid := range included {
		res.Uids = append(res.Uids, uid)
	}
	sort.Slice(res.Uids, func(i, j int) bool { return res.Uids[i] < res.Uids[j] })
	return res
}

func fetchValues(ctx context.Context, in *pb.Query, idx int, or chan orderResult) {
	var err error
	in.Reverse = strings.HasPrefix(in.Attr, "~")
	if in.Reverse {
		in.Attr = strings.TrimPrefix(in.Attr, "~")
	}
	r, err := ProcessTaskOverNetwork(ctx, in)
	or <- orderResult{
		idx: idx,
		err: err,
		r:   r,
	}
}

type intersectedList struct {
	offset int
	ulist  *pb.List
	values []types.Val
	uset   map[uint64]struct{}
}

// intersectBucket intersects every UID list in the UID matrix with the
// indexed bucket.
func intersectBucket(ctx context.Context, ts *pb.SortMessage, token string,
	out []intersectedList) error {
	count := int(ts.Count)
	order := ts.Order[0]
	sType, err := schema.State().TypeOf(order.Attr)
	if err != nil || !sType.IsScalar() {
		return x.Errorf("Cannot sort attribute %s of type object.", order.Attr)
	}
	scalar := sType

	key := x.IndexKey(order.Attr, token)
	// Don't put the Index keys in memory.
	pl, err := posting.GetNoStore(key)
	if err != nil {
		return err
	}
	var vals []types.Val

	// For each UID list, we need to intersect with the index bucket.
	for i, ul := range ts.UidMatrix {
		il := &out[i]
		if count > 0 && len(il.ulist.Uids) >= count {
			continue
		}

		// Intersect index with i-th input UID list.
		listOpt := posting.ListOptions{
			Intersect: ul,
			ReadTs:    ts.ReadTs,
		}
		result, err := pl.Uids(listOpt) // The actual intersection work is done here.
		if err != nil {
			return err
		}

		// Duplicates will exist between buckets if there are multiple language
		// variants of a predicate.
		result.Uids = removeDuplicates(result.Uids, il.uset)

		// Check offsets[i].
		n := len(result.Uids)
		if il.offset >= n {
			// We are going to skip the whole intersection. No need to do actual
			// sorting. Just update offsets[i]. We now offset less.
			il.offset -= n
			continue
		}

		// We are within the page. We need to apply sorting.
		// Sort results by value before applying offset.
		if vals, err = sortByValue(ctx, ts, result, scalar); err != nil {
			return err
		}

		// Result set might have reduced after sorting. As some uids might not have a
		// value in the lang specified.
		n = len(result.Uids)

		if il.offset > 0 {
			// Apply the offset.
			result.Uids = result.Uids[il.offset:n]
			if len(ts.Order) > 1 {
				vals = vals[il.offset:n]
			}
			il.offset = 0
			n = len(result.Uids)
		}

		// n is number of elements to copy from result to out.
		// In case of multiple sort, we dont wan't to apply the count and copy all uids for the
		// current bucket.
		if count > 0 && (len(ts.Order) == 1) {
			slack := count - len(il.ulist.Uids)
			if slack < n {
				n = slack
			}
		}

		il.ulist.Uids = append(il.ulist.Uids, result.Uids[:n]...)
		if len(ts.Order) > 1 {
			il.values = append(il.values, vals[:n]...)
		}
	} // end for loop over UID lists in UID matrix.

	// Check out[i] sizes for all i.
	for i := 0; i < len(ts.UidMatrix); i++ { // Iterate over UID lists.
		if len(out[i].ulist.Uids) < count {
			return errContinue
		}

		if len(ts.Order) == 1 {
			x.AssertTruef(len(out[i].ulist.Uids) == count, "%d %d", len(out[i].ulist.Uids), count)
		}
	}
	// All UID lists have enough items (according to pagination). Let's notify
	// the outermost loop.
	return errDone
}

// removeDuplicates removes elements from uids if they are in set. It also adds
// all uids to set.
func removeDuplicates(uids []uint64, set map[uint64]struct{}) []uint64 {
	for i := 0; i < len(uids); i++ {
		uid := uids[i]
		if _, ok := set[uid]; ok {
			copy(uids[i:], uids[i+1:])
			uids = uids[:len(uids)-1]
			i-- // we just removed an entry, so go back one step
		} else {
			set[uid] = struct{}{}
		}
	}
	return uids
}

func paginate(ts *pb.SortMessage, dest *pb.List, vals []types.Val) (int, int, error) {
	count := int(ts.Count)
	offset := int(ts.Offset)
	start, end := x.PageRange(count, offset, len(dest.Uids))

	// For multiple sort, we need to take all equal values at the end. So we update end.
	for len(ts.Order) > 1 && end < len(dest.Uids) {
		eq, err := types.Equal(vals[end-1], vals[end])
		if err != nil {
			return 0, 0, err
		}
		if !eq {
			break
		}
		end++
	}

	return start, end, nil
}

// sortByValue fetches values and sort UIDList.
func sortByValue(ctx context.Context, ts *pb.SortMessage, ul *pb.List,
	typ types.TypeID) ([]types.Val, error) {
	lenList := len(ul.Uids)
	uids := make([]uint64, 0, lenList)
	values := make([][]types.Val, 0, lenList)
	multiSortVals := make([]types.Val, 0, lenList)
	order := ts.Order[0]
	for i := 0; i < lenList; i++ {
		select {
		case <-ctx.Done():
			return multiSortVals, ctx.Err()
		default:
			uid := ul.Uids[i]
			uids = append(uids, uid)
			val, err := fetchValue(uid, order.Attr, order.Langs, typ, ts.ReadTs)
			if err != nil {
				// Value couldn't be found or couldn't be converted to the sort
				// type.  By using a nil Value, it will appear at the
				// end (start) for orderasc (orderdesc).
				val.Value = nil
			}
			values = append(values, []types.Val{val})
		}
	}
	err := types.Sort(values, &pb.List{Uids: uids}, []bool{order.Desc})
	ul.Uids = uids
	if len(ts.Order) > 1 {
		for _, v := range values {
			multiSortVals = append(multiSortVals, v[0])
		}
	}
	return multiSortVals, err
}

// fetchValue gets the value for a given UID.
func fetchValue(uid uint64, attr string, langs []string, scalar types.TypeID,
	readTs uint64) (types.Val, error) {
	// Don't put the values in memory
	pl, err := posting.GetNoStore(x.DataKey(attr, uid))
	if err != nil {
		return types.Val{}, err
	}

	src, err := pl.ValueFor(readTs, langs)

	if err != nil {
		return types.Val{}, err
	}
	dst, err := types.Convert(src, scalar)
	if err != nil {
		return types.Val{}, err
	}

	return dst, nil
}
