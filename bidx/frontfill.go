package bidx

import (
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

type mutation struct {
	Delete bool
	Attr   string
	UID    uint64
	Value  string
}

func NewFrontfillAdd(attr string, uid uint64, val string) *mutation {
	return &mutation{
		Attr:  attr,
		UID:   uid,
		Value: val,
	}
}

func NewFrontfillDel(attr string, uid uint64) *mutation {
	return &mutation{
		Delete: true,
		Attr:   attr,
		UID:    uid,
	}
}

// FrontfillAdd inserts with overwrite (replace) key, value into our indices.
func FrontfillAdd(attr string, uid uint64, val string) {
	globalIndices.Frontfill(NewFrontfillAdd(attr, uid, val))
}

// FrontfillDel deletes a key, value from our indices.
func FrontfillDel(attr string, uid uint64) {
	globalIndices.Frontfill(NewFrontfillDel(attr, uid))
}

func (s *Indices) Frontfill(m *mutation) error {
	index, found := s.index[m.Attr]
	if !found {
		return nil // This predicate is not indexed, which can be common.
	}
	return index.frontfill(m)
}

func (s *Index) frontfill(m *mutation) error {
	whichShard := m.UID % uint64(s.config.NumShards)
	return s.shard[whichShard].frontfill(m)
}

func (s *IndexShard) frontfill(m *mutation) error {
	if s.mutationC == nil {
		return x.Errorf("mutationC nil for %s %d", s.config.Attribute, s.shard)
	}
	s.mutationC <- m
	return nil
}

func (s *Indices) initFrontfill() {
	for _, index := range s.index {
		index.initFrontfill()
	}
}

func (s *Index) initFrontfill() {
	for _, shard := range s.shard {
		shard.initFrontfill()
	}
}

func (s *IndexShard) initFrontfill() {
	s.mutationC = make(chan *mutation, 100)
	go s.handleFrontfill()
}

func (s *IndexShard) handleFrontfill() {
	for m := range s.mutationC {
		s.bindexLock.Lock()
		if !m.Delete {
			s.bindex.Index(string(posting.UID(m.UID)), m.Value)
		} else {
			s.bindex.Delete(string(posting.UID(m.UID)))
		}
		s.bindexLock.Unlock()
	}
}
