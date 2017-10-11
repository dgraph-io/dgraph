package xidmap

import (
	"container/list"
	"encoding/binary"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

type Options struct {
	NumShards int
	LRUSize   int
}

type XidMap struct {
	shards []shard
	kv     *badger.KV
	up     UidProvider
	opt    Options

	// For one-off uid requests.
	lastUsed, lease uint64
}

type shard struct {
	sync.Mutex

	lastUsed uint64
	lease    uint64

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

type UidProvider interface {
	ReserveUidRange(size uint64) (start, end uint64, err error)
}

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

// AssignUid creates new or looks up existing XID to UID mappings.
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

	if sh.lastUsed == sh.lease {
		start, end, err := m.up.ReserveUidRange(1e5)
		if err != nil {
			return 0, false, err
		}
		sh.lastUsed, sh.lease = start, end
	}
	sh.lastUsed++
	sh.add(xid, sh.lastUsed, false)
	return sh.lastUsed, true, nil
}

func (m *XidMap) ReserveUid() (uint64, error) {
	if m.lastUsed == m.lease {
		start, end, err := m.up.ReserveUidRange(1e5)
		if err != nil {
			return 0, err
		}
		m.lastUsed, m.lease = start, end
	}
	m.lastUsed++
	return m.lastUsed, nil
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
