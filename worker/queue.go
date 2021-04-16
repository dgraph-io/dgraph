/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

var (
	Tasks tasks
)

func InitTasks() {
	// #nosec G404: weak RNG
	Tasks = tasks{
		queue:  make(chan taskRequest, 16),
		log:    z.NewTree(),
		logMu:  new(sync.Mutex),
		raftId: State.WALstore.Uint(raftwal.RaftId),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	Tasks.deleteExpired()
	Tasks.cancelQueued()
	go Tasks.run()
}

type tasks struct {
	// queue stores the full Protobuf request.
	queue chan taskRequest
	// log stores the timestamp, TaskKind, and TaskStatus.
	log   *z.Tree
	logMu *sync.Mutex

	raftId uint64
	rng    *rand.Rand
}

// Returns 0 if the task was not found.
func (t tasks) Get(id uint64) TaskMeta {
	if id == 0 || id == math.MaxUint64 {
		return 0
	}
	t.logMu.Lock()
	defer t.logMu.Unlock()
	return TaskMeta(t.log.Get(id))
}

func (t tasks) set(id uint64, meta TaskMeta) {
	t.logMu.Lock()
	defer t.logMu.Unlock()
	t.log.Set(id, uint64(meta))
}

// cancelQueuedTasks marks all queued tasks in the log as canceled.
func (t tasks) cancelQueued() {
	t.logMu.Lock()
	defer t.logMu.Unlock()

	t.log.IterateKV(func(id, val uint64) uint64 {
		meta := TaskMeta(val)
		if meta.Status() == TaskStatusQueued {
			return uint64(newTaskMeta(meta.Kind(), TaskStatusCanceled))
		}
		return 0
	})
}

// deleteExpired deletes all expired tasks.
// TODO(ajeet): figure out how to call this once a week.
func (t tasks) deleteExpired() {
	const ttl = 7 * 24 * time.Hour // 1 week
	minTs := time.Now().UTC().Add(-ttl).Unix()
	minMeta := uint64(minTs) << 32

	t.logMu.Lock()
	defer t.logMu.Unlock()
	t.log.DeleteBelow(minMeta)
}

// run loops forever, running queued tasks one at a time. Any returned errors are logged.
func (t tasks) run() {
	for {
		// If the server is shutting down, return immediately. Else, fetch a task from the queue.
		var task taskRequest
		select {
		case <-x.ServerCloser.HasBeenClosed():
			break
		case task = <-t.queue:
		}

		// Fetch the task from the log. If the task isn't found, this means it has expired (older
		// than taskTtl.
		meta := TaskMeta(t.Get(task.id))
		if meta == 0 {
			glog.Errorf("task 0x%x: is expired, skipping", task.id)
			continue
		}
		// Only proceed if the task is still queued. It's possible that the task got canceled
		// before we were able to run it.
		if status := meta.Status(); status != TaskStatusQueued {
			glog.Errorf("task 0x%x: status is set to %s, skipping", task.id, status)
		}
		// Change the task status to RUNNING.
		t.set(task.id, newTaskMeta(meta.Kind(), TaskStatusRunning))

		// Run the task.
		var status TaskStatus
		if err := task.run(); err != nil {
			status = TaskStatusError
			glog.Errorf("task 0x%x: %s: %v", task.id, status, err)
		} else {
			status = TaskStatusSuccess
			glog.Infof("task 0x%x: %s", task.id, status)
		}

		// Change the task status to SUCCESS / ERROR.
		t.set(task.id, newTaskMeta(meta.Kind(), status))
	}
}

func (t tasks) QueueBackup(req *pb.BackupRequest) (uint64, error) {
	return t.queueTask(req)
}

func (t tasks) QueueExport(req *pb.ExportRequest) (uint64, error) {
	return t.queueTask(req)
}

// queueTask queues a task of any type. Don't use this function directly.
func (t tasks) queueTask(req interface{}) (uint64, error) {
	task := taskRequest{req: req}
	for attempt := 0; ; attempt++ {
		task.id = t.newId()
		// z.Tree cannot store 0 or math.MaxUint64. Check that taskId is unique.
		if task.id != 0 && task.id != math.MaxUint64 && t.Get(task.id) == 0 {
			break
		}
		// Unable to generate a unique random number.
		if attempt >= 8 {
			t.rng.Seed(time.Now().UnixNano())
			return 0, fmt.Errorf("unable to generate unique task ID")
		}
	}

	var kind TaskKind
	switch req.(type) {
	case *pb.BackupRequest:
		kind = TaskKindBackup
	case *pb.ExportRequest:
		kind = TaskKindExport
	}

	select {
	case t.queue <- task:
		t.set(task.id, newTaskMeta(kind, TaskStatusQueued))
		return task.id, nil
	default:
		return 0, fmt.Errorf("too many pending tasks, please try again later")
	}
}

// newId generates a random task ID.
//
// The format of this is:
// 32 bits: raft ID
// 32 bits: random number
func (t tasks) newId() uint64 {
	return t.raftId<<32 | uint64(t.rng.Int())
}

type taskRequest struct {
	id  uint64
	req interface{} // *pb.BackupRequest, *pb.ExportRequest
}

// run starts a task and blocks till it completes.
func (t taskRequest) run() error {
	switch req := t.req.(type) {
	case *pb.BackupRequest:
		if err := ProcessBackupRequest(context.Background(), req); err != nil {
			return err
		}
	case *pb.ExportRequest:
		files, err := ExportOverNetwork(context.Background(), req)
		if err != nil {
			return err
		}
		glog.Infof("task 0x%x: exported files: %v", t.id, files)
	default:
		glog.Errorf(
			"task 0x%x: received request of unknown type (%T)", t.id, reflect.TypeOf(t.req))
	}
	return nil
}

// TaskMeta stores a timestamp, a TaskKind and a Status.
//
// The format of this is:
// 32 bits: UNIX timestamp (overflows on 2106-02-07)
// 16 bits: TaskKind
// 16 bits: TaskStatus
type TaskMeta uint64

func newTaskMeta(kind TaskKind, status TaskStatus) TaskMeta {
	now := time.Now().UTC().Unix()
	return TaskMeta(now)<<32 | TaskMeta(kind)<<16 | TaskMeta(status)
}

func (t TaskMeta) Timestamp() time.Time {
	return time.Unix(int64(t>>32), 0)
}

func (t TaskMeta) Kind() TaskKind {
	return TaskKind((t >> 16) & math.MaxUint16)
}

func (t TaskMeta) Status() TaskStatus {
	return TaskStatus(t & math.MaxUint16)
}

const (
	// Reserve the zero value for errors.
	TaskKindBackup TaskKind = iota + 1
	TaskKindExport
)

type TaskKind uint64

func (k TaskKind) String() string {
	switch k {
	case TaskKindBackup:
		return "Backup"
	case TaskKindExport:
		return "Export"
	default:
		return "Unknown"
	}
}

const (
	// Reserve the zero value for errors.
	TaskStatusQueued TaskStatus = iota + 1
	TaskStatusRunning
	TaskStatusCanceled
	TaskStatusError
	TaskStatusSuccess
)

type TaskStatus uint64

func (status TaskStatus) String() string {
	switch status {
	case TaskStatusQueued:
		return "Queued"
	case TaskStatusRunning:
		return "Running"
	case TaskStatusCanceled:
		return "Canceled"
	case TaskStatusError:
		return "Error"
	case TaskStatusSuccess:
		return "Success"
	default:
		return "Unknown"
	}
}
