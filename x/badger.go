package x

import (
	"math"
	"sync"

	"github.com/dgraph-io/badger"
)

type TxnWriter struct {
	DB  *badger.DB
	wg  sync.WaitGroup
	che chan error
}

func (w *TxnWriter) cb(err error) {
	defer w.wg.Done()
	if err == nil {
		return
	}
	select {
	case w.che <- err:
	default:
	}
}

func (w *TxnWriter) SetAt(key, val []byte, meta byte, ts uint64) error {
	txn := w.DB.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()
	// TODO: We should probably do a Get to ensure that we don't end up
	// overwriting an already existing value at that ts, which might be there
	// due to a previous rollup event.
	if err := txn.SetWithMeta(key, val, meta); err != nil {
		return err
	}
	w.wg.Add(1)
	return txn.CommitAt(ts, w.cb)
}

func (w *TxnWriter) Flush() error {
	w.wg.Wait()
	select {
	case err := <-w.che:
		return err
	default:
		return nil
	}
}
