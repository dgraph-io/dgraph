/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
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
	"sync/atomic"
)

// SafeLock can be used in place of sync.RWMutex
type SafeMutex struct {
	m       sync.RWMutex
	wait    *SafeWait
	writer  int32
	readers int32
}

func (s *SafeMutex) Lock() {
	s.m.Lock()
	AssertTrue(atomic.AddInt32(&s.writer, 1) == 1)
}

func (s *SafeMutex) Unlock() {
	AssertTrue(atomic.AddInt32(&s.writer, -1) == 0)
	s.m.Unlock()
}

func (s *SafeMutex) AssertLock() {
	AssertTrue(atomic.LoadInt32(&s.writer) == 1)
}

func (s *SafeMutex) RLock() {
	s.m.RLock()
	atomic.AddInt32(&s.readers, 1)
}

func (s *SafeMutex) RUnlock() {
	atomic.AddInt32(&s.readers, -1)
	s.m.RUnlock()
}

func (s *SafeMutex) AssertRLock() {
	AssertTrue(atomic.LoadInt32(&s.readers) > 0 ||
		atomic.LoadInt32(&s.writer) == 1)
}

type SafeWait struct {
	wg      sync.WaitGroup
	waiting int32
}

func (s *SafeWait) Done() {
	AssertTrue(s != nil && atomic.LoadInt32(&s.waiting) > 0)
	s.wg.Done()
	atomic.AddInt32(&s.waiting, -1)
}

func (s *SafeMutex) StartWait() *SafeWait {
	s.AssertLock()
	if s.wait != nil {
		AssertTrue(atomic.LoadInt32(&s.wait.waiting) == 0)
	}
	s.wait = new(SafeWait)
	s.wait.wg = sync.WaitGroup{}
	s.wait.wg.Add(1)
	atomic.AddInt32(&s.wait.waiting, 1)
	return s.wait
}

func (s *SafeMutex) Wait() {
	s.AssertRLock()
	if s.wait == nil {
		return
	}
	atomic.AddInt32(&s.wait.waiting, 1)
	s.wait.wg.Wait()
	atomic.AddInt32(&s.wait.waiting, -1)
}
