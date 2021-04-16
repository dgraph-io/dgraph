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
		queue:  make(chan task, 16),
		log:    z.NewTree(),
		raftId: State.WALstore.Uint(raftwal.RaftId),
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	Tasks.deleteExpired()
	Tasks.cancelQueued()
	go Tasks.run()
}

type tasks struct {
	queue  chan task
	log    *z.Tree
	raftId uint64
	rng    *rand.Rand
}

func (t tasks) GetStatus(id uint64) TaskStatus {
	glog.Infof("Fetching task status: 0x%x", id)
	if id == 0 || id == math.MaxUint64 {
		return 0
	}

	value := t.log.Get(id)
	return getTaskStatus(value)
}

// cancelQueuedTasks marks all queued tasks in the log as canceled.
func (t tasks) cancelQueued() {
	t.log.IterateKV(func(id, val uint64) uint64 {
		if getTaskStatus(val) == TaskStatusQueued {
			return newTaskValue(TaskStatusCanceled)
		}
		return 0
	})
}

// deleteExpired deletes all expired tasks in the log.
func (t tasks) deleteExpired() {
	const ttl = 7 * 24 * time.Hour // 1 week
	minTs := time.Now().UTC().Add(-ttl).Unix()
	minId := uint64(minTs) << 32
	t.log.DeleteBelow(minId)
}

// run loops forever, running queued tasks one at a time. Any returned errors are logged.
func (t tasks) run() {
	for {
		// If the server is shutting down, return immediately. Else, fetch a task from the queue.
		var task task
		select {
		case <-x.ServerCloser.HasBeenClosed():
			break
		case task = <-t.queue:
		}

		// Fetch the task from the log. If the task isn't found, this means it has expired (older
		// than taskTtl.
		value := t.log.Get(task.id)
		if value == 0 {
			glog.Errorf("task 0x%x: is expired, skipping", task.id)
			continue
		}
		// Only proceed if the task is still queued. It's possible that the task got canceled
		// before we were able to run it.
		if status := getTaskStatus(value); status != TaskStatusQueued {
			glog.Errorf("task 0x%x: status is set to %s, skipping", task.id, status)
		}
		// Change the task status to RUNNING.
		t.log.Set(task.id, newTaskValue(TaskStatusRunning))

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
		t.log.Set(task.id, newTaskValue(status))
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
	task := task{req: req}
	for attempt := 0; ; attempt++ {
		task.id = t.newId()
		if t.log.Get(task.id) == 0 {
			break
		}
		// Unable to generate a unique random number.
		if attempt >= 8 {
			t.rng.Seed(time.Now().UnixNano())
			return 0, fmt.Errorf("unable to generate unique task ID")
		}
	}

	select {
	case t.queue <- task:
		t.log.Set(task.id, newTaskValue(TaskStatusQueued))
		return task.id, nil
	default:
		return 0, fmt.Errorf("too many pending tasks, please try again later")
	}
}

// TODO(ajeet): figure out edge cases
func (t tasks) newId() uint64 {
	rnd := t.rng.Int()
	return uint64(rnd)<<32 | uint64(t.raftId)
}

type task struct {
	id  uint64
	req interface{} // *pb.BackupRequest, *pb.ExportRequest
}

// run starts a task and blocks till it completes.
func (t task) run() error {
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
		err := fmt.Errorf(
			"task 0x%x: received request of unknown type (%T)", t.id, reflect.TypeOf(t.req))
		panic(err)
	}
	return nil
}

// newTaskValue returns a new task value with the given status.
func newTaskValue(status TaskStatus) uint64 {
	now := time.Now().UTC().Unix()
	// It's safe to use a 32-bit timestamp, this will only overflow on 2106-02-07.
	return uint64(now)<<32 | uint64(status)
}

// newTaskValue extracts a taskStatus from a task value.
func getTaskStatus(value uint64) TaskStatus {
	return TaskStatus(value) & math.MaxUint32
}

type TaskStatus uint64

const (
	// newTaskValue must never return zero, since z.Tree does not support zero values.
	// Starting taskStatus at 1 guarantees this.
	TaskStatusQueued TaskStatus = iota + 1
	TaskStatusRunning
	TaskStatusCanceled
	TaskStatusError
	TaskStatusSuccess
)

func (status TaskStatus) String() string {
	switch status {
	case TaskStatusQueued:
		return "QUEUED"
	case TaskStatusRunning:
		return "RUNNING"
	case TaskStatusCanceled:
		return "CANCELED"
	case TaskStatusError:
		return "ERROR"
	case TaskStatusSuccess:
		return "SUCCESS"
	default:
		return "UNKNOWN"
	}
}
