package bidx

import (
	"log"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

// Backfill simply adds stuff from posting list into index.
func (s *Indices) Backfill(ps *store.Store) error {
	for _, index := range s.index {
		go index.backfill(ps, s.done)
	}
	for i := 0; i < len(s.index); i++ {
		if err := <-s.done; err != nil {
			return err
		}
	}
	return nil
}

func (s *Index) backfill(ps *store.Store, done chan error) {
	log.Printf("Backfilling attribute: %s\n", s.config.Attribute)
	for i := 0; i < s.config.NumShards; i++ {
		go s.shard[i].backfill(ps, s.done)
	}

	it := ps.GetIterator()
	defer it.Close()
	prefix := s.config.Attribute + "|"
	for it.Seek([]byte(prefix)); it.Valid(); it.Next() {
		uid, attr := posting.DecodeKey(it.Key().Data())
		if attr != s.config.Attribute {
			// Keys are of the form attr|uid and sorted. Once we hit a attr that is
			// wrong, we are done.
			break
		}

		whichShard := uid % uint64(s.config.NumShards)
		pl := types.GetRootAsPostingList(it.Value().Data(), 0)
		var p types.Posting
		for i := 0; i < pl.PostingsLength(); i++ {
			x.Assertf(pl.Postings(&p, i), "Unable to get posting: %d %s", uid, attr)
			if p.ValueLength() == 0 {
				continue
			}
			value := string(p.ValueBytes())
			s.shard[whichShard].jobQueue <- indexJob{
				op:    jobOpAdd,
				uid:   uid,
				value: value,
			}
		}
	}

	for i := 0; i < s.config.NumShards; i++ {
		close(s.shard[i].jobQueue)
	}
	for i := 0; i < s.config.NumShards; i++ {
		if err := <-s.done; err != nil {
			done <- err // Some shard failed. Inform our parent and return.
			return
		}
	}
	done <- nil
}

// Returns count incremented by batch size.
func (s *IndexShard) doIndex(count uint64) uint64 {
	if s.batch.Size() == 0 {
		return count
	}
	newCount := count + uint64(s.batch.Size())
	log.Printf("Attr[%s] shard %d batch[%d, %d]\n",
		s.config.Attribute, s.shard, count, newCount)
	s.bindex.Batch(s.batch)
	s.batch.Reset()
	return newCount
}

func (s *IndexShard) backfill(ps *store.Store, done chan error) {
	var count uint64
	for job := range s.jobQueue {
		if job.op == jobOpAdd {
			s.batch.Index(string(posting.UID(job.uid)), job.value)
		} else {
			log.Fatalf("Unknown job operation for backfill: %d", job.op)
		}
		if s.batch.Size() >= batchSize {
			count = s.doIndex(count)
		}
	}
	s.doIndex(count)
	done <- nil
}
