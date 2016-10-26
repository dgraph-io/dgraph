package worker

import (
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/posting"
	//	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// SortOverNetwork sends sort query over the network.
func SortOverNetwork(ctx context.Context, qu []byte) (result []byte, rerr error) {
	q := task.GetRootAsSort(qu, 0)
	attr := string(q.Attr())
	gid := BelongsTo(attr)
	x.Trace(ctx, "attr: %v groupId: %v", attr, gid)

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
		return result, x.Wrapf(err, "SortOverNetwork: while retrieving connection.")
	}
	defer pl.Put(conn)
	x.Trace(ctx, "Sending request to %v", addr)

	c := NewWorkerClient(conn)
	reply, err := c.Sort(ctx, &Payload{Data: qu})
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.Sort"))
		return []byte(""), err
	}

	x.Trace(ctx, "Reply from server. length: %v Addr: %v Attr: %v",
		len(reply.Data), addr, attr)
	return reply.Data, nil
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

// processSort does either a coarse or a fine sort.
func processSort(qu []byte) ([]byte, error) {
	ts := task.GetRootAsSort(qu, 0)
	x.Assert(ts != nil)

	attr := string(ts.Attr())
	x.Assertf(ts.Count() >= 0,
		("We do not yet support negative count with sorting: %s %d. " +
			"Try flipping order and return first few elements instead."),
		attr, ts.Count())

	n := ts.UidmatrixLength()
	out := make([][]uint64, n)
	for i := 0; i < n; i++ {
		out[i] = make([]uint64, 0, 10)
	}

	// offsets[i] is the offset for i-th posting list. It gets decremented as we
	// iterate over buckets.
	offsets := make([]int, n)
	for i := 0; i < n; i++ {
		offsets[i] = int(ts.Offset())
	}

	// Iterate over every bucket in TokensTable.
	t := posting.GetTokensTable(attr)
	for token := t.GetFirst(); len(token) > 0; token = t.GetNext(token) {
		key := types.IndexKey(attr, token)
		if intersectBucket(ts, key, offsets, int(ts.Count()), out) {
			break
		}
	}

	// Convert out to flatbuffers output.
	b := flatbuffers.NewBuilder(0)
	uidOffsets := make([]flatbuffers.UOffsetT, 0, n)
	for _, ul := range out {
		var l algo.UIDList
		l.FromUints(ul)
		uidOffsets = append(uidOffsets, l.AddTo(b))
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

func intersectBucket(ts *task.Sort, key []byte, offsets []int, count int,
	out [][]uint64) bool {
	pl, decr := posting.GetOrCreate(key, ws.dataStore)
	defer decr()

	x.Printf("~~~~pl key=[%s] len=%d", string(key), pl.Length())

	for i := 0; i < ts.UidmatrixLength(); i++ { // Iterate over UID lists.
		if count > 0 && len(out[i]) >= count {
			continue
		}
		var result *algo.UIDList
		{
			var l algo.UIDList
			var ul task.UidList
			x.Assert(ts.Uidmatrix(&ul, i))
			l.FromTask(&ul)
			listOpt := posting.ListOptions{Intersect: &l}
			// Intersect index with i-th input UID list.
			result = pl.Uids(listOpt)
		}
		n := result.Size()

		// Check offsets[i].
		if offsets[i] >= n {
			// We are going to skip the whole intersection. No need to do actual
			// sorting. Just update offsets[i].
			offsets[i] -= n
			continue
		}

		// We need to do the sort. Let's do that later.
		////// SORT result before applying offset.
		if offsets[i] > 0 {
			result.Slice(offsets[i], n)
			offsets[i] = 0
			n = result.Size()
		}

		// m is number of elements to copy from result to out.
		m := n
		if count > 0 {
			slack := count - len(out[i])
			if slack < m {
				m = slack
			}
		}

		// Copy from result to out.
		for j := 0; j < m; j++ {
			out[i] = append(out[i], result.Get(j))
		}
	}

	if count == 0 {
		// We are never done early if there is no "count" defined.
		return false
	}
	// Check out[i] sizes for all i.
	for i := 0; i < ts.UidmatrixLength(); i++ { // Iterate over UID lists.
		if len(out[i]) < count {
			return false
		}
		x.Assert(len(out[i]) == count)
	}
	return true
}

// doFineSort sorts the UIDs by their values. The attribute has to be
// specified in the schema with a type that makes sense. We do not return the
// values as they may be long strings. We just return the ordering of the
// UIDs. For example, if the input is 100, 150, 200, 250, and after sorting by
// some attribute, it is 150, 250, 200, 100. Then we will return 1, 3, 2, 0.
//func doFineSort(attr string, s *task.FineSort) ([]byte, error) {
//	ul := s.Uid(nil)
//	x.Assert(ul != nil)

//	sType := schema.TypeOf(attr)
//	if !sType.IsScalar() {
//		return nil, x.Errorf("Cannot sort attribute %s of type object.", attr)
//	}
//	scalar := sType.(types.Scalar)

//	values := make([]types.Value, ul.UidsLength())
//	for i := 0; i < ul.UidsLength(); i++ {
//		uid := ul.Uids(i)
//		val, err := fetchValue(uid, attr, scalar)
//		if err != nil {
//			return []byte{}, err
//		}
//		values[i] = val
//	}

//	idx, err := scalar.Sort(values)
//	if err != nil {
//		return []byte{}, err
//	}

//	// Serialize idx.
//	b := flatbuffers.NewBuilder(0)
//	task.SortResultStartIdxVector(b, len(idx))
//	for i := len(idx) - 1; i >= 0; i-- {
//		b.PrependUint32(idx[i])
//	}
//	idxEnd := b.EndVector(len(idx))
//	task.SortResultStart(b)
//	task.SortResultAddIdx(b, idxEnd)
//	b.Finish(task.SortResultEnd(b))
//	return b.FinishedBytes(), nil
//}

//func fetchValue(uid uint64, attr string, scalar types.Scalar) (types.Value, error) {
//	pl, decr := posting.GetOrCreate(posting.Key(uid, attr), ws.dataStore)
//	defer decr()

//	valBytes, vType, err := pl.Value()
//	if err != nil {
//		return nil, err
//	}
//	val := types.ValueForType(types.TypeID(vType))
//	if val == nil {
//		return nil, x.Errorf("Invalid type: %v", vType)
//	}
//	err = val.UnmarshalBinary(valBytes)
//	if err != nil {
//		return nil, err
//	}

//	schemaVal, err := scalar.Convert(val)
//	if err != nil {
//		return nil, err
//	}
//	return schemaVal, nil
//}
