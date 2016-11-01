package worker

import (
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// SortOverNetwork sends sort query over the network.
func SortOverNetwork(ctx context.Context, qu []byte) ([]byte, error) {
	q := task.GetRootAsSort(qu, 0)
	attr := string(q.Attr())
	gid := BelongsTo(attr)
	x.Trace(ctx, "worker.Sort attr: %v groupId: %v", attr, gid)

	if groups().ServesGroup(gid) {
		// No need for a network call, as this should be run from within this instance.
		return processSort(qu)
	}

	// Send this over the network.
	// TODO: Send the request to multiple servers as described in Jeff Dean's talk.
	addr := groups().AnyServer(gid)
	pl := pools().get(addr)

	conn, err := pl.Get()
	if err != nil {
		return []byte{}, x.Wrapf(err, "SortOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := NewWorkerClient(conn)
	reply := new(Payload)
	cerr := make(chan error, 1)
	go func() {
		var err error
		reply, err = c.Sort(ctx, &Payload{Data: qu})
		cerr <- err
	}()

	select {
	case <-ctx.Done():
		return []byte{}, ctx.Err()
	case err := <-cerr:
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.Sort"))
		}
		return reply.Data, err
	}
}

// Sort is used to sort given UID matrix.
func (w *grpcWorker) Sort(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	s := task.GetRootAsSort(query.Data, 0)
	gid := BelongsTo(string(s.Attr()))
	//x.Trace(ctx, "Attribute: %q NumUids: %v groupId: %v Sort", q.Attr(), q.UidsLength(), gid)

	reply := new(Payload)
	x.Assertf(groups().ServesGroup(gid),
		"attr: %q groupId: %v Request sent to wrong server.", s.Attr(), gid)

	c := make(chan error, 1)
	go func() {
		var err error
		reply.Data, err = processSort(query.Data)
		c <- err
	}()

	select {
	case <-ctx.Done():
		return reply, ctx.Err()
	case err := <-c:
		return reply, err
	}
}

var (
	errContinue = x.Errorf("Continue processing buckets")
	errDone     = x.Errorf("Done processing buckets")
)

// processSort does either a coarse or a fine sort.
func processSort(qu []byte) ([]byte, error) {
	ts := task.GetRootAsSort(qu, 0)
	x.AssertTrue(ts != nil)

	attr := string(ts.Attr())
	x.Assertf(ts.Count() > 0,
		("We do not yet support negative or infinite count with sorting: %s %d. " +
			"Try flipping order and return first few elements instead."),
		attr, ts.Count())

	n := ts.UidmatrixLength()
	out := make([]intersectedList, n)
	for i := 0; i < n; i++ {
		// offsets[i] is the offset for i-th posting list. It gets decremented as we
		// iterate over buckets.
		out[i].offset = int(ts.Offset())
		out[i].ulist = algo.NewUIDList([]uint64{})
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
			return []byte{}, err
		}
	}

	// Convert out to flatbuffers output.
	b := flatbuffers.NewBuilder(0)
	uidOffsets := make([]flatbuffers.UOffsetT, 0, n)
	for _, il := range out {
		uidOffsets = append(uidOffsets, il.ulist.AddTo(b))
	}
	task.SortResultStartUidmatrixVector(b, n)
	for i := n - 1; i >= 0; i-- {
		b.PrependUOffsetT(uidOffsets[i])
	}
	uend := b.EndVector(n)
	task.SortResultStart(b)
	task.SortResultAddUidmatrix(b, uend)
	b.Finish(task.SortResultEnd(b))
	return b.FinishedBytes(), nil
}

type intersectedList struct {
	offset int
	ulist  *algo.UIDList
}

func intersectBucket(ts *task.Sort, attr string, token string, out []intersectedList) error {
	count := int(ts.Count())
	sType := schema.TypeOf(attr)
	if !sType.IsScalar() {
		return x.Errorf("Cannot sort attribute %s of type object.", attr)
	}
	scalar := sType.(types.Scalar)

	key := types.IndexKey(attr, token)
	pl, decr := posting.GetOrCreate(key)
	defer decr()

	for i := 0; i < ts.UidmatrixLength(); i++ { // Iterate over UID lists.
		il := &out[i]
		if count > 0 && il.ulist.Size() >= count {
			continue
		}
		var result *algo.UIDList
		{
			var l algo.UIDList
			var ul task.UidList
			x.AssertTrue(ts.Uidmatrix(&ul, i))
			l.FromTask(&ul)
			listOpt := posting.ListOptions{Intersect: &l}
			// Intersect index with i-th input UID list.
			result = pl.Uids(listOpt)
		}
		n := result.Size()

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
			result.Slice(il.offset, n)
			il.offset = 0
			n = result.Size()
		}

		// n is number of elements to copy from result to out.
		if count > 0 {
			slack := count - il.ulist.Size()
			if slack < n {
				n = slack
			}
		}

		// Copy from result to out.
		for j := 0; j < n; j++ {
			il.ulist.Add(result.Get(j))
		}
	} // end for loop

	// Check out[i] sizes for all i.
	for i := 0; i < ts.UidmatrixLength(); i++ { // Iterate over UID lists.
		if out[i].ulist.Size() < count {
			return errContinue
		}
		x.AssertTrue(out[i].ulist.Size() == count)
	}
	return errDone
}

// sortByValue fetches values and sort UIDList.
func sortByValue(attr string, ul *algo.UIDList, scalar types.Scalar) error {
	values := make([]types.Value, ul.Size())
	for i := 0; i < ul.Size(); i++ {
		uid := ul.Get(i)
		val, err := fetchValue(uid, attr, scalar)
		if err != nil {
			return err
		}
		values[i] = val
	}
	return scalar.Sort(values, ul)
}

// fetchValue gets the value for a given UID.
func fetchValue(uid uint64, attr string, scalar types.Scalar) (types.Value, error) {
	pl, decr := posting.GetOrCreate(posting.Key(uid, attr))
	defer decr()

	valBytes, vType, err := pl.Value()
	if err != nil {
		return nil, err
	}
	val := types.ValueForType(types.TypeID(vType))
	if val == nil {
		return nil, x.Errorf("Invalid type: %v", vType)
	}
	err = val.UnmarshalBinary(valBytes)
	if err != nil {
		return nil, err
	}

	schemaVal, err := scalar.Convert(val)
	if err != nil {
		return nil, err
	}
	return schemaVal, nil
}
