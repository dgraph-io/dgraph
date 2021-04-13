package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type taskStatus []byte

var (
	taskStatusQueued   = taskStatus("Queued")
	taskStatusRunning  = taskStatus("Running")
	taskStatusCanceled = taskStatus("Canceled")
	taskStatusError    = taskStatus("Error")
	taskStatusSuccess  = taskStatus("Success")

	pendingTasks  = newTaskQueue()
	taskKeyPrefix = []byte("!dgraphTask!")
)

func InitTaskQueue() {
	if err := cancelQueuedTasks(); err != nil {
		glog.Errorf("tasks: failed to cancel unfinished tasks: %v", err)
	}
	go pendingTasks.run()
}

// cancelQueuedTasks marks all queued tasks in Badger as canceled.
func cancelQueuedTasks() error {
	txn := State.Pstore.NewTransactionAt(math.MaxUint64, true)
	defer txn.Discard()

	opts := badger.DefaultIteratorOptions
	opts.Prefix = taskKeyPrefix

	it := txn.NewIterator(opts)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		key := item.Key()

		isQueued := false
		if err := item.Value(func(status []byte) error {
			isQueued = bytes.Equal(status, taskStatusQueued)
			return nil
		}); err != nil {
			return err
		}

		if isQueued {
			entry := newTaskEntry(key, taskStatusCanceled)
			if err := txn.SetEntry(entry); err != nil {
				return err
			}
		}
	}

	return txn.CommitAt(math.MaxUint64, nil)
}

type taskQueue chan task

func newTaskQueue() taskQueue {
	const maxQueueSize = 16
	return make(chan task, maxQueueSize)
}

// run loops forever, running queued tasks one at a time. Any returned errors are logged.
func (q taskQueue) run() {
	for {
		// Fetch a task from the queue. If the server is shutting down, break out of the loop.
		var task task
		select {
		case <-x.ServerCloser.HasBeenClosed():
			break
		case task = <-q:
		}
		key := task.id.getKey()

		if err := func() error {
			txn := State.Pstore.NewTransactionAt(math.MaxUint64, true)
			defer txn.Discard()

			// Fetch task from Badger. If the task isn't found, this means it has expired (older
			// than taskTTL).
			item, err := txn.Get(key)
			if err != nil {
				if err == badger.ErrKeyNotFound {
					return fmt.Errorf("task is expired, skipping")
				}
				return err
			}

			// Only proceed if the task is still queued. It's possible that the task got canceled
			// before we were able to run it.
			if err := item.Value(func(status []byte) error {
				if !bytes.Equal(status, taskStatusQueued) {
					return fmt.Errorf("status is set to %s, skipping", status)
				}
				return nil
			}); err != nil {
				return err
			}

			// Change the task status to "running".
			entry := newTaskEntry(key, taskStatusRunning)
			if err := txn.SetEntry(entry); err != nil {
				return err
			}
			return txn.CommitAt(math.MaxUint64, nil)

		}(); err != nil {
			glog.Errorf("task %d: error: %v", task.id, err)
			continue
		}

		// Run the task.
		var status taskStatus
		if err := task.run(); err != nil {
			glog.Errorf("task %d: error: %v", task.id, err)
			status = taskStatusError
		} else {
			glog.Infof("task %d: complete", task.id)
			status = taskStatusSuccess
		}

		// Change the task status to "complete" / "error".
		entry := newTaskEntry(key, status)
		if err := func() error {
			txn := State.Pstore.NewTransactionAt(math.MaxUint64, true)
			defer txn.Discard()
			if err := txn.SetEntry(entry); err != nil {
				return err
			}
			return txn.CommitAt(math.MaxUint64, nil)
		}(); err != nil {
			glog.Errorf("task %d: error: could not save status: %v", task.id, err)
		}
	}
}

func (q taskQueue) queueBackup(req *pb.BackupRequest) (taskId, error) {
	return q._queueTask(req)
}

func (q taskQueue) queueExport(req *pb.ExportRequest) (taskId, error) {
	return q._queueTask(req)
}

// _queueTask queues a task of any type. Don't use this function directly.
func (q taskQueue) _queueTask(req interface{}) (taskId, error) {
	task := task{newTaskId(), req}
	select {
	case q <- task:
		key := task.id.getKey()
		entry := newTaskEntry(key, taskStatusQueued)
		if err := func() error {
			txn := State.Pstore.NewTransactionAt(math.MaxUint64, true)
			defer txn.Discard()
			if err := txn.SetEntry(entry); err != nil {
				return err
			}
			return txn.CommitAt(math.MaxUint64, nil)
		}(); err != nil {
			return 0, errors.Wrapf(err, "could not save task status")
		}
		return task.id, nil
	default:
		return 0, fmt.Errorf("too many pending tasks, please try again later")
	}
}

type task struct {
	id  taskId
	req interface{} // *pb.BackupRequest, *pb.ExportRequest
}

// run starts a task and blocks till it completes.
func (t task) run() error {
	switch req := t.req.(type) {
	case *pb.BackupRequest:
		// TODO(ajeet): shouldn't forceFull be part of pb.BackupRequest?
		if err := ProcessBackupRequest(context.Background(), req, true); err != nil {
			return err
		}
	case *pb.ExportRequest:
		files, err := ExportOverNetwork(context.Background(), req)
		if err != nil {
			return err
		}
		glog.Infof("task %d: exported files: %v", t.id, files)
	default:
		err := fmt.Errorf(
			"a request of unknown type (%T) was queued", reflect.TypeOf(t.req))
		panic(err)
	}

	return nil
}

type taskId int64

// newTaskId returns a unique, random, unguessable, non-negative int64.
//
// The format of this is (from MSB to LSB):
// bit [0,1):   always 0
// bit [1,33):  32-bit UNIX timestamp (in seconds)
// bit [33,64): 31-bit cryptographically secure random number
func newTaskId() taskId {
	// Overflows on 2106-02-07
	now := int64(time.Now().Unix()) & math.MaxUint32
	rnd, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt32))
	if err != nil {
		panic(err)
	}
	return taskId(now<<31 | rnd.Int64())
}

// getKey generates a Badger key for the given taskId.
//
// The format of this is:
// byte [0, l):   taskKeyPrefix
// byte [l, l+8]: taskId
func (id taskId) getKey() []byte {
	l := len(taskKeyPrefix)
	key := make([]byte, l+8)
	x.AssertTrue(copy(key[0:l], []byte(taskKeyPrefix)) == l)
	binary.LittleEndian.PutUint64(key[l:l+8], uint64(id))
	return key
}

func newTaskEntry(key []byte, status taskStatus) *badger.Entry {
	const ttl = 7 * 24 * time.Hour // 1 week
	return badger.NewEntry(key, status).WithDiscard().WithTTL(ttl)
}
