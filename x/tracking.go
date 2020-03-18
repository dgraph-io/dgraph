/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"sync"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

var (
	// Used to track the tasks going on in the system.
	tasks    map[int]*Task = make(map[int]*Task)
	tasksMut sync.Mutex
)

// Task is something that needs to be tracked.
type Task struct {
	// id of the Task.
	ID int
	// Closer is used to signal and wait for the task to finish/cancel.
	Closer *y.Closer
}

const (
	TaskRollup = iota + 1
	TaskSnapshot
	TaskIndexing
)

// StartTask is used to check whether a task is already going on.
// If rollup is going on, we cancel and wait for rollup to complete
// before we return. If the same task is already going, we return error.
func StartTask(id int) (*Task, error) {
	tasksMut.Lock()
	defer tasksMut.Unlock()

	switch id {
	case TaskRollup:
		if len(tasks) > 0 {
			return nil, errors.Errorf("a task is already running, tasks:%v", tasks)
		}
	case TaskSnapshot, TaskIndexing:
		if t, ok := tasks[TaskRollup]; ok {
			glog.Info("Found a rollup going on. waiting for rollup!")
			t.Closer.SignalAndWait()
			glog.Info("Rollup cancelled.")
		} else if len(tasks) > 0 {
			return nil, errors.Errorf("a task is already running, tasks:%v", tasks)
		}
	default:
		glog.Errorf("Got an unhandled task %d. Ignoring...", id)
		return nil, nil
	}

	t := &Task{ID: id, Closer: y.NewCloser(1)}
	tasks[id] = t
	glog.Infof("Task started with id: %d", id)
	return t, nil
}

// StopTask will delete the entry from the map that keeps track of
// the tasks and then signal that task has been cancelled/completed.
func StopTask(t *Task) {
	t.Closer.Done()

	tasksMut.Lock()
	delete(tasks, t.ID)
	tasksMut.Unlock()
	glog.Infof("Task completed with id: %d", t.ID)
}

// WaitForTask waits for a given task to complete.
func WaitForTask(id int) {
	tasksMut.Lock()
	t, ok := tasks[id]
	tasksMut.Unlock()
	if !ok {
		return
	}
	t.Closer.Wait()
}
