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
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
)

var (
	Tasks   tasks
	taskRng *rand.Rand
)

func InitTasks() {
	Tasks = newTasks()
	taskRng = rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404

	Tasks.deleteExpired()
	Tasks.cancelQueued()

	go Tasks.run()
}

type tasks struct {
	queue chan task
	log   *z.Tree
}

func newTasks() tasks {
	const maxQueueSize = 16
	return tasks{
		queue: make(chan task, maxQueueSize),
		log:   z.NewTree(),
	}
}

// cancelQueuedTasks marks all queued tasks in the log as canceled.
func (t *tasks) cancelQueued() {
	t.log.IterateKV(func(key, val uint64) uint64 {
		if getTaskStatus(val) == taskStatusQueued {
			return newTaskValue(taskStatusCanceled)
		}
		return 0
	})
}

// deleteExpired deletes all expired tasks in the log.
func (t *tasks) deleteExpired() {
	const ttl = 7 * 24 * time.Hour // 1 week
	minTs := time.Now().UTC().Add(-ttl).Unix()
	minKey := uint64(minTs) << 32
	t.log.DeleteBelow(minKey)
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

		value := t.log.Get(task.key)
		// Fetch the task from the log. If the task isn't found, this means it has expired (older
		// than taskTtl.
		if value == 0 {
			glog.Errorf("task %d is expired, skipping", task.key)
			continue
		}
		// Only proceed if the task is still queued. It's possible that the task got canceled
		// before we were able to run it.
		if status := getTaskStatus(value); status != taskStatusQueued {
			glog.Errorf("task %d is set to %s, skipping", task.key, status)
		}
		// Change the task status to "running".
		t.log.Set(task.key, newTaskValue(taskStatusRunning))

		// Run the task.
		var status taskStatus
		if err := task.run(); err != nil {
			status = taskStatusError
			glog.Errorf("task %d: %s: %v", task.key, status, err)
		} else {
			status = taskStatusSuccess
			glog.Infof("task %d: %s", task.key, status)
		}

		// Change the task status to "complete" / "error".
		t.log.Set(task.key, newTaskValue(status))
	}
}

func (t tasks) QueueBackup(req *pb.BackupRequest) (int64, error) {
	return t.queueTask(req)
}

func (t tasks) QueueExport(req *pb.ExportRequest) (int64, error) {
	return t.queueTask(req)
}

// queueTask queues a task of any type. Don't use this function directly.
func (t tasks) queueTask(req interface{}) (int64, error) {
	task := task{req: req}
	for attempt := 0; ; attempt++ {
		task.key = newTaskKey()
		if t.log.Get(task.key) == 0 {
			break
		}
		// Unable to generate a unique random number.
		if attempt >= 8 {
			taskRng.Seed(time.Now().UnixNano())
			return 0, fmt.Errorf("unable to generate unique task ID")
		}
	}

	select {
	case t.queue <- task:
		t.log.Set(task.key, newTaskValue(taskStatusQueued))
		// Casting to int64 is safe here because task.key is always < math.MaxInt64
		return int64(task.key), nil
	default:
		return 0, fmt.Errorf("too many pending tasks, please try again later")
	}
}

type task struct {
	key uint64
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
		glog.Infof("task %d: exported files: %v", t.key, files)
	default:
		err := fmt.Errorf(
			"task %d: received request of unknown type (%T)", t.key, reflect.TypeOf(t.req))
		panic(err)
	}
	return nil
}

// newTaskKey returns a new task key in the range [1, math.MaxInt64].
func newTaskKey() uint64 {
	rnd := taskRng.Int31()
	raftId := 1 // TODO(ajeet)
	return uint64(rnd)<<32 | uint64(raftId)
}

// newTaskValue returns a new task value with the given status.
func newTaskValue(status taskStatus) uint64 {
	now := time.Now().UTC().Unix()
	return uint64(now)<<32 | uint64(status)
}

// newTaskValue extracts a taskStatus from a task value.
func getTaskStatus(value uint64) taskStatus {
	return taskStatus(value) & math.MaxUint32
}

type taskStatus uint64

const (
	taskStatusQueued taskStatus = iota + 1
	taskStatusRunning
	taskStatusCanceled
	taskStatusError
	taskStatusSuccess
)

func (status taskStatus) String() string {
	switch status {
	case taskStatusQueued:
		return "QUEUED"
	case taskStatusRunning:
		return "RUNNING"
	case taskStatusCanceled:
		return "CANCELED"
	case taskStatusError:
		return "ERROR"
	case taskStatusSuccess:
		return "SUCCESS"
	default:
		return "UNKNOWN"
	}
}
