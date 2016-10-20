package worker

import (
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/index"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
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
		return result, x.Wrapf(err, "ProcessTaskOverNetwork: while retrieving connection.")
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
	s := task.GetRootAsSort(qu, 0)
	if s.Coarse() == 0 {
		return doFineSort(s)
	}
	return newCoarseSorter(s).run()
}

type coarseSorter struct {
	s       *task.Sort
	attr    string
	offset  int
	count   int
	state   []byte
	offsets []int
	counts  []int
	out     [][]uint64
}

func newCoarseSorter(s *task.Sort) *coarseSorter {
	x.Assert(s != nil)
	x.Assertf(s.Count() >= 0,
		"We do not yet support negative count with sorting: %s %d",
		s.Attr(), s.Count())

	n := s.UidmatrixLength()
	cs := &coarseSorter{
		s:       s,
		attr:    string(s.Attr()),
		offset:  int(s.Offset()),
		count:   int(s.Count()),
		state:   make([]byte, n),
		offsets: make([]int, n),
		out:     make([][]uint64, n), // Intermediate results.
	}

	for i := 0; i < n; i++ {
		cs.offsets[i] = cs.offset
	}

	if cs.count > 0 {
		cs.counts = make([]int, n)
		for i := 0; i < n; i++ {
			cs.counts[i] = cs.count
		}
	}

	for i := 0; i < n; i++ {
		cs.out[i] = make([]uint64, 0, 10)
	}
	return cs
}

// Example: Fix one UID list. Intersect with 4 buckets. Say the size of
// intersections of each bucket is: 30 40 50 30.
// Say offset is 45. In this case, you can only discard the first bucket.
// You have to keep everything from the second bucket as the ordering within
// a bucket is undefined. After dropping the first bucket, the "offset"
// should be modifed to 15.

// runHelper looks up UIDs for one token / bucket and intersect with each
// UID list in the UID matrix. Returns true if all UID lists are done.
func (cs *coarseSorter) runHelper(key []byte) bool {
	pl, decr := posting.GetOrCreate(key, ws.dataStore)
	defer decr()

	// If some UID list is not done, "done" will be set to false.
	done := true
	for i := 0; i < cs.s.UidmatrixLength(); i++ { // Iterate over UID lists.
		var ul task.UidList
		var l algo.UIDList
		x.Assert(cs.s.Uidmatrix(&ul, i))
		l.FromTask(&ul)
		listOpt := posting.ListOptions{Intersect: &l}
		// Intersect index with i-th input UID list.
		result := pl.Uids(listOpt)
		n := result.Size()

		if cs.state[i] == 2 {
			// This UID list is done. No need to process.
			continue
		}
		// By default, we want to output this bucket's intersesction.
		wantBucket := true
		if cs.state[i] == 0 {
			if cs.offsets[i] >= n {
				// We do not need this bucket. Let's not output it.
				cs.offsets[i] -= n
				wantBucket = false
			} else {
				// This is the first bucket we need. Enter new state. Update count.
				x.Printf("~~~## i=%d n=%d offsets[i]=%d", i, n, cs.offsets[i])
				cs.state[i] = 1
				if cs.count > 0 {
					// Say intersection is 100 elements. Offset is 3. So, we're
					// keeping 97 elements from this bucket. Decrement count by 97.
					cs.counts[i] -= (n - cs.offsets[i])
					if cs.counts[i] <= 0 {
						cs.state[i] = 2 // We are done with this UID list.
					}
				}
			}
		} else if cs.state[i] == 1 {
			if cs.count > 0 {
				cs.counts[i] -= n
				if cs.counts[i] <= 0 {
					cs.state[i] = 2 // We are done with this UID list.
				}
			}
		}

		if wantBucket {
			for j := 0; j < result.Size(); j++ {
				cs.out[i] = append(cs.out[i], result.Get(j))
			}
		}

		if cs.state[i] != 2 {
			// There is a UID list that is not done. So we need to continue
			// iterating over buckets.
			done = false
		}
	}
	return done
}

func (cs *coarseSorter) run() ([]byte, error) {
	kt := index.GetTokensTable(cs.attr)
	for token := kt.GetFirst(); len(token) > 0; token = kt.GetNext(token) {
		if cs.runHelper(types.IndexKey(cs.attr, token)) {
			break
		}
	}

	x.Printf("~~~~sort out:%v", cs.out)

	// Convert out to flatbuffers output.
	b := flatbuffers.NewBuilder(0)

	// Add UID matrix.
	n := cs.s.UidmatrixLength()
	uidOffsets := make([]flatbuffers.UOffsetT, 0, n)
	for _, ul := range cs.out {
		var l algo.UIDList
		l.FromUints(ul)
		uidOffsets = append(uidOffsets, l.AddTo(b))
	}

	task.SortResultStartUidmatrixVector(b, n)
	for i := n - 1; i >= 0; i-- {
		b.PrependUOffsetT(uidOffsets[i])
	}
	uend := b.EndVector(n)

	task.SortResultStartOffsetVector(b, n)
	for i := n - 1; i >= 0; i-- {
		b.PrependInt32(int32(cs.offsets[i]))
	}
	offsetOffset := b.EndVector(n)

	task.SortResultStart(b)
	task.SortResultAddUidmatrix(b, uend)
	task.SortResultAddOffset(b, offsetOffset)
	b.Finish(task.SortResultEnd(b))
	return b.FinishedBytes(), nil
}

func doFineSort(s *task.Sort) ([]byte, error) {
	x.Assertf(s.UidmatrixLength() == 1,
		"Expected only one list to be sorted for now: %d", s.UidmatrixLength())

	var ul task.UidList
	x.Assert(s.Uidmatrix(&ul, 0))
	attr := string(s.Attr())
	sType := schema.TypeOf(attr)
	if !sType.IsScalar() {
		return nil, x.Errorf("Cannot sort attribute %s of type object.", attr)
	}
	scalar := sType.(types.Scalar)

	values := make([]types.Value, ul.UidsLength())
	for i := 0; i < ul.UidsLength(); i++ {
		uid := ul.Uids(i)
		val, err := fetchValue(uid, attr, scalar)
		if err != nil {
			return []byte{}, err
		}
		x.Printf("~~~~%s %s", val.String(), val.Type())
		values[i] = val
	}

	scalar.Sort(values)
	x.Printf("~~~sorted! %v", values)
	return []byte{}, nil
}

func fetchValue(uid uint64, attr string, scalar types.Scalar) (types.Value, error) {
	pl, decr := posting.GetOrCreate(posting.Key(uid, attr), ws.dataStore)
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
