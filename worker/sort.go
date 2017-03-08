package worker

import (
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/workerp"
	"github.com/dgraph-io/dgraph/schema"
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
		return processSort(q)
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
	//x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v Sort", q.Attr(), q.UidsLength(), gid)

	var reply *taskp.SortResult
	x.AssertTruef(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", s.Attr, gid)

	c := make(chan error, 1)
	go func() {
		var err error
		reply, err = processSort(s)
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

// processSort does sorting with pagination. It works by iterating over index
// buckets. As it iterates, it intersects with each UID list of the UID
// matrix. To optimize for pagination, we maintain the "offsets and sizes" or
// pagination window for each UID list. For each UID list, we ignore the
// bucket if we haven't hit the offset. We stop getting results when we got
// enough for our pagination params. When all the UID lists are done, we stop
// iterating over the index.
func processSort(ts *taskp.Sort) (*taskp.SortResult, error) {
	attr := ts.Attr
	x.AssertTruef(ts.Count > 0,
		("We do not yet support negative or infinite count with sorting: %s %d. " +
			"Try flipping order and return first few elements instead."),
		attr, ts.Count)

	n := len(ts.UidMatrix)
	out := make([]intersectedList, n)
	for i := 0; i < n; i++ {
		// offsets[i] is the offset for i-th posting list. It gets decremented as we
		// iterate over buckets.
		out[i].offset = int(ts.Offset)
		var emptyList taskp.List
		out[i].ulist = &emptyList
		out[i].excludeSet = make(map[uint64]struct{})
	}

	// Iterate over every bucket / token.
	it := pstore.NewIterator()
	defer it.Close()
	pk := x.Parse(x.IndexKey(attr, ""))
	indexPrefix := pk.IndexPrefix()

	if !ts.Desc {
		it.Seek(indexPrefix)
	} else {
		it.Seek(pk.SkipRangeOfSameType())
		if it.Valid() {
			it.Prev()
		} else {
			it.SeekToLast()
		}
	}

BUCKETS:

	// Outermost loop is over index buckets.
	for it.ValidForPrefix(indexPrefix) {
		k := x.Parse(it.Key().Data())
		x.AssertTrue(k != nil)
		x.AssertTrue(k.IsIndex())
		token := k.Term

		// Intersect every UID list with the index bucket, and update their
		// results (in out).
		err := intersectBucket(ts, attr, token, out)
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

	r := new(taskp.SortResult)
	for _, il := range out {
		r.UidMatrix = append(r.UidMatrix, il.ulist)
	}
	return r, nil
}

type intersectedList struct {
	offset int
	ulist  *taskp.List

	// For term index, a UID might appear in multiple buckets. We want to dedup.
	// We cannot do this at the end of the sort because we do need to track offsets and counts.
	excludeSet map[uint64]struct{}
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
	pl, decr := posting.GetOrCreate(key, 0)
	defer decr()

	// For each UID list, we need to intersect with the index bucket.
	for i, ul := range ts.UidMatrix {
		il := &out[i]
		if count > 0 && len(il.ulist.Uids) >= count {
			continue
		}

		// Intersect index with i-th input UID list.
		listOpt := posting.ListOptions{
			Intersect:  ul,
			ExcludeSet: il.excludeSet,
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
		sortByValue(attr, ts.Langs, result, scalar, ts.Desc)

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
			il.excludeSet[uid] = struct{}{}
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

// sortByValue fetches values and sort UIDList.
func sortByValue(attr string, langs []string, ul *taskp.List, typ types.TypeID, desc bool) error {
	lenList := len(ul.Uids)
	values := make([]types.Val, 0, lenList)
	for i := 0; i < lenList; i++ {
		uid := ul.Uids[i]
		val, err := fetchValue(uid, attr, langs, typ)
		if err != nil {
			return err
		}
		values = append(values, val)
	}
	return types.Sort(typ, values, ul, desc)
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
