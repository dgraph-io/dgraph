package xidmap

import (
	"container/list"
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
	"google.golang.org/grpc"
)

// Options controls the performance characteristics of the XidMap.
type Options struct {
	// NumShards controls the number of shards the XidMap is broken into. More
	// shards reduces lock contention.
	NumShards int
	// LRUSize controls the total size of the LRU cache. The LRU is split
	// between all shards, so with 4 shards and an LRUSize of 100, each shard
	// receives 25 LRU slots.
	LRUSize int
}

// XidMap allocates and tracks mappings between Xids and Uids in a threadsafe
// manner. It's memory friendly because the mapping is stored on disk, but fast
// because it uses an LRU cache.
type XidMap struct {
	shards    []shard
	kv        *badger.DB
	opt       Options
	newRanges chan rangeResponse

	noMapMu sync.Mutex
	noMap   block // block for allocating uids without an xid to uid mapping
}

type shard struct {
	sync.Mutex
	block

	elems        map[string]*list.Element
	queue        *list.List
	beingEvicted map[string]uint64

	xm *XidMap
}

type mapping struct {
	xid       string
	uid       uint64
	persisted bool
}

type block struct {
	start, end uint64
}

func (b *block) assign(ch <-chan rangeResponse) uint64 {
	if b.end == 0 || b.start > b.end {
		newRange := <-ch
		b.start, b.end = newRange.start, newRange.end
	}
	x.AssertTrue(b.start <= b.end)
	uid := b.start
	b.start++
	return uid
}

type rangeResponse struct {
	start uint64
	end   uint64
}

// New creates an XidMap with given badger and uid provider.
func New(kv *badger.DB, zero *grpc.ClientConn, opt Options) *XidMap {
	x.AssertTrue(opt.LRUSize != 0)
	x.AssertTrue(opt.NumShards != 0)
	xm := &XidMap{
		shards:    make([]shard, opt.NumShards),
		kv:        kv,
		opt:       opt,
		newRanges: make(chan rangeResponse),
	}
	for i := range xm.shards {
		xm.shards[i].elems = make(map[string]*list.Element)
		xm.shards[i].queue = list.New()
		xm.shards[i].xm = xm
	}
	go func() {
		zc := protos.NewZeroClient(zero)
		const initBackoff = 10 * time.Millisecond
		const maxBackoff = 30 * time.Second
		backoff := initBackoff
		for {
			assigned, err := zc.AssignUids(context.Background(), &protos.Num{Val: 10000})
			if err != nil {
				x.Printf("Error while getting lease: %v\n", err)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				time.Sleep(backoff)
			} else {
				backoff = initBackoff
				xm.newRanges <- rangeResponse{
					start: assigned.GetStartId(),
					end:   assigned.GetEndId(),
				}
			}
		}
	}()
	return xm
}

// AssignUid creates new or looks up existing XID to UID mappings.
func (m *XidMap) AssignUid(xid string) (uid uint64, isNew bool) {
	fp := farm.Fingerprint64([]byte(xid))
	idx := fp % uint64(m.opt.NumShards)
	sh := &m.shards[idx]

	sh.Lock()
	defer sh.Unlock()

	var ok bool
	uid, ok = sh.lookup(xid)
	if ok {
		return uid, false
	}

	x.Check(m.kv.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(xid))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		x.Check(err)
		uidBuf, err := item.Value()
		x.Check(err)
		x.AssertTrue(len(uidBuf) > 0)
		var n int
		uid, n = binary.Uvarint(uidBuf)
		x.AssertTrue(n == len(uidBuf))
		ok = true
		return nil
	}))
	if ok {
		sh.add(xid, uid, true)
		return uid, false
	}

	uid = sh.assign(m.newRanges)
	sh.add(xid, uid, false)
	return uid, true
}

// AllocateUid gives a single uid without creating an xid to uid mapping.
func (m *XidMap) AllocateUid() uint64 {
	m.noMapMu.Lock()
	defer m.noMapMu.Unlock()
	return m.noMap.assign(m.newRanges)
}

func (s *shard) lookup(xid string) (uint64, bool) {
	elem, ok := s.elems[xid]
	if ok {
		s.queue.MoveToBack(elem)
		return elem.Value.(*mapping).uid, true
	}
	if uid, ok := s.beingEvicted[xid]; ok {
		s.add(xid, uid, true)
		return uid, true
	}
	return 0, false
}

func (s *shard) add(xid string, uid uint64, persisted bool) {
	lruSizePerShard := s.xm.opt.LRUSize / s.xm.opt.NumShards
	if s.queue.Len() >= lruSizePerShard && len(s.beingEvicted) == 0 {
		s.evict()
	}

	m := &mapping{
		xid:       xid,
		uid:       uid,
		persisted: persisted,
	}
	elem := s.queue.PushBack(m)
	s.elems[xid] = elem
}

func (s *shard) evict() {
	const evictRatio = 0.5
	evict := int(float64(s.queue.Len()) * evictRatio)
	s.beingEvicted = make(map[string]uint64)
	txn := s.xm.kv.NewTransaction(true)
	defer txn.Discard()
	for i := 0; i < evict; i++ {
		m := s.queue.Remove(s.queue.Front()).(*mapping)
		delete(s.elems, m.xid)
		s.beingEvicted[m.xid] = m.uid
		if !m.persisted {
			var uidBuf [binary.MaxVarintLen64]byte
			n := binary.PutUvarint(uidBuf[:], m.uid)
			txn.Set([]byte(m.xid), uidBuf[:n])
		}

	}
	txn.Commit(func(err error) {
		x.Check(err)

		s.Lock()
		s.beingEvicted = nil
		s.Unlock()
	})
}
