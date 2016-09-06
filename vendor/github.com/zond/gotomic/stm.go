package gotomic

import (
	"bytes"
	"fmt"
	"sort"
	"sync/atomic"
	"unsafe"
)

const (
	undecided = iota
	read_check
	successful
	failed
)

var lastCommit uint64 = 0
var lastBegin uint64 = 0

/*
 Clonable types can be handled by the transaction layer.
*/
type Clonable interface {
	Clone() Clonable
}

/*
 Handle wraps any type of data that is supposed to be handled by the transaction layer.
*/
type Handle struct {
	/*
	 Will point to a version.
	*/
	Pointer unsafe.Pointer
}

/*
 NewHandle will wrap a Clonable value to enable its use in the transaction layer.
*/
func NewHandle(c Clonable) *Handle {
	return &Handle{unsafe.Pointer(&version{0, nil, c})}
}

/*
 Current returns the current content of this Handle, disregarding any transactional state.
*/
func (self *Handle) Current() Clonable {
	return self.getVersion().content
}
func (self *Handle) getVersion() *version {
	return (*version)(atomic.LoadPointer(&self.Pointer))
}
func (self *Handle) replace(old, neu *version) bool {
	return atomic.CompareAndSwapPointer(&self.Pointer, unsafe.Pointer(old), unsafe.Pointer(neu))
}

type version struct {
	/*
	 The number of the transaction that created this version.
	*/
	commitNumber uint64
	/*
	 The transaction (or nil) having locked this version.
	*/
	lockedBy *Transaction
	/*
	 The content in this version.
	*/
	content Clonable
}

func (self *version) clone() *version {
	return &version{atomic.LoadUint64(&self.commitNumber), nil, self.content.Clone()}
}

type snapshot struct {
	old *version
	neu *version
}

type write struct {
	handle   *Handle
	snapshot *snapshot
}

type writes []write

func (self writes) Len() int {
	return len(self)
}
func (self writes) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}
func (self writes) Less(i, j int) bool {
	return uintptr(unsafe.Pointer(self[i].handle)) < uintptr(unsafe.Pointer(self[j].handle))
}

/*
 Transaction is based on "Concurrent Programming Without Locks" by Keir Fraser and Tim Harris <http://www.cl.cam.ac.uk/research/srg/netos/papers/2007-cpwl.pdf>

 It has a few tweaks that I don't believe break it (but I haven't even tried proving it):

 1) It has an ever increasing counter for the last transaction to commit.

 It uses this counter to fail transactions fast when they try to read a value that another
 transaction has changed since the first transaction began.

 2) It copies the data not only on write opening, but also on read opening.

 These changes will make the transactions act more along the lines of "Sandboxing Transactional Memory" by Luke Dalessandro and Michael L. Scott <http://www.cs.rochester.edu/u/scott/papers/2012_TRANSACT_sandboxing.pdf> and will hopefully avoid the need to kill transactions exhibiting invalid behaviour due to inconsistent states.
*/
type Transaction struct {
	/*
	 Steadily incrementing number for each committed transaction.
	*/
	beginNumber  uint64
	commitNumber uint64
	status       int32
	readHandles  map[*Handle]*snapshot
	writeHandles map[*Handle]*snapshot
	sortedWrites writes
}

func NewTransaction() *Transaction {
	return &Transaction{
		atomic.AddUint64(&lastBegin, 1),
		atomic.LoadUint64(&lastCommit),
		undecided,
		make(map[*Handle]*snapshot),
		make(map[*Handle]*snapshot),
		nil,
	}
}
func (self *Transaction) getStatus() int32 {
	return atomic.LoadInt32(&self.status)
}
func (self *Transaction) objRead(h *Handle) (rval *version, err error) {
	version := h.getVersion()
	other := version.lockedBy
	if other != nil {
		if other.getStatus() == read_check && self.getStatus() == read_check && self.beginNumber < other.beginNumber {
			other.Abort()
		} else {
			other.commit()
		}
		if other.getStatus() == successful {
			version = other.writeHandles[h].neu
			atomic.StoreUint64(&version.commitNumber, atomic.LoadUint64(&other.commitNumber))
		}
	}
	if atomic.LoadUint64(&version.commitNumber) > atomic.LoadUint64(&self.commitNumber) {
		err = fmt.Errorf("%v has changed", version.content)
	} else {
		rval = version
	}
	return
}

/*
 sortWrites will put all writeHandles in sortedWrites and remove writeHandles.

 _not_ safe to call multiple times!
*/
func (self *Transaction) sortWrites() {
	for handle, snapshot := range self.writeHandles {
		self.sortedWrites = append(self.sortedWrites, write{handle, snapshot})
	}
	sort.Sort(self.sortedWrites)
}
func (self *Transaction) release() {
	stat := self.getStatus()
	for _, w := range self.sortedWrites {
		current := w.handle.getVersion()
		if current.lockedBy == self {
			wanted := w.snapshot.old
			if stat == successful {
				wanted = w.snapshot.neu
				atomic.StoreUint64(&wanted.commitNumber, atomic.LoadUint64(&self.commitNumber))
			}
			w.handle.replace(current, wanted)
		}
	}
}
func (self *Transaction) acquire() bool {
	for _, w := range self.sortedWrites {
		for {
			lockedVersion := w.snapshot.old.clone()
			lockedVersion.lockedBy = self
			if w.handle.replace(w.snapshot.old, lockedVersion) {
				break
			}
			current := w.handle.getVersion()
			if current.lockedBy == self {
				break
			}
			switch status := self.getStatus(); status {
			case read_check:
				return true
			case successful:
				return true
			case failed:
				return false
			}
			if current.lockedBy == nil {
				return false
			}
			current.lockedBy.commit()
		}
	}
	return true
}
func (self *Transaction) readCheck() bool {
	for handle, snapshot := range self.readHandles {
		if handle.getVersion() != snapshot.old {
			if self.getStatus() == successful {
				return true
			}
			return false
		}
	}
	return true
}

/*
 commit the transaction without mutating anything inside it except with atomic
 methods. Useful for other helpful Transactions.

 Safe to call multiple times (and it _will_ be if we have contention).
*/
func (self *Transaction) commit() bool {
	if !self.acquire() {
		self.Abort()
		return false
	}
	defer self.release()
	if atomic.CompareAndSwapInt32(&self.status, undecided, read_check) {
		atomic.StoreUint64(&self.commitNumber, atomic.AddUint64(&lastCommit, 1))
	}
	if !self.readCheck() {
		self.Abort()
		return false
	}
	atomic.CompareAndSwapInt32(&self.status, read_check, successful)
	return self.getStatus() == successful
}

/*
 Commit the transaction. Will return whether the commit was successful or not.

 Safe to call multiple times, but only from one thread.
*/
func (self *Transaction) Commit() bool {
	status := self.getStatus()
	if status == undecided {
		self.sortWrites()
		return self.commit()
	} else if status == failed {
		return false
	} else if status == successful {
		return true
	} else if status == read_check {
		return self.commit()
	}
	panic(fmt.Errorf("%#v has illegal state!"))
}

/*
 Abort the transaction unless it is already successful.

 Safe to call multiple times.

 Unless the transaction is half-committed Abort isn't really necessary, the gc will clean it up properly.
*/
func (self *Transaction) Abort() {
	stat := self.getStatus()
	for stat != successful && stat != failed {
		atomic.CompareAndSwapInt32(&self.status, stat, failed)
		stat = self.getStatus()
	}
	self.release()
}

func (self *Transaction) Describe() string {
	buf := bytes.NewBufferString(fmt.Sprintf("Transaction:%p (beginNumber: %v, commitNumber: %v):\n readHandles:\n", self, self.beginNumber, self.commitNumber))
	for _, snapshot := range self.readHandles {
		fmt.Fprintf(buf, "  %v (%v) => %v\n", snapshot.old.content, snapshot.old.commitNumber, snapshot.neu.content)
	}
	fmt.Fprint(buf, " writeHandles:\n")
	for _, snapshot := range self.writeHandles {
		fmt.Fprintf(buf, "  %v (%v) => %v\n", snapshot.old.content, snapshot.old.commitNumber, snapshot.neu.content)
	}
	return string(buf.Bytes())
}

/*
 Read will return a version of the data in h that is guaranteed to not have been changed since this Transaction started.

 Any changes made to the return value will *not* be saved when the Transaction commits.

 If another Transaction changes the data in h before this Transaction commits the commit will fail.
*/
func (self *Transaction) Read(h *Handle) (rval Clonable, err error) {
	if self.getStatus() != undecided {
		return nil, fmt.Errorf("%v is not undecided", self)
	}
	if snapshot, ok := self.readHandles[h]; ok {
		return snapshot.neu.content, nil
	}
	if snapshot, ok := self.writeHandles[h]; ok {
		return snapshot.neu.content, nil
	}
	oldVersion, err := self.objRead(h)
	if err != nil {
		return nil, err
	}
	newVersion := oldVersion.clone()
	self.readHandles[h] = &snapshot{oldVersion, newVersion}
	return newVersion.content, nil
}

/*
 Write will return a version of the data in h that is guaranteed to not have been changed since this Transaction started.

 All changes made to the return value *will* be saved when the Transaction commits.

 If another Transaction changes the data in h before this Transaction commits the commit will fail.
*/
func (self *Transaction) Write(h *Handle) (rval Clonable, err error) {
	if self.getStatus() != undecided {
		return nil, fmt.Errorf("%v is not undecided", self)
	}
	if snapshot, ok := self.writeHandles[h]; ok {
		return snapshot.neu.content, nil
	}
	if snapshot, ok := self.readHandles[h]; ok {
		delete(self.readHandles, h)
		self.writeHandles[h] = snapshot
		return snapshot.neu.content, nil
	}
	oldVersion, err := self.objRead(h)
	if err != nil {
		return nil, err
	}
	newVersion := oldVersion.clone()
	self.writeHandles[h] = &snapshot{oldVersion, newVersion}
	return newVersion.content, nil
}
