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

func processSort(qu []byte) ([]byte, error) {
	s := task.GetRootAsSort(qu, 0)
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

	for token := kt.GetFirst(); len(token) > 0; token = kt.GetNext(token) {
		if len(token) == 0 {
			break
		}
		key := types.IndexKey(attr, token)
		pl, decr := posting.GetOrCreate(key, ws.dataStore)
		x.Assertf(pl != nil, "%s %s", attr, token)

		// Iterate over UID lists.
		for i := 0; i < m; i++ {
			var ul task.UidList
			var l algo.UIDList
			x.Assert(s.Uidmatrix(&ul, i))
			l.FromTask(&ul)
			listOpt := posting.ListOptions{
				Intersect: &l,
			}
			// Intersect index with i-th input UID list.
			result := pl.Uids(listOpt)
			x.Printf("~~~Intersect token=[%s] i=%d sizeOfResult=%d", token, i, result.Size())
			// Append result.
			for j := 0; j < result.Size(); j++ {
				out[i] = append(out[i], result.Get(j))
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

	task.SortResultStart(b)
	task.SortResultAddUidmatrix(b, uend)
	b.Finish(task.SortResultEnd(b))
	return b.FinishedBytes(), nil
}
