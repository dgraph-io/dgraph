package worker

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
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

	// Iterate over every bucket in TokensTable.
	t := posting.GetTokensTable(attr)

BUCKETS:
	for token := t.GetFirst(); len(token) > 0; token = t.GetNext(token) {
		err := intersectBucket(ts, attr, token, out)
		switch err {
		case errDone:
			break BUCKETS
		case errContinue:
			// Continue iterating over tokens.
		default:
			return &emptySortResult, err
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
	sType := schema.TypeOf(attr)
	if !sType.IsScalar() {
		return x.Errorf("Cannot sort attribute %s of type object.", attr)
	}
	scalar := sType.(types.TypeID)

	key := x.IndexKey(attr, token)
	pl, decr := posting.GetOrCreate(key)
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
		sortByValue(attr, result, scalar)

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
func sortByValue(attr string, ul *task.List, scalar types.TypeID) error {
	values := make([]interface{}, len(ul.Uids))
	for i, uid := range ul.Uids {
		val, err := fetchValue(uid, attr, scalar)
		if err != nil {
			return err
		}
		values[i] = val
		fmt.Println("**", val, scalar, attr)
	}
	return types.Sort(scalar, values, ul)
}

// fetchValue gets the value for a given UID.
func fetchValue(uid uint64, attr string, scalar types.TypeID) (interface{}, error) {
	pl, decr := posting.GetOrCreate(x.DataKey(attr, uid))
	defer decr()

	valBytes, vType, err := pl.Value()
	if err != nil {
		return nil, err
	}
	vID := types.TypeID(vType)
	val := types.ValueForType(scalar)
	if val == nil {
		return nil, x.Errorf("Invalid type: %v", vType)
	}
	err = types.Convert(vID, scalar, valBytes, &val)
	if err != nil {
		return nil, err
	}

	return val, nil
}
