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
	batchSize        = 10000
	backfillBufSize  = 20000
	frontfillBufSize = 200
)

type mutation struct {
	remove bool   // If false, this is a "insert with overwrite" operation.
	attr   string // The predicate. Can be left empty if obvious in code.
	uid    uint64 // The subject.
	value  string // If remove=true, this field is ignored.
}

// Indices holds predicate indices and is the core object of the package.
type Indices struct {
	dir        string
	cfg        *Configs
	indexer    indexer.Indexer    // Indexer is expected to be a pointer.
	pred       map[string]*Config // Maps predicate to its config.
	frontfillC chan *mutation
}

// CreateIndices takes a given config and prepare a new empty indices directory.
// One thing it does is to copy the given config to the new directory. However,
// the copy at the new directory is supposed to be internal and the user should
// not meddle with it.
func CreateIndices(cfg *Configs, dir string) (*Indices, error) {
	x.Check(os.MkdirAll(dir, 0700))
	cfg.write(dir)
	indexer := indexer.New(cfg.Indexer)
	if err := indexer.Create(dir); err != nil {
		return nil, err
	}
	return initIndices(cfg, dir, indexer)
}

// NewIndices constructs Indices from dir which contains a config file.
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
	pred := make(map[string]*Config)
	for _, c := range cfg.Cfg { // For each predicate.
		pred[c.Attr] = c
	}
	indices := &Indices{
		dir:        dir,
		cfg:        cfg,
		indexer:    indexer,
		pred:       pred,
		frontfillC: make(chan *mutation, frontfillBufSize),
	}
	// Goroutine consumes jobs from frontfillC.
	go indices.processFrontfillC()
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
	x.Assertf(s.frontfillC != nil)
	s.frontfillC <- m
	return nil
}

func (s *Indices) processFrontfillC() {
	for m := range s.frontfillC {
		buf, err := x.EncodeUint64Ordered(m.uid)
		x.Check(err)
		if !m.remove {
			s.indexer.Insert(m.attr, string(buf), m.value)
		} else {
			s.indexer.Remove(m.attr, string(buf))
		}
	}
}

///////////////////////////// BACKFILL /////////////////////////////
// Backfill simply adds stuff from posting list into index.
func (s *Indices) Backfill(ctx context.Context, ps *store.Store) error {
	errC := make(chan error)
	for _, cfg := range s.pred {
		go s.doBackfill(ctx, cfg, ps, errC) // One goroutine per predicate.
	}
	for i := 0; i < len(s.pred); i++ {
		if err := <-errC; err != nil {
			return err
		}
	}
	return nil
}

// doBackfill takes care of backfill for a single predicate. When we are done, or
// if there is any error, just push to errC.
func (s *Indices) doBackfill(ctx context.Context, cfg *Config, ps *store.Store, errC chan error) {
	x.Trace(ctx, "Backfilling attribute: %s", cfg.Attr)
	// We do not add backfillC or some other vars here to Indices object because
	// they are internal to backfilling and do not need to persist outside of the
	// backfill task.
	backfillC := make(chan *mutation, backfillBufSize)
	go s.backfillBatch(ctx, cfg, backfillC, errC)

	it := ps.NewIterator()
	defer it.Close()
	prefix := []byte(cfg.Attr + "|")
	attr := []byte(cfg.Attr)

	for it.Seek(prefix); it.ValidForPrefix(attr); it.Next() {
		uid, _ := posting.DecodeKey(it.Key().Data())
		pl := types.GetRootAsPostingList(it.Value().Data(), 0)
		var p types.Posting
		for i := 0; i < pl.PostingsLength(); i++ {
			x.Assertf(pl.Postings(&p, i), "Fail to get posting %d %s", uid, cfg.Attr)
			if p.ValueLength() > 0 {
				// There is no need to populate "attr" because it is clear what it is.
				backfillC <- &mutation{
					uid:   uid,
					value: string(p.ValueBytes()),
				}
			}
		}
	}
	close(backfillC)
}

func (s *Indices) backfillBatch(ctx context.Context, cfg *Config, backfillC chan *mutation, errC chan error) {
	batch, err := s.indexer.NewBatch()
	if err != nil {
		errC <- err
		return
	}
	var count uint64
	for job := range backfillC {
		if !job.remove {
			buf, err := x.EncodeUint64Ordered(job.uid)
			x.Check(err)
			batch.Insert(cfg.Attr, string(buf), job.value)
		} else {
			errC <- x.Errorf("Backfill cannot remove %s %d", job.attr, job.uid)
			return
		}
		if batch.Size() >= batchSize {
			if err := s.doIndex(ctx, cfg, batch, &count); err != nil {
				errC <- err
				return
			}
		}
	}
	errC <- s.doIndex(ctx, cfg, batch, &count)
}

// doIndex apply the batch. count is incremented by batch size. Returns any error.
func (s *Indices) doIndex(ctx context.Context, cfg *Config, batch indexer.Batch, count *uint64) error {
	if batch.Size() == 0 {
		return nil
	}
	newCount := *count + uint64(batch.Size())
	x.Trace(ctx, "Attr[%s] batch[%d, %d]\n", cfg.Attr, count, newCount)
	err := x.Wrap(s.indexer.Batch(batch))
	batch.Reset()
	*count = newCount
	return err
}
