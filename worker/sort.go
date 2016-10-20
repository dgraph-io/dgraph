package worker

import (
	"github.com/google/flatbuffers/go"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/index"
	"github.com/dgraph-io/dgraph/posting"
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
	return doCoarseSort(s)
}

func doFineSort(s *task.Sort) ([]byte, error) {
	return []byte{}, nil
}

func doCoarseSort(s *task.Sort) ([]byte, error) {
	x.Assert(s != nil)
	attr := string(s.Attr())

	m := s.UidmatrixLength()

	// Iterate over buckets, get posting lists, intersect and dump to "out".
	// It is hard to write directly to a flatbuffer output because you have to
	// output one UID list at a time, whereas we process bucket by bucket.
	kt := index.GetTokensTable(attr)
	out := make([][]uint64, m) // Store the intermediate results.
	for i := 0; i < m; i++ {
		out[i] = make([]uint64, 0, 10)
	}

	offset := int(s.Offset())
	count := int(s.Count())
	x.Assertf(count >= 0,
		"We do not yet support negative count with sorting: %d", count)

	// Example: Fix one UID list. Intersect with 4 buckets. Say the size of
	// intersections of each bucket is: 30 40 50 30.
	// Say offset is 45. In this case, you can only discard the first bucket.
	// You have to keep everything from the second bucket as the ordering within
	// a bucket is undefined. After dropping the first bucket, the "offset"
	// should be modifed to 15.

	// State 0: Waiting to hit first bucket that we want to include.
	// State 1: Waiting to hit last bucket that we want to include.
	// State 2: Done with this UID list.
	state := make([]byte, m)

	// offsets[i] is the offset for i-th UID list. As we intersect with buckets,
	// we decrement offsets[i] until we go into state 1.
	offsets := make([]int, m)
	for i := 0; i < m; i++ {
		offsets[i] = offset
	}

	// counts[i] is the number of elements that are now covered. While in
	// state 1, we keep decrementing counts[i] by size of intersection. We go
	// into state 2 when counts[i] <= 0.
	// If count == 0, we ignore counts and enter 2 only after intersecting with
	// all buckets.
	var counts []int
	if count > 0 {
		counts = make([]int, m)
		for i := 0; i < m; i++ {
			counts[i] = count
		}
	}

	var done bool
	for token := kt.GetFirst(); !done && len(token) > 0; token = kt.GetNext(token) {
		if len(token) == 0 {
			break
		}
		key := types.IndexKey(attr, token)
		pl, decr := posting.GetOrCreate(key, ws.dataStore)
		x.Assertf(pl != nil, "%s %s", attr, token)

		// If some UID list is not done, "done" will be set to false.
		done = true
		for i := 0; i < m; i++ { // Iterate over UID lists.
			var ul task.UidList
			var l algo.UIDList
			x.Assert(s.Uidmatrix(&ul, i))
			l.FromTask(&ul)
			listOpt := posting.ListOptions{Intersect: &l}
			// Intersect index with i-th input UID list.
			result := pl.Uids(listOpt)
			n := result.Size()
			x.Printf("~~~Intersect token=[%s] i=%d sizeOfResult=%d", token, i, n)

			wantBucket := state[i] < 2
			if state[i] == 0 {
				if offsets[i] >= n {
					// We do not need this bucket. Let's drop it.
					offsets[i] -= n
					wantBucket = false
				} else {
					// This is the first bucket we need. Enter new state. Update count.
					x.Printf("~~~## i=%d n=%d offsets[i]=%d", i, n, offsets[i])
					state[i] = 1
					if count > 0 {
						// Say intersection is 100 elements. Offset is 3. So, we're
						// keeping 97 elements from this bucket. Decrement count by 97.
						counts[i] -= (n - offsets[i])
						if counts[i] <= 0 {
							state[i] = 2
						}
					}
				}
			} else if state[i] == 1 {
				if count > 0 {
					counts[i] -= n
					if counts[i] <= 0 {
						state[i] = 2
					}
				}
			}

			if wantBucket {
				for j := 0; j < result.Size(); j++ {
					out[i] = append(out[i], result.Get(j))
				}
			}

			x.Printf("~~~EndOfIter: token=[%s] state[%d]=%d", token, i, state[i])
			if state[i] != 2 {
				// There is a UID list that is not done. So we need to continue
				// iterating over buckets.
				done = false
			}
		}
		decr() // Done with this posting list.
	}

	x.Printf("~~~~sort out:%v", out)

	// Convert out to flatbuffers output.
	b := flatbuffers.NewBuilder(0)

	// Add UID matrix.
	uidOffsets := make([]flatbuffers.UOffsetT, 0, m)
	for _, ul := range out {
		var l algo.UIDList
		l.FromUints(ul)
		uidOffsets = append(uidOffsets, l.AddTo(b))
	}

	task.SortResultStartUidmatrixVector(b, m)
	for i := m - 1; i >= 0; i-- {
		b.PrependUOffsetT(uidOffsets[i])
	}
	uend := b.EndVector(m)

	task.SortResultStartOffsetVector(b, m)
	for i := m - 1; i >= 0; i-- {
		b.PrependInt32(int32(offsets[i]))
	}
	offsetOffset := b.EndVector(m)

	task.SortResultStart(b)
	task.SortResultAddUidmatrix(b, uend)
	task.SortResultAddOffset(b, offsetOffset)
	b.Finish(task.SortResultEnd(b))
	return b.FinishedBytes(), nil
}
