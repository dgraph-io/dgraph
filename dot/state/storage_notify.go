// Copyright 2020 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.
package state

import (
	"errors"
)

// KeyValue struct to hold key value pairs
type KeyValue struct {
	Key   []byte
	Value []byte
}

// RegisterStorageChangeChannel function to register storage change channels
func (s *StorageState) RegisterStorageChangeChannel(ch chan<- *KeyValue) (byte, error) {
	s.changedLock.RLock()

	if len(s.changed) == 256 {
		return 0, errors.New("channel limit reached")
	}

	var id byte
	for {
		id = generateID()
		if s.changed[id] == nil {
			break
		}
	}

	s.changedLock.RUnlock()

	s.changedLock.Lock()
	s.changed[id] = ch
	s.changedLock.Unlock()
	return id, nil
}

// UnregisterStorageChangeChannel removes the storage change notification channel with the given ID.
// A channel must be unregistered before closing it.
func (s *StorageState) UnregisterStorageChangeChannel(id byte) {
	s.changedLock.Lock()
	defer s.changedLock.Unlock()

	delete(s.changed, id)
}

func (s *StorageState) notifyChanged(change *KeyValue) {
	s.changedLock.RLock()
	defer s.changedLock.RUnlock()

	if len(s.changed) == 0 {
		return
	}

	logger.Trace("notifying changed storage chans...", "chans", s.changed)

	for _, ch := range s.changed {
		go func(ch chan<- *KeyValue) {
			ch <- change
		}(ch)
	}
}
