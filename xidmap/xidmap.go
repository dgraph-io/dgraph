package xidmap

import (
	"container/list"
	"encoding/binary"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
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
	shards []shard
	kv     *badger.KV
	up     UidProvider
	opt    Options

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

func (b *block) assign(up UidProvider) (uint64, error) {
	if b.end == 0 || b.start > b.end {
		start, end, err := up.ReserveUidRange()
		if err != nil {
			return 0, err
		}
		b.start, b.end = start, end
	}
	x.AssertTrue(b.start <= b.end)
	uid := b.start
	b.start++
	return uid, nil
}

// UidProvider allows the XidMap to obtain ranges of uids that it can then
// allocate freely. Implementations should expect to be called concurrently.
type UidProvider interface {
	// ReserveUidRange should give a range of new uids from start to end
	// (start and end are both inclusive).
	ReserveUidRange() (start, end uint64, err error)
}

// New creates an XidMap with given badger and uid provider.
func New(kv *badger.KV, up UidProvider, opt Options) *XidMap {
	x.AssertTrue(opt.LRUSize != 0)
	x.AssertTrue(opt.NumShards != 0)
	xm := &XidMap{
		shards: make([]shard, opt.NumShards),
		kv:     kv,
		up:     up,
		opt:    opt,
	}
	for i := range xm.shards {
		xm.shards[i].elems = make(map[string]*list.Element)
		xm.shards[i].queue = list.New()
		xm.shards[i].xm = xm
	}
	return xm
}

// AssignUid creates new or looks up existing XID to UID mappings. Any errors
// returned originate from the UidProvider.
func (m *XidMap) AssignUid(xid string) (uid uint64, isNew bool, err error) {
	fp := farm.Fingerprint64([]byte(xid))
	idx := fp % uint64(m.opt.NumShards)
	sh := &m.shards[idx]

	sh.Lock()
	defer sh.Unlock()

	var ok bool
	uid, ok = sh.lookup(xid)
	if ok {
		return uid, false, nil
	}

	var item badger.KVItem
	x.Check(m.kv.Get([]byte(xid), &item))
	x.Check(item.Value(func(uidBuf []byte) error {
		if uidBuf == nil {
			return nil
		}
		var n int
		uid, n = binary.Uvarint(uidBuf)
		x.AssertTrue(n == len(uidBuf))
		ok = true
		return nil
	}))
	if ok {
		sh.add(xid, uid, true)
		return uid, false, nil
	}

	uid, err = sh.assign(sh.xm.up)
	sh.add(xid, uid, false)
	return uid, true, err
}

// AllocateUid gives a single uid without creating an xid to uid mapping.
func (m *XidMap) AllocateUid() (uint64, error) {
	m.noMapMu.Lock()
	defer m.noMapMu.Unlock()
	return m.noMap.assign(m.up)
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
	batch := make([]*badger.Entry, 0, evict)
	for i := 0; i < evict; i++ {
		m := s.queue.Remove(s.queue.Front()).(*mapping)
		delete(s.elems, m.xid)
		s.beingEvicted[m.xid] = m.uid
		if !m.persisted {
			var uidBuf [binary.MaxVarintLen64]byte
			n := binary.PutUvarint(uidBuf[:], m.uid)
			batch = append(batch, &badger.Entry{
				Key:   []byte(m.xid),
				Value: uidBuf[:n],
			})
		}

	}
	s.xm.kv.BatchSetAsync(batch, func(err error) {
		x.Check(err)
		for _, e := range batch {
			x.Check(e.Error)
		}

		s.Lock()
		s.beingEvicted = nil
		s.Unlock()
	})
}
