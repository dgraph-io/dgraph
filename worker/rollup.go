package worker

import (
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"golang.org/x/net/context"
)

// We use the same Send interface, but instead write back to DB.
type writeToStore struct{}

func (ws *writeToStore) Send(kvs *intern.KVS) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	var count int
	for _, kv := range kvs.GetKv() {
		if kv.Version == 0 {
			continue
		}
		txn := pstore.NewTransactionAt(kv.Version, true)
		x.Check(txn.SetWithDiscard(kv.Key, kv.Val, kv.UserMeta[0]))
		wg.Add(1)
		err := txn.CommitAt(kv.Version, func(err error) {
			defer wg.Done()
			if err != nil {
				x.Printf("Error while writing list to Badger: %v\n", err)
				select {
				case errCh <- err:
				default:
				}
			}
		})
		if err != nil {
			return err
		}
		count++
	}
	wg.Wait()
	x.Printf("During snapshot, wrote %d keys to disk\n", count)
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func rollupPostingLists(snapshotTs uint64) error {
	var ws writeToStore
	sl := streamLists{stream: &ws, db: pstore}
	sl.chooseKey = func(_ []byte, _ uint64, userMeta byte) bool {
		return userMeta&posting.BitDeltaPosting > 0
	}

	// We own the key byte slice provided.
	sl.itemToKv = func(key []byte, itr *badger.Iterator) (*intern.KV, error) {
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, err
		}
		pl, err := l.Rollup()
		if err != nil {
			return nil, err
		}
		// TODO: We can update the in-memory version of posting list here.
		data, meta := posting.MarshalPostingList(pl)
		kv := &intern.KV{
			Key:      key,
			Val:      data,
			Version:  pl.Commit,
			UserMeta: []byte{meta},
		}
		return kv, nil
	}

	return sl.orchestrate(context.Background(), "", snapshotTs)
}
