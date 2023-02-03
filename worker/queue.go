/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

// TaskStatusOverNetwork fetches the status of a task over the network. Alphas only know about the
// tasks created by them, but this function would fetch the task from the correct Alpha.
func TaskStatusOverNetwork(ctx context.Context, req *pb.TaskStatusRequest,
) (*pb.TaskStatusResponse, error) {
	// Extract Raft ID from Task ID.
	taskId := req.GetTaskId()
	if taskId == 0 {
		return nil, fmt.Errorf("invalid task ID: %#x", taskId)
	}
	raftId := taskId >> 32

	// Skip the network call if the required Alpha is me.
	myRaftId := State.WALstore.Uint(raftwal.RaftId)
	if raftId == myRaftId {
		worker := (*grpcWorker)(nil)
		return worker.TaskStatus(ctx, req)
	}

	// Find the Alpha with the required Raft ID.
	var addr string
	for _, group := range groups().state.GetGroups() {
		for _, member := range group.GetMembers() {
			if member.GetId() == raftId {
				addr = member.GetAddr()
			}
		}
	}
	if addr == "" {
		return nil, fmt.Errorf("the Alpha that served that task is not available")
	}

	// Send the request to the Alpha.
	pool, err := conn.GetPools().Get(addr)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to reach the Alpha that served that task")
	}
	client := pb.NewWorkerClient(pool.Get())
	return client.TaskStatus(ctx, req)
}

// TaskStatus retrieves metadata for a given task ID.
func (*grpcWorker) TaskStatus(ctx context.Context, req *pb.TaskStatusRequest,
) (*pb.TaskStatusResponse, error) {
	taskId := req.GetTaskId()
	meta, err := Tasks.get(taskId)
	if err != nil {
		return nil, err
	}

	resp := &pb.TaskStatusResponse{TaskMeta: meta.uint64()}
	return resp, nil
}

var (
	// Tasks is a global persistent task queue.
	// Do not use this before calling InitTasks.
	Tasks *tasks
)

// InitTasks initializes the global Tasks variable.
func InitTasks() {
	path := filepath.Join(x.WorkerConfig.TmpDir, "tasks.buf")
	log, err := z.NewTreePersistent(path)
	x.Check(err)

	// #nosec G404: weak RNG
	Tasks = &tasks{
		queue: make(chan taskRequest, 16),
		log:   log,
		logMu: new(sync.Mutex),
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Mark all pending tasks as failed.
	Tasks.logMu.Lock()
	Tasks.log.IterateKV(func(id, val uint64) uint64 {
		meta := TaskMeta(val)
		if status := meta.Status(); status == TaskStatusQueued || status == TaskStatusRunning {
			return uint64(newTaskMeta(meta.Kind(), TaskStatusFailed))
		}
		return 0
	})
	Tasks.logMu.Unlock()

	// Start the task runner.
	go Tasks.worker()
}

// tasks is a persistent task queue.
type tasks struct {
	// queue stores the full Protobuf request.
	queue chan taskRequest
	// log stores the timestamp, TaskKind, and TaskStatus.
	log   *z.Tree
	logMu *sync.Mutex

	rng *rand.Rand
}

// Enqueue adds a new task to the queue, waits for 3 seconds, and returns any errors that
// may have happened in that span of time. The request must be of type:
// - *pb.BackupRequest
// - *pb.ExportRequest
func (t *tasks) Enqueue(req interface{}) (uint64, error) {
	if t == nil {
		return 0, fmt.Errorf("task queue hasn't been initialized yet")
	}

	id, err := t.enqueue(req)
	if err != nil {
		return 0, err
	}

	// Wait for upto 3 seconds to check for errors.
	for i := 0; i < 3; i++ {
		time.Sleep(time.Second)

		t.logMu.Lock()
		meta := TaskMeta(t.log.Get(id))
		t.logMu.Unlock()

		// Early return
		switch meta.Status() {
		case TaskStatusFailed:
			return 0, fmt.Errorf("task failed")
		case TaskStatusSuccess:
			return id, nil
		}
	}

	return id, nil
}

// enqueue adds a new task to the queue. This must be of type:
// - *pb.BackupRequest
// - *pb.ExportRequest
func (t *tasks) enqueue(req interface{}) (uint64, error) {
	var kind TaskKind
	switch req.(type) {
	case *pb.BackupRequest:
		kind = TaskKindBackup
	case *pb.ExportRequest:
		kind = TaskKindExport
	default:
		err := fmt.Errorf("invalid TaskKind: %d", kind)
		panic(err)
	}

	t.logMu.Lock()
	defer t.logMu.Unlock()

	task := taskRequest{
		id:  t.newId(),
		req: req,
	}
	select {
	// t.logMu must be acquired before pushing to t.queue, otherwise the worker might start the
	// task, and won't be able to find it in t.log.
	case t.queue <- task:
		t.log.Set(task.id, newTaskMeta(kind, TaskStatusQueued).uint64())
		return task.id, nil
	default:
		return 0, fmt.Errorf("too many pending tasks, please try again later")
	}
}

// get retrieves metadata for a given task ID.
func (t *tasks) get(id uint64) (TaskMeta, error) {
	if t == nil {
		return 0, fmt.Errorf("task queue hasn't been initialized yet")
	}

	if id == 0 || id == math.MaxUint64 {
		return 0, fmt.Errorf("task ID is invalid: %d", id)
	}
	t.logMu.Lock()
	defer t.logMu.Unlock()
	meta := TaskMeta(t.log.Get(id))
	if meta == 0 {
		return 0, fmt.Errorf("task does not exist or has expired")
	}
	return meta, nil
}

// worker loops forever, running queued tasks one at a time. Any returned errors are logged.
func (t *tasks) worker() {
	shouldCleanup := time.NewTicker(time.Hour)
	defer shouldCleanup.Stop()
	for {
		// If the server is shutting down, return immediately. Else, fetch a task from the queue.
		var task taskRequest
		select {
		case <-x.ServerCloser.HasBeenClosed():
			t.log.Close()
			return
		case <-shouldCleanup.C:
			t.cleanup()
		case task = <-t.queue:
			if err := t.run(task); err != nil {
				glog.Errorf("task %#x: failed: %s", task.id, err)
			} else {
				glog.Infof("task %#x: completed successfully", task.id)
			}
		}
	}
}

func (t *tasks) run(task taskRequest) error {
	// Fetch the task from the log. If the task isn't found, this means it has expired (older than
	// taskTtl).
	t.logMu.Lock()
	meta := TaskMeta(t.log.Get(task.id))
	t.logMu.Unlock()
	if meta == 0 {
		return fmt.Errorf("is expired, skipping")
	}

	// Only proceed if the task is still queued. It's possible that the task got canceled before we
	// were able to run it.
	if status := meta.Status(); status != TaskStatusQueued {
		return fmt.Errorf("status is set to %s, skipping", status)
	}

	// Change the task status to Running.
	t.logMu.Lock()
	t.log.Set(task.id, newTaskMeta(meta.Kind(), TaskStatusRunning).uint64())
	t.logMu.Unlock()

	// Run the task.
	var status TaskStatus
	err := task.run()
	if err != nil {
		status = TaskStatusFailed
	} else {
		status = TaskStatusSuccess
	}

	// Change the task status to Success / Failed.
	t.logMu.Lock()
	t.log.Set(task.id, newTaskMeta(meta.Kind(), status).uint64())
	t.logMu.Unlock()

	// Return the error from the task.
	return err
}

// cleanup deletes all expired tasks.
func (t *tasks) cleanup() {
	const taskTtl = 7 * 24 * time.Hour // 1 week
	minTs := time.Now().UTC().Add(-taskTtl).Unix()
	minMeta := uint64(minTs) << 32

	t.logMu.Lock()
	defer t.logMu.Unlock()
	t.log.DeleteBelow(minMeta)
}

// newId generates a random unique task ID. logMu must be acquired before calling this function.
//
// The format of this is:
// 32 bits: raft ID
// 32 bits: random number
func (t *tasks) newId() uint64 {
	myRaftId := State.WALstore.Uint(raftwal.RaftId)
	for {
		id := myRaftId<<32 | uint64(t.rng.Intn(math.MaxUint32))
		// z.Tree cannot store 0 or math.MaxUint64. Check that id is unique.
		if id != 0 && id != math.MaxUint64 && t.log.Get(id) == 0 {
			return id
		}
	}
}

type taskRequest struct {
	id  uint64
	req interface{} // *pb.BackupRequest, *pb.ExportRequest
}

// run starts a task and blocks till it completes.
func (t *taskRequest) run() error {
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
		glog.Infof("task %#x: exported files: %v", t.id, files)
	default:
		glog.Errorf(
			"task %#x: received request of unknown type (%T)", t.id, reflect.TypeOf(t.req))
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

// Timestamp returns the timestamp of the last status change of the task.
func (t TaskMeta) Timestamp() time.Time {
	return time.Unix(int64(t>>32), 0)
}

// Kind returns the type of the task.
func (t TaskMeta) Kind() TaskKind {
	return TaskKind((t >> 16) & math.MaxUint16)
}

// Status returns the current status of the task.
func (t TaskMeta) Status() TaskStatus {
	return TaskStatus(t & math.MaxUint16)
}

// uint64 represents the TaskMeta as a uint64.
func (t TaskMeta) uint64() uint64 {
	return uint64(t)
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
	TaskStatusFailed
	TaskStatusSuccess
)

type TaskStatus uint64

func (status TaskStatus) String() string {
	switch status {
	case TaskStatusQueued:
		return "Queued"
	case TaskStatusRunning:
		return "Running"
	case TaskStatusFailed:
		return "Failed"
	case TaskStatusSuccess:
		return "Success"
	default:
		return "Unknown"
	}
}
