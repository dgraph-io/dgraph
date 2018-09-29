package x

import (
	"math"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/golang/glog"
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

	// We do a Get to ensure that we don't end up overwriting an already
	// existing delta or state at the ts.
	if item, err := txn.Get(key); err == badger.ErrKeyNotFound {
		// pass
	} else if err != nil {
		return err

	} else if item.Version() >= ts {
		// Found an existing commit at an equal or higher timestamp. So, skip writing.
		if glog.V(2) {
			pk := Parse(key)
			glog.Warningf("Existing >= Commit [%d >= %d]. Skipping write: %v",
				item.Version(), ts, pk)
		}
		return nil
	}
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
