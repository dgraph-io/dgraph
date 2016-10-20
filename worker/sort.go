package worker

import (
	"time"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/algo"
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

func processSort(qu []byte) ([]byte, error) {
	s := task.GetRootAsSort(qu, 0)
	x.Assert(s != nil)
	attr := string(s.Attr())

	// Iterate over buckets.
	prefix := types.IndexKey(attr, "") // Do it the simple way first.

	posting.MergeLists(10)
	time.Sleep(time.Second)
	x.Printf("~~~~~ReceiveSort: attr=%s #uids=%d prefix=%s", attr, s.UidsLength(), prefix)
	//	for iter.Seek(prefix); iter.Valid(); iter.Next() {
	//		x.Printf("~~~~HIT prefix=%s", prefix)
	//	}

	iter := ws.dataStore.NewIterator()
	defer iter.Close()

	for iter.Seek(prefix); iter.ValidForPrefix(prefix); iter.Next() {
		x.Printf("~~~Key=%s", string(iter.Key().Data()))
	}

	var ul task.UidList
	x.Assert(s.Uids(&ul, 0))
	var l algo.UIDList
	l.FromTask(&ul)
	x.Printf("~~~~~ReceiveSortData: %s", l.DebugString())

	return nil, nil
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
