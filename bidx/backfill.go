package bidx

import (
	"log"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/store"
)

// Backfill simply adds stuff from posting list into index.
func (s *Indices) Backfill(ps *store.Store) error {
	for _, index := range s.Index {
		go index.Backfill(ps, s.Done)
	}
	for i := 0; i < len(s.Index); i++ {
		if err := <-s.Done; err != nil {
			return err
		}
	}
	return nil
}

func (s *Index) Backfill(ps *store.Store, done chan error) {
	log.Printf("Backfilling attribute: %s\n", s.Config.Attribute)
	for i := 0; i < s.Config.NumShards; i++ {
		go s.Shard[i].Backfill(ps, s.Done)
	}

	it := ps.GetIterator()
	defer it.Close()
	prefix := s.Config.Attribute + "|"
	for it.Seek([]byte(prefix)); it.Valid(); it.Next() {
		uid, attr := posting.DecodeKey(it.Key().Data())
		if attr != s.Config.Attribute {
			// Keys are of the form attr|uid and sorted. Once we hit a attr that is
			// wrong, we are done.
			break
		}

		whichShard := uid % uint64(s.Config.NumShards)
		pl := types.GetRootAsPostingList(it.Value().Data(), 0)
		var p types.Posting
		for i := 0; i < pl.PostingsLength(); i++ {
			if !pl.Postings(&p, i) {
				log.Fatalf("Unable to get posting: %d %s", uid, attr)
			}

			if p.ValueLength() == 0 {
				continue
			}
			value := string(p.ValueBytes())
			s.Shard[whichShard].JobQueue <- indexJob{
				op:    jobOpAdd,
				uid:   uid,
				value: value,
			}
		}
	}

	for i := 0; i < s.Config.NumShards; i++ {
		close(s.Shard[i].JobQueue)
	}
	for i := 0; i < s.Config.NumShards; i++ {
		if err := <-s.Done; err != nil {
			done <- err // Some shard failed. Inform our parent and return.
			return
		}
	}
	done <- nil
}

func (s *IndexShard) Backfill(ps *store.Store, done chan error) {
	var count uint64
	for job := range s.JobQueue {
		if job.op == jobOpAdd {
			s.Batch.Index(string(posting.UID(job.uid)), job.value)
		} else {
			log.Fatalf("Unknown job operation for backfill: %d", job.op)
		}
		if s.Batch.Size() >= batchSize {
			newCount := count + uint64(s.Batch.Size())
			log.Printf("Attr[%s] shard %d batch[%d, %d]\n",
				s.Config.Attribute, s.Shard, count, newCount)
			s.Bindex.Batch(s.Batch)
			s.Batch.Reset()
			count = newCount
		}
	}
	s.Bindex.Batch(s.Batch)
	s.Batch.Reset()
	done <- nil
}
