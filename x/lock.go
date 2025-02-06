/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
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
