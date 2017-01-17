package worker

import (
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/keys"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var emptySortResult task.SortResult

// SortOverNetwork sends sort query over the network.
func SortOverNetwork(ctx context.Context, q *task.Sort) (*task.SortResult, error) {
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

	c := NewWorkerClient(conn)
	var reply *task.SortResult
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
func (w *grpcWorker) Sort(ctx context.Context, s *task.Sort) (*task.SortResult, error) {
	if ctx.Err() != nil {
		return &emptySortResult, ctx.Err()
	}

	gid := group.BelongsTo(s.Attr)
	//x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v Sort", q.Attr(), q.UidsLength(), gid)

	var reply *task.SortResult
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

// processSort does either a coarse or a fine sort.
func processSort(ts *task.Sort) (*task.SortResult, error) {
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
		out[i].ulist = &task.List{Uids: []uint64{}}
	}

	// Iterate over every bucket / token.
	it := pstore.NewIterator()
	defer it.Close()
	pk := keys.Parse(keys.IndexKey(attr, "", ts.PluginContexts))
	indexPrefix := pk.IndexPrefix()

	if !ts.Desc {
		it.Seek(indexPrefix)
	} else {
		it.Seek(pk.SkipRangeOfSameType())
		it.Prev()
	}

BUCKETS:

	for it.ValidForPrefix(indexPrefix) {
		k := keys.Parse(it.Key().Data())
		x.AssertTrue(k != nil)
		x.AssertTrue(k.IsIndex())
		token := k.Term

		err := intersectBucket(ts, attr, token, out)
		switch err {
		case errDone:
			break BUCKETS
		case errContinue:
			// Continue iterating over tokens.
		default:
			return &emptySortResult, err
		}
		if ts.Desc {
			it.Prev()
		} else {
			it.Next()
		}
	}

	r := new(task.SortResult)
	for _, il := range out {
		r.UidMatrix = append(r.UidMatrix, il.ulist)
	}
	return r, nil
}

type intersectedList struct {
	offset int
	ulist  *task.List
}

func intersectBucket(ts *task.Sort, attr, token string, out []intersectedList) error {
	count := int(ts.Count)
	sType, err := schema.TypeOf(attr)
	if err != nil || !sType.IsScalar() {
		return x.Errorf("Cannot sort attribute %s of type object.", attr)
	}
	scalar := sType

	key := keys.IndexKey(attr, token, ts.PluginContexts)
	pl, decr := posting.GetOrCreate(key, 0)
	defer decr()

	for i, ul := range ts.UidMatrix {
		il := &out[i]
		if count > 0 && len(il.ulist.Uids) >= count {
			continue
		}

		// Intersect index with i-th input UID list.
		listOpt := posting.ListOptions{Intersect: ul}
		result := pl.Uids(listOpt)
		n := len(result.Uids)

		// Check offsets[i].
		if il.offset >= n {
			// We are going to skip the whole intersection. No need to do actual
			// sorting. Just update offsets[i].
			il.offset -= n
			continue
		}

		// Sort results by value before applying offset.
		sortByValue(attr, result, scalar, ts.Desc, ts.PluginContexts)

		if il.offset > 0 {
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

		// Copy from result to out.
		for j := 0; j < n; j++ {
			il.ulist.Uids = append(il.ulist.Uids, result.Uids[j])
		}
	} // end for loop

	// Check out[i] sizes for all i.
	for i := 0; i < len(ts.UidMatrix); i++ { // Iterate over UID lists.
		if len(out[i].ulist.Uids) < count {
			return errContinue
		}
		x.AssertTrue(len(out[i].ulist.Uids) == count)
	}
	return errDone
}

// sortByValue fetches values and sort UIDList.
func sortByValue(attr string, ul *task.List, typ types.TypeID, desc bool,
	pluginContexts []string) error {
	values := make([]types.Val, len(ul.Uids))
	for i, uid := range ul.Uids {
		val, err := fetchValue(uid, attr, typ, pluginContexts)
		if err != nil {
			return err
		}
		values[i] = val
	}
	return types.Sort(typ, values, ul, desc)
}

// fetchValue gets the value for a given UID.
func fetchValue(uid uint64, attr string, scalar types.TypeID,
	pluginContexts []string) (types.Val, error) {
	// TODO: Maybe use posting.Get
	pl, decr := posting.GetOrCreate(keys.DataKey(attr, uid, pluginContexts), group.BelongsTo(attr))
	defer decr()

	src, err := pl.Value()
	if err != nil {
		return types.Val{}, err
	}
	dst := types.ValueForType(scalar)
	err = types.Convert(src, &dst)
	if err != nil {
		return types.Val{}, err
	}

	return dst, nil
}
