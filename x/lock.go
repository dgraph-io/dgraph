/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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
	"sync/atomic"
)

// SafeMutex can be used in place of sync.RWMutex. It allows code to assert
// whether the mutex is locked.
type SafeMutex struct {
	m sync.RWMutex
	// m deadlock.RWMutex // Useful during debugging and testing for detecting locking issues.
	writer  int32
	readers int32
}

// AlreadyLocked returns true if safe mutex is already being held.
func (s *SafeMutex) AlreadyLocked() bool {
	return atomic.LoadInt32(&s.writer) > 0
}

// Lock locks the safe mutex.
func (s *SafeMutex) Lock() {
	s.m.Lock()
	AssertTrue(atomic.AddInt32(&s.writer, 1) == 1)
}

// Unlock unlocks the safe mutex.
func (s *SafeMutex) Unlock() {
	AssertTrue(atomic.AddInt32(&s.writer, -1) == 0)
	s.m.Unlock()
}

// AssertLock asserts whether the lock is being held.
func (s *SafeMutex) AssertLock() {
	AssertTrue(s.AlreadyLocked())
}

// RLock holds the reader lock.
func (s *SafeMutex) RLock() {
	s.m.RLock()
	atomic.AddInt32(&s.readers, 1)
}

// RUnlock releases the reader lock.
func (s *SafeMutex) RUnlock() {
	atomic.AddInt32(&s.readers, -1)
	s.m.RUnlock()
}

// AssertRLock asserts whether the reader lock is being held.
func (s *SafeMutex) AssertRLock() {
	AssertTrue(atomic.LoadInt32(&s.readers) > 0 ||
		atomic.LoadInt32(&s.writer) == 1)
}
