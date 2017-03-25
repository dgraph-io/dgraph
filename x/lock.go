/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
