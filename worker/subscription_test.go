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
	wg *sync.WaitGroup
}

func (m *mockObserver) PredicateUpdated(predicate string) {
	m.Called(predicate)
	m.wg.Done()
}

func (m *mockObserver) IsActive() bool {
	args := m.Called()
	if args.Bool(0) {
		m.wg.Done()
	}
	return args.Bool(0)
}

func TestSubscriptionGetUpdateStream(t *testing.T) {
	predicates := []string{"pred1", "pred2", "pred3"}
	wg := new(sync.WaitGroup)
	observer := &mockObserver{wg: wg}
	wg.Add(6) // 3 x IsActive + 3 x PredicateUpdated
	observer.On("IsActive").Return(true)
	observer.On("PredicateUpdated", "pred1").Return().Once()
	observer.On("PredicateUpdated", "pred2").Return().Twice()
	defer observer.AssertExpectations(t)

	var updateDispatcher = NewUpdateDispatcher()
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

	wg.Wait()
	observer.AssertNotCalled(t, "PredicateUpdated", "pred4")
}

func TestSubscriptionMultipleObservers(t *testing.T) {
	predicates1 := []string{"pred1", "pred2"}
	wg := new(sync.WaitGroup)
	observer1 := &mockObserver{wg: wg}
	wg.Add(4) // 2 x IsActive + 2 x Predicate Updated
	observer1.On("IsActive").Return(true)
	observer1.On("PredicateUpdated", "pred1").Return().Once()
	observer1.On("PredicateUpdated", "pred2").Return().Once()
	defer observer1.AssertExpectations(t)

	predicates2 := []string{"pred3", "pred2"}
	observer2 := &mockObserver{wg: wg}
	wg.Add(4) // 2 x IsActive + 2 x Predicate Updated
	observer2.On("IsActive").Return(true)
	observer2.On("PredicateUpdated", "pred3").Return().Once()
	observer2.On("PredicateUpdated", "pred2").Return().Once()
	defer observer2.AssertExpectations(t)

	var updateDispatcher = NewUpdateDispatcher()
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

	wg.Wait()

	observer1.AssertNotCalled(t, "PredicateUpdated", "pred3")
	observer2.AssertNotCalled(t, "PredicateUpdated", "pred1")
}

func TestSubscriptionDeadObserverRemoved(t *testing.T) {
	predicates := []string{"pred1", "pred2"}
	wg := new(sync.WaitGroup)
	observer := &mockObserver{wg: wg}
	wg.Add(4) // 2 x IsActive + 2 x Predicate Updated
	observer.On("IsActive").Return(true).Twice()
	observer.On("IsActive").Return(false)
	observer.On("PredicateUpdated", "pred1").Return().Twice()
	defer observer.AssertExpectations(t)

	observer2 := &mockObserver{wg: wg}
	wg.Add(10) // 5 x IsActive + 5 x Predicate Updated
	observer2.On("IsActive").Return(true)
	observer2.On("PredicateUpdated", "pred1").Return().Times(5)
	defer observer2.AssertExpectations(t)

	var updateDispatcher = NewUpdateDispatcher()
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

	wg.Wait()

	observer.AssertNotCalled(t, "PredicateUpdated", "pred2")
}

// This test uses many gorutines to that all operations can safely be executed concurrently
func TestSubscriptionDispatcherMultithreading(t *testing.T) {
	predicates := []string{"a", "b", "c", "d"}
	n := 10  // number of observers
	m := 100 // number of iterations where all observers are alive

	var updateDispatcher = NewUpdateDispatcher()

	observers := make([]*mockObserver, n)
	wg := new(sync.WaitGroup)

	test := func(i, n int) {
		for ; i < n; i++ {
			observers[i] = &mockObserver{wg: wg}
			k := m + n - i/2
			wg.Add(2 * k) // k x IsActive + k x PredicateUpdated
			observers[i].On("IsActive").Return(true).Times(k)
			observers[i].On("IsActive").Return(false)
			observers[i].On("PredicateUpdated", mock.AnythingOfType("string")).Return().Times(k)
			err := updateDispatcher.GetUpdateStream(predicates, observers[i])
			require.NoError(t, err)
		}
		wg.Done()
	}

	wg.Add(2)
	go test(0, n/2)
	go test(n/2, n)

	for i := 0; i < 2*m; i++ {
		wg.Add(2)
		go func() {
			updateDispatcher.Update("a")
			updateDispatcher.Update("c")
			updateDispatcher.Update("d")
			updateDispatcher.Update("b")
			wg.Done()
		}()
		go func() {
			if m%n == 1 {
				k := n / 2
				obs := &mockObserver{wg: wg}
				wg.Add(2 * len(predicates) * k)
				obs.On("IsActive").Return(true).Times(len(predicates) * k)
				obs.On("IsActive").Return(false)
				for _, pred := range predicates {
					obs.On("PredicateUpdated", pred).Return().Times(k)
				}
				defer obs.AssertExpectations(t)

				updateDispatcher.GetUpdateStream(predicates, obs)
			}
		}()
	}

	waitTimeout(wg, 5*time.Second)
}

// Ensures that after removing predicate from dispatcher, observers are not called
func TestSubscriptionRemovePredicate(t *testing.T) {
	predicates := []string{"pred1", "pred2", "pred3"}
	wg := new(sync.WaitGroup)
	observer := &mockObserver{wg: wg}
	wg.Add(6) // 2 x IsActive + 2 x PredicateUpdated
	observer.On("IsActive").Return(true)
	observer.On("PredicateUpdated", "pred1").Return().Twice()
	observer.On("PredicateUpdated", "pred2").Return().Once()
	defer observer.AssertExpectations(t)

	var updateDispatcher = NewUpdateDispatcher()
	err := updateDispatcher.GetUpdateStream(predicates, observer)
	require.NoError(t, err)

	err = updateDispatcher.Update("pred1")
	require.NoError(t, err)
	err = updateDispatcher.Update("pred2")
	require.NoError(t, err)

	updateDispatcher.RemovePredicate("pred2")

	err = updateDispatcher.Update("pred4")
	require.NoError(t, err)

	err = updateDispatcher.Update("pred1")
	require.NoError(t, err)
	err = updateDispatcher.Update("pred2")
	require.NoError(t, err)

	waitTimeout(wg, time.Second)
	observer.AssertNotCalled(t, "PredicateUpdated", "pred4")
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
