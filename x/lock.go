package x

import (
	"sync"
	"sync/atomic"
)

// SafeLock can be used in place of sync.RWMutex
type SafeMutex struct {
	m       sync.RWMutex
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
