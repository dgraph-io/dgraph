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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/y"
)

var emptySortResult intern.SortResult

type sortresult struct {
	reply *intern.SortResult
	vals  [][]types.Val
	err   error
}

// SortOverNetwork sends sort query over the network.
func SortOverNetwork(ctx context.Context, q *intern.SortMessage) (*intern.SortResult, error) {
	gid := groups().BelongsTo(q.Order[0].Attr)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("worker.Sort attr: %v groupId: %v", q.Order[0].Attr, gid)
	}

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processSort(ctx, q)
	}

	result, err := processWithBackupRequest(ctx, gid, func(ctx context.Context, c intern.WorkerClient) (interface{}, error) {
		return c.Sort(ctx, q)
	})
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while calling worker.Sort: %v", err)
		}
		return nil, err
	}
	return result.(*intern.SortResult), nil
}

// Sort is used to sort given UID matrix.
func (w *grpcWorker) Sort(ctx context.Context, s *intern.SortMessage) (*intern.SortResult, error) {
	if ctx.Err() != nil {
		return &emptySortResult, ctx.Err()
	}

	gid := groups().BelongsTo(s.Order[0].Attr)
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Sorting: Attribute: %q groupId: %v Sort", s.Order[0].Attr, gid)
	}

	var reply *intern.SortResult
	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", s.Order[0].Attr, gid)

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

func sortWithoutIndex(ctx context.Context, ts *intern.SortMessage) *sortresult {
	n := len(ts.UidMatrix)
	r := new(intern.SortResult)
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
			tempList := &intern.List{ts.UidMatrix[i].Uids}
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

func sortWithIndex(ctx context.Context, ts *intern.SortMessage) *sortresult {
	n := len(ts.UidMatrix)
	out := make([]intersectedList, n)
	values := make([][]types.Val, 0, n) // Values corresponding to uids in the uid matrix.
	for i := 0; i < n; i++ {
		// offsets[i] is the offset for i-th posting list. It gets decremented as we
		// iterate over buckets.
		out[i].offset = int(ts.Offset)
		var emptyList intern.List
		out[i].ulist = &emptyList
		out[i].uset = map[uint64]struct{}{}
	}

	order := ts.Order[0]
	r := new(intern.SortResult)
	// Iterate over every bucket / token.
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.PrefetchValues = false
	iterOpt.Reverse = order.Desc
	txn := pstore.NewTransactionAt(ts.ReadTs, false)
	defer txn.Discard()

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

	indexPrefix := x.IndexKey(order.Attr, string(tokenizer.Identifier()))
	var seekKey []byte
	if !order.Desc {
		// We need to seek to the first key of this index type.
		seekKey = indexPrefix
	} else {
		// We need to reach the last key of this index type.
		seekKey = x.IndexKey(order.Attr, string(tokenizer.Identifier()+1))
	}
	it := posting.NewTxnPrefixIterator(txn, iterOpt, indexPrefix)
	defer it.Close()
	it.Seek(seekKey)

BUCKETS:

	// Outermost loop is over index buckets.
	for it.Valid() {
		key := it.Key()
		select {
		case <-ctx.Done():
			return &sortresult{&emptySortResult, nil, ctx.Err()}
		default:
			k := x.Parse(key)
			if k == nil {
				it.Next()
				continue
			}

			x.AssertTrue(k.IsIndex())
			token := k.Term
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("processSort: Token: %s", token)
			}
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
			it.Next()
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
	r   *intern.Result
	err error
}

func multiSort(ctx context.Context, r *sortresult, ts *intern.SortMessage) error {
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
		in := &intern.Query{
			Attr:    ts.Order[i].Attr,
			UidList: dest,
			Langs:   ts.Order[i].Langs,
			LinRead: ts.LinRead,
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
		y.MergeLinReads(r.reply.LinRead, result.LinRead)
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
func processSort(ctx context.Context, ts *intern.SortMessage) (*intern.SortResult, error) {
	n := groups().Node
	if err := n.WaitForMinProposal(ctx, ts.LinRead); err != nil {
		return &emptySortResult, err
	}
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
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(r.err.Error())
		}
		r = <-resCh
	}

	if r.err != nil {
		return nil, r.err
	}
	if r.reply.LinRead == nil {
		r.reply.LinRead = &api.LinRead{
			Ids: make(map[uint32]uint64),
		}
	}
	r.reply.LinRead.Ids[n.RaftContext.Group] = n.Applied.DoneUntil()
	// If request didn't have multiple attributes we return.
	if len(ts.Order) <= 1 {
		return r.reply, nil
	}

	err := multiSort(ctx, r, ts)
	return r.reply, err
}

func destUids(uidMatrix []*intern.List) *intern.List {
	included := make(map[uint64]struct{})
	for _, ul := range uidMatrix {
		for _, uid := range ul.Uids {
			included[uid] = struct{}{}
		}
	}

	res := &intern.List{Uids: make([]uint64, 0, len(included))}
	for uid := range included {
		res.Uids = append(res.Uids, uid)
	}
	sort.Slice(res.Uids, func(i, j int) bool { return res.Uids[i] < res.Uids[j] })
	return res
}

func fetchValues(ctx context.Context, in *intern.Query, idx int, or chan orderResult) {
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
	ulist  *intern.List
	values []types.Val
	uset   map[uint64]struct{}
}

// intersectBucket intersects every UID list in the UID matrix with the
// indexed bucket.
func intersectBucket(ctx context.Context, ts *intern.SortMessage, token string,
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
	pl := posting.GetNoStore(key)
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

		// Deduplicate uid list.
		for i := 0; i < len(result.Uids); i++ {
			uid := result.Uids[i]
			if _, ok := il.uset[uid]; ok {
				copy(result.Uids[:i], result.Uids[i+1:])
				result.Uids = result.Uids[:len(result.Uids)-1]
			} else {
				il.uset[uid] = struct{}{}
			}
		}

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

func paginate(ts *intern.SortMessage, dest *intern.List, vals []types.Val) (int, int, error) {
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
func sortByValue(ctx context.Context, ts *intern.SortMessage, ul *intern.List,
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
	err := types.Sort(values, &intern.List{uids}, []bool{order.Desc})
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
	pl := posting.GetNoStore(x.DataKey(attr, uid))

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
