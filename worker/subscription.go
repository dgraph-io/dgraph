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

package worker

import (
	"sync"
)

type UpdateObserver interface {
	PredicateUpdated(predicate string)
}

// UpdateDispatcher is a thread-safe implementation of Subject from Observer pattern.
type UpdateDispatcher struct {
	observers map[string]map[UpdateObserver]bool
	mutex     sync.RWMutex
}

func NewUpdateDispatcher() *UpdateDispatcher {
	return &UpdateDispatcher{observers: make(map[string]map[UpdateObserver]bool)}
}

// Registers observer for monitoring of all updates related to predicates
func (d *UpdateDispatcher) GetUpdateStream(predicates []string, observer UpdateObserver) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	for _, predicate := range predicates {
		set, ok := d.observers[predicate]
		if !ok {
			set = make(map[UpdateObserver]bool)
			d.observers[predicate] = set
		}
		set[observer] = true
	}
	return nil
}

// Update should be called if and only if there was a change related to given predicate.
// This method notifies all observers registered for the predicated about the change.
// As a side effect, all observers that are not alive are unregistered.
func (d *UpdateDispatcher) Update(predicate string) error {
	d.mutex.RLock()
	for observer, _ := range d.observers[predicate] {
		go observer.PredicateUpdated(predicate)
	}
	d.mutex.RUnlock()

	return nil
}

// Removes observer from dispatcher. Each observer should call this method as soon as it is
// no longer interested in updates or stopped working.
func (d *UpdateDispatcher) Remove(observer UpdateObserver) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	for _, set := range d.observers {
		delete(set, observer)
	}
}
