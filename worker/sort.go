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
	"strings"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var emptySortResult taskp.SortResult

// SortOverNetwork sends sort query over the network.
func SortOverNetwork(ctx context.Context, q *taskp.Sort) (*taskp.SortResult, error) {
	gid := group.BelongsTo(q.Attr)
	x.Trace(ctx, "worker.Sort attr: %v groupId: %v", q.Attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processSort(ctx, q)
	}

	// Send this over the network.
	// TODO: Send the request to multiple servers as described in Jeff Dean's talk.
	addr := groups().AnyServer(gid)
	pl := pools().get(addr)

	conn, err := pl.Get()
	if err != nil {
		return &emptySortResult, x.Wrapf(err, "SortOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := workerp.NewWorkerClient(conn)
	var reply *taskp.SortResult
	cerr := make(chan error, 1)
	go func() {
		var err error
		reply, err = c.Sort(ctx, q)
		cerr <- err
	}()

	select {
	case <-ctx.Done():
		return &emptySortResult, ctx.Err()
	case err := <-cerr:
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.Sort"))
		}
		return reply, err
	}
}

// Sort is used to sort given UID matrix.
func (w *grpcWorker) Sort(ctx context.Context, s *taskp.Sort) (*taskp.SortResult, error) {
	if ctx.Err() != nil {
		return &emptySortResult, ctx.Err()
	}

	gid := group.BelongsTo(s.Attr)
	x.Trace(ctx, "Sorting: Attribute: %q groupId: %v Sort", s.Attr, gid)

	var reply *taskp.SortResult
	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", s.Attr, gid)

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

func sortWithoutIndex(ctx context.Context, ts *taskp.Sort) (*taskp.SortResult, error) {
	n := len(ts.UidMatrix)
	r := new(taskp.SortResult)
	// Sort and paginate directly as it'd be expensive to iterate over the index which
	// might have millions of keys just for retrieving some values.
	sType, err := schema.State().TypeOf(ts.Attr)
	if err != nil || !sType.IsScalar() {
		return r, x.Errorf("Cannot sort attribute %s of type object.", ts.Attr)
	}
	if sType == types.BoolID {
		return r, x.Errorf("Cannot sort attribute %v of type bool", ts.Attr)
	}
	for i := 0; i < n; i++ {
		select {
		case <-ctx.Done():
			return nil, nil
		default:
			// Copy, otherwise it'd affect the destUids and hence the srcUids of Next level.
			tempList := &taskp.List{ts.UidMatrix[i].Uids}
			if err := sortByValue(ts.Attr, ts.Langs, tempList, sType, ts.Desc); err != nil {
				return r, err
			}
			paginate(int(ts.Offset), int(ts.Count), tempList)
			r.UidMatrix = append(r.UidMatrix, tempList)
		}
	}
	return r, nil
}

func sortWithIndex(ctx context.Context, ts *taskp.Sort) (*taskp.SortResult, error) {
	n := len(ts.UidMatrix)
	out := make([]intersectedList, n)
	for i := 0; i < n; i++ {
		// offsets[i] is the offset for i-th posting list. It gets decremented as we
		// iterate over buckets.
		out[i].offset = int(ts.Offset)
		var emptyList taskp.List
		out[i].ulist = &emptyList
	}
	r := new(taskp.SortResult)
	// Iterate over every bucket / token.
	it := pstore.NewIterator()
	defer it.Close()

	typ, err := schema.State().TypeOf(ts.Attr)
	if err != nil {
		return &emptySortResult, fmt.Errorf("Attribute %s not defined in schema", ts.Attr)
	}

	// Get the tokenizers and choose the corresponding one.
	if !schema.State().IsIndexed(ts.Attr) {
		return &emptySortResult, x.Errorf("Attribute %s is not indexed.", ts.Attr)
	}

	tokenizers := schema.State().Tokenizer(ts.Attr)
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
			return &emptySortResult, x.Errorf("Attribute:%s does not have exact index for sorting.",
				ts.Attr)
		}
		// Other types just have one tokenizer, so if we didn't find a
		// sortable tokenizer, then attribute isn't sortable.
		return &emptySortResult, x.Errorf("Attribute:%s is not sortable.", ts.Attr)
	}

	indexPrefix := x.IndexKey(ts.Attr, string(tokenizer.Identifier()))
	if !ts.Desc {
		// We need to seek to the first key of this index type.
		seekKey := indexPrefix
		it.Seek(seekKey)
	} else {
		// We need to reach the last key of this index type.
		seekKey := x.IndexKey(ts.Attr, string(tokenizer.Identifier()+1))
		it.SeekForPrev(seekKey)
	}

BUCKETS:

	// Outermost loop is over index buckets.
	for it.ValidForPrefix(indexPrefix) {
		select {
		case <-ctx.Done():
			return nil, nil
		default:
			k := x.Parse(it.Key().Data())
			x.AssertTrue(k != nil)
			x.AssertTrue(k.IsIndex())
			token := k.Term
			x.Trace(ctx, "processSort: Token: %s", token)
			// Intersect every UID list with the index bucket, and update their
			// results (in out).
			err := intersectBucket(ts, ts.Attr, token, out)
			switch err {
			case errDone:
				break BUCKETS
			case errContinue:
				// Continue iterating over tokens / index buckets.
			default:
				return &emptySortResult, err
			}
			if ts.Desc {
				it.Prev()
			} else {
				it.Next()
			}
		}
	}

	for _, il := range out {
		r.UidMatrix = append(r.UidMatrix, il.ulist)
	}
	select {
	case <-ctx.Done():
		return nil, nil
	default:
		return r, nil
	}
}

// processSort does sorting with pagination. It works by iterating over index
// buckets. As it iterates, it intersects with each UID list of the UID
// matrix. To optimize for pagination, we maintain the "offsets and sizes" or
// pagination window for each UID list. For each UID list, we ignore the
// bucket if we haven't hit the offset. We stop getting results when we got
// enough for our pagination params. When all the UID lists are done, we stop
// iterating over the index.
func processSort(ctx context.Context, ts *taskp.Sort) (*taskp.SortResult, error) {
	if ts.Count < 0 {
		return nil, x.Errorf("We do not yet support negative or infinite count with sorting: %s %d. "+
			"Try flipping order and return first few elements instead.", ts.Attr, ts.Count)
	}
	attrData := strings.Split(ts.Attr, "@")
	ts.Attr = attrData[0]
	if len(attrData) == 2 {
		ts.Langs = strings.Split(attrData[1], ":")
	}

	type result struct {
		err error
		res *taskp.SortResult
	}
	cctx, cancel := context.WithCancel(ctx)
	resCh := make(chan result, 2)
	go func() {
		r, err := sortWithoutIndex(cctx, ts)
		resCh <- result{
			res: r,
			err: err,
		}
	}()

	go func() {
		r, err := sortWithIndex(cctx, ts)
		resCh <- result{
			res: r,
			err: err,
		}
	}()

	r := <-resCh
	if r.err == nil {
		cancel()
		// wait for other goroutine to get cancelled
		<-resCh
	} else {
		x.TraceError(ctx, r.err)
		r = <-resCh
	}
	return r.res, r.err
}

type intersectedList struct {
	offset int
	ulist  *taskp.List
}

// intersectBucket intersects every UID list in the UID matrix with the
// indexed bucket.
func intersectBucket(ts *taskp.Sort, attr, token string, out []intersectedList) error {
	count := int(ts.Count)
	sType, err := schema.State().TypeOf(attr)
	if err != nil || !sType.IsScalar() {
		return x.Errorf("Cannot sort attribute %s of type object.", attr)
	}
	scalar := sType

	key := x.IndexKey(attr, token)
	pl, decr := posting.GetOrCreate(key, 1)
	defer decr()

	// For each UID list, we need to intersect with the index bucket.
	for i, ul := range ts.UidMatrix {
		il := &out[i]
		if count > 0 && len(il.ulist.Uids) >= count {
			continue
		}

		// Intersect index with i-th input UID list.
		listOpt := posting.ListOptions{
			Intersect: ul,
		}
		result := pl.Uids(listOpt) // The actual intersection work is done here.
		n := len(result.Uids)

		// Check offsets[i].
		if il.offset >= n {
			// We are going to skip the whole intersection. No need to do actual
			// sorting. Just update offsets[i]. We now offset less.
			il.offset -= n
			continue
		}

		// We are within the page. We need to apply sorting.
		// Sort results by value before applying offset.
		if err := sortByValue(attr, ts.Langs, result, scalar, ts.Desc); err != nil {
			return err
		}

		if il.offset > 0 {
			// Apply the offset.
			result.Uids = result.Uids[il.offset:n]
			il.offset = 0
			n = len(result.Uids)
		}

		// n is number of elements to copy from result to out.
		if count > 0 {
			slack := count - len(il.ulist.Uids)
			if slack < n {
				n = slack
			}
		}

		for j := 0; j < n; j++ {
			uid := result.Uids[j]
			il.ulist.Uids = append(il.ulist.Uids, uid)
		}
	} // end for loop over UID lists in UID matrix.

	// Check out[i] sizes for all i.
	for i := 0; i < len(ts.UidMatrix); i++ { // Iterate over UID lists.
		if len(out[i].ulist.Uids) < count {
			return errContinue
		}

		x.AssertTruef(len(out[i].ulist.Uids) == count, "%d %d", len(out[i].ulist.Uids), count)
	}
	// All UID lists have enough items (according to pagination). Let's notify
	// the outermost loop.
	return errDone
}

func paginate(offset, count int, dest *taskp.List) {
	start, end := x.PageRange(count, offset, len(dest.Uids))
	dest.Uids = dest.Uids[start:end]
}

// sortByValue fetches values and sort UIDList.
func sortByValue(attr string, langs []string, ul *taskp.List, typ types.TypeID, desc bool) error {
	lenList := len(ul.Uids)
	var uids []uint64
	values := make([]types.Val, 0, lenList)
	for i := 0; i < lenList; i++ {
		uid := ul.Uids[i]
		val, err := fetchValue(uid, attr, langs, typ)
		if err != nil {
			// If a value is missing, skip that UID in the result.
			continue
		}
		uids = append(uids, uid)
		values = append(values, val)
	}
	err := types.Sort(typ, values, &taskp.List{uids}, desc)
	ul.Uids = uids
	return err
}

// fetchValue gets the value for a given UID.
func fetchValue(uid uint64, attr string, langs []string, scalar types.TypeID) (types.Val, error) {
	// TODO: Maybe use posting.Get
	pl, decr := posting.GetOrCreate(x.DataKey(attr, uid), group.BelongsTo(attr))
	defer decr()

	src, err := pl.ValueFor(langs)
	if err != nil {
		return types.Val{}, err
	}
	dst, err := types.Convert(src, scalar)
	if err != nil {
		return types.Val{}, err
	}

	return dst, nil
}
