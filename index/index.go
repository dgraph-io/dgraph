/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package index

import (
	"bufio"
	"context"
	"os"

	"github.com/dgraph-io/dgraph/index/indexer"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// We try to batch our updates so that they are more efficient.
	batchSize = 10000

	backfillBufSize  = 20000
	frontfillBufSize = 200
)

type mutation struct {
	remove bool   // If false, this is a "insert with overwrite" operation.
	attr   string // The predicate. Can be left empty if obvious in code.
	uid    uint64 // The subject.
	value  string // If del, this field is ignored.
}

// Indices holds predicate indices and is the core object of the package.
type Indices struct {
	dir        string
	cfg        *Configs
	indexer    indexer.Indexer       // Indexer is expected to be a pointer.
	idx        map[string]*predIndex // Maps predicate / attribute to index.
	frontfillC chan *mutation
}

// index is index for one predicate.
type predIndex struct {
	indexer indexer.Indexer
	cfg     *Config
}

// CreateIndices creates new empty dirs given config file and basedir.
func CreateIndices(cfg *Configs, dir string) (*Indices, error) {
	x.Check(os.MkdirAll(dir, 0700))
	cfg.write(dir)
	indexer := indexer.New(cfg.Indexer)
	if err := indexer.Create(dir); err != nil {
		return nil, err
	}
	return initIndices(cfg, dir, indexer)
}

// NewIndices constructs Indices from basedir which contains a config file.
func NewIndices(dir string) (*Indices, error) {
	fin, err := os.Open(getDefaultConfig(dir))
	if err != nil {
		return nil, err
	}
	defer fin.Close()
	cfg, err := ReadConfigs(bufio.NewReader(fin))
	if err != nil {
		return nil, err
	}
	indexer := indexer.New(cfg.Indexer)
	err = indexer.Open(dir)
	if err != nil {
		return nil, err
	}
	return initIndices(cfg, dir, indexer)
}

func initIndices(cfg *Configs, dir string, indexer indexer.Indexer) (*Indices, error) {
	idx := make(map[string]*predIndex)
	indices := &Indices{
		dir:        dir,
		cfg:        cfg,
		indexer:    indexer,
		idx:        idx,
		frontfillC: make(chan *mutation, frontfillBufSize),
	}

	go indices.processFrontfillC() // This func consumes jobs from frontfillC.

	for _, c := range cfg.Cfg { // For each predicate.
		idx := &predIndex{
			indexer: indexer,
			cfg:     c,
		}
		indices.idx[c.Attr] = idx
	}
	return indices, nil
}

///////////////////////////// FRONTFILL /////////////////////////////
// FrontfillAdd inserts with overwrite (replace) key, value into our indices.
func (s *Indices) FrontfillAdd(ctx context.Context, attr string, uid uint64, val string) error {
	return s.frontfill(ctx, &mutation{false, attr, uid, val})
}

// FrontfillDel deletes a key, value from our indices.
func (s *Indices) FrontfillDel(ctx context.Context, attr string, uid uint64) error {
	return s.frontfill(ctx, &mutation{true, attr, uid, ""})
}

// Frontfill updates indices given mutation.
func (s *Indices) frontfill(ctx context.Context, m *mutation) error {
	if s.frontfillC == nil {
		return x.Errorf("Channel nil")
	}
	s.frontfillC <- m
	return nil
}

func (s *Indices) processFrontfillC() {
	for m := range s.frontfillC {
		uid := string(posting.UID(m.uid))
		if !m.remove {
			s.indexer.Insert(m.attr, uid, m.value)
		} else {
			s.indexer.Remove(m.attr, uid)
		}
	}
}

///////////////////////////// BACKFILL /////////////////////////////
// Backfill simply adds stuff from posting list into index.
func (s *Indices) Backfill(ctx context.Context, ps *store.Store) error {
	errC := make(chan error)
	for _, idx := range s.idx { // For each predicate.
		go idx.backfill(ctx, ps, errC)
	}
	for i := 0; i < len(s.idx); i++ {
		if err := <-errC; err != nil {
			return err
		}
	}
	return nil
}

func (s *predIndex) backfill(ctx context.Context, ps *store.Store, errC chan error) {
	x.Trace(ctx, "Backfilling attribute: %s", s.cfg.Attr)
	backfillC := make(chan *mutation, backfillBufSize)
	go s.backfillBatch(ctx, backfillC, errC)

	it := ps.NewIterator()
	defer it.Close()
	prefix := []byte(s.cfg.Attr + "|")
	attr := []byte(s.cfg.Attr)

	for it.Seek(prefix); it.ValidForPrefix(attr); it.Next() {
		uid, _ := posting.DecodeKey(it.Key().Data())
		pl := types.GetRootAsPostingList(it.Value().Data(), 0)
		var p types.Posting
		for i := 0; i < pl.PostingsLength(); i++ {
			x.Assertf(pl.Postings(&p, i), "Fail to get posting %d %s", uid, s.cfg.Attr)
			if p.ValueLength() == 0 {
				continue
			}
			value := string(p.ValueBytes())
			backfillC <- &mutation{
				uid:   uid,
				value: value,
			}
		}
	}
	close(backfillC)
}

func (s *predIndex) backfillBatch(ctx context.Context, backfillC chan *mutation, errC chan error) {
	batch, err := s.indexer.NewBatch()
	if err != nil {
		errC <- err
		return
	}
	var count uint64
	for job := range backfillC {
		if !job.remove {
			batch.Insert(s.cfg.Attr, string(posting.UID(job.uid)), job.value)
		} else {
			errC <- x.Errorf("Backfill cannot remove %s %d", job.attr, job.uid)
			return
		}
		if batch.Size() >= batchSize {
			if err := s.doIndex(ctx, batch, &count); err != nil {
				errC <- err
				return
			}
		}
	}
	errC <- s.doIndex(ctx, batch, &count)
}

// doIndex apply the batch. count is incremented by batch size. Returns any error.
func (s *predIndex) doIndex(ctx context.Context, batch indexer.Batch, count *uint64) error {
	if batch.Size() == 0 {
		return nil
	}
	newCount := *count + uint64(batch.Size())
	x.Trace(ctx, "Attr[%s] batch[%d, %d]\n", s.cfg.Attr, count, newCount)
	err := x.Wrap(s.indexer.Batch(batch))
	batch.Reset()
	*count = newCount
	return err
}
