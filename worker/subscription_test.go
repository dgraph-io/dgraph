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
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockObserver struct {
	mock.Mock
	dispatcher *UpdateDispatcher
	wg         *sync.WaitGroup
}

func NewMockObserver(dispatcher *UpdateDispatcher, wg *sync.WaitGroup) *mockObserver {
	return &mockObserver{dispatcher: dispatcher, wg: wg}
}

func (m *mockObserver) PredicateUpdated(predicate string) {
	if !m.isActive() {
		m.dispatcher.Remove(m)
	} else {
		m.wg.Done()
		m.Called(predicate)
	}
}

func (m *mockObserver) isActive() bool {
	args := m.Called()
	return args.Bool(0)
}

func TestSubscriptionGetUpdateStream(t *testing.T) {
	predicates := []string{"pred1", "pred2", "pred3"}
	wg := new(sync.WaitGroup)
	updateDispatcher := NewUpdateDispatcher()
	observer := NewMockObserver(updateDispatcher, wg)
	wg.Add(3) // 3 x PredicateUpdated
	observer.On("isActive").Return(true)
	observer.On("PredicateUpdated", "pred1").Return().Once()
	observer.On("PredicateUpdated", "pred2").Return().Twice()
	defer observer.AssertExpectations(t)

	err := updateDispatcher.GetUpdateStream(predicates, observer)
	require.NoError(t, err)

	err = updateDispatcher.Update("pred1")
	require.NoError(t, err)
	err = updateDispatcher.Update("pred2")
	require.NoError(t, err)

	err = updateDispatcher.Update("pred4")
	require.NoError(t, err)

	err = updateDispatcher.Update("pred2")
	require.NoError(t, err)

	require.False(t, waitTimeout(wg, time.Second))
	observer.AssertNotCalled(t, "PredicateUpdated", "pred4")
}

func TestSubscriptionMultipleObservers(t *testing.T) {
	predicates1 := []string{"pred1", "pred2"}
	wg := new(sync.WaitGroup)
	updateDispatcher := NewUpdateDispatcher()
	observer1 := NewMockObserver(updateDispatcher, wg)
	wg.Add(2) // 2 x Predicate Updated
	observer1.On("isActive").Return(true)
	observer1.On("PredicateUpdated", "pred1").Return().Once()
	observer1.On("PredicateUpdated", "pred2").Return().Once()
	defer observer1.AssertExpectations(t)

	predicates2 := []string{"pred3", "pred2"}
	observer2 := NewMockObserver(updateDispatcher, wg)
	wg.Add(2) // 2 x Predicate Updated
	observer2.On("isActive").Return(true)
	observer2.On("PredicateUpdated", "pred3").Return().Once()
	observer2.On("PredicateUpdated", "pred2").Return().Once()
	defer observer2.AssertExpectations(t)

	err := updateDispatcher.GetUpdateStream(predicates1, observer1)
	require.NoError(t, err)

	err = updateDispatcher.GetUpdateStream(predicates2, observer2)
	require.NoError(t, err)

	err = updateDispatcher.Update("pred1")
	require.NoError(t, err)
	err = updateDispatcher.Update("pred2")
	require.NoError(t, err)
	err = updateDispatcher.Update("pred3")
	require.NoError(t, err)

	require.False(t, waitTimeout(wg, time.Second))

	observer1.AssertNotCalled(t, "PredicateUpdated", "pred3")
	observer2.AssertNotCalled(t, "PredicateUpdated", "pred1")
}

func TestSubscriptionDeadObserverRemoved(t *testing.T) {
	predicates := []string{"pred1", "pred2"}
	wg := new(sync.WaitGroup)
	updateDispatcher := NewUpdateDispatcher()
	observer := NewMockObserver(updateDispatcher, wg)
	wg.Add(2) // 2 x Predicate Updated
	observer.On("isActive").Return(true).Twice()
	observer.On("isActive").Return(false)
	observer.On("PredicateUpdated", "pred1").Return().Twice()
	defer observer.AssertExpectations(t)

	observer2 := NewMockObserver(updateDispatcher, wg)
	wg.Add(5) // 5 x Predicate Updated
	observer2.On("isActive").Return(true)
	observer2.On("PredicateUpdated", "pred1").Return().Times(5)
	defer observer2.AssertExpectations(t)

	err := updateDispatcher.GetUpdateStream(predicates, observer)
	require.NoError(t, err)
	err = updateDispatcher.GetUpdateStream([]string{"pred1"}, observer2)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		err := updateDispatcher.Update("pred1")
		require.NoError(t, err)
	}

	err = updateDispatcher.Update("pred2")
	require.NoError(t, err)

	require.False(t, waitTimeout(wg, time.Second))

	observer.AssertNotCalled(t, "PredicateUpdated", "pred2")
}

// This test uses many gorutines to that all operations can safely be executed concurrently
func TestSubscriptionDispatcherMultithreading(t *testing.T) {
	predicates := []string{"a", "b", "c", "d"}
	n := 10  // number of observers
	m := 500 // number of iterations where all observers are alive

	var updateDispatcher = NewUpdateDispatcher()

	observers := make([]*mockObserver, n)
	wg := new(sync.WaitGroup)

	test := func(i, n int) {
		wg.Add(1)
		defer wg.Done()
		for ; i < n; i++ {
			observers[i] = NewMockObserver(updateDispatcher, wg)
			k := m + n - i/2
			wg.Add(k) // k x PredicateUpdated
			observers[i].On("isActive").Return(true).Times(k)
			observers[i].On("isActive").Return(false)
			observers[i].On("PredicateUpdated", mock.AnythingOfType("string")).Return().Times(k)
			err := updateDispatcher.GetUpdateStream(predicates, observers[i])
			require.NoError(t, err)
		}
	}

	go test(0, n/2)
	go test(n/2, n)

	for i := 0; i < 2*m; i++ {
		go func() {
			wg.Add(1)
			updateDispatcher.Update("a")
			updateDispatcher.Update("c")
			updateDispatcher.Update("d")
			updateDispatcher.Update("b")
			wg.Done()
		}()
		go func() {
			wg.Add(1)
			defer wg.Done()
			if m%n == 1 {
				k := n / 2
				obs := NewMockObserver(updateDispatcher, wg)
				wg.Add(len(predicates) * k)
				obs.On("isActive").Return(true).Times(len(predicates) * k)
				obs.On("isActive").Return(false)
				for _, pred := range predicates {
					obs.On("PredicateUpdated", pred).Return().Times(k)
				}
				defer obs.AssertExpectations(t)

				updateDispatcher.GetUpdateStream(predicates, obs)
			}
		}()
	}

	require.False(t, waitTimeout(wg, 10*time.Second))
}

// code from: https://stackoverflow.com/a/32843750/3474438
// waitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}
