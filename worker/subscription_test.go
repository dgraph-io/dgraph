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
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockObserver struct {
	mock.Mock
}

func (m *mockObserver) PredicateUpdated(predicate string) {
	m.Called(predicate)
}

func (m *mockObserver) IsActive() bool {
	args := m.Called()
	return args.Bool(0)
}

func TestGetUpdateStream(t *testing.T) {
	predicates := []string{"pred1", "pred2", "pred3"}
	observer := new(mockObserver)
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

	observer.AssertNotCalled(t, "PredicateUpdated", "pred4")
}

func TestMultipleObservers(t *testing.T) {
	predicates1 := []string{"pred1", "pred2"}
	observer1 := new(mockObserver)
	observer1.On("IsActive").Return(true)
	observer1.On("PredicateUpdated", "pred1").Return().Once()
	observer1.On("PredicateUpdated", "pred2").Return().Once()
	defer observer1.AssertExpectations(t)

	predicates2 := []string{"pred3", "pred2"}
	observer2 := new(mockObserver)
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

	observer1.AssertNotCalled(t, "PredicateUpdated", "pred3")
	observer2.AssertNotCalled(t, "PredicateUpdated", "pred1")
}

func TestDeadObserverRemoved(t *testing.T) {
	predicates := []string{"pred1", "pred2"}
	observer := new(mockObserver)
	observer.On("IsActive").Return(true).Twice()
	observer.On("IsActive").Return(false)
	observer.On("PredicateUpdated", "pred1").Return().Twice()
	defer observer.AssertExpectations(t)

	observer2 := new(mockObserver)
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

	observer.AssertNotCalled(t, "PredicateUpdated", "pred2")
}

// This test uses many gorutines to that all operations can safely be executed concurrently
func TestDispatcherMultithreading(t *testing.T) {
	predicates := []string{"a", "b", "c", "d"}
	n := 100  // number of observers
	m := 1000 // number of iterations where all observers are alive

	var updateDispatcher = NewUpdateDispatcher()

	observers := make([]*mockObserver, n)

	test := func(i, n int) {
		for ; i < n; i++ {
			observers[i] = new(mockObserver)
			k := m + n - i/1
			observers[i].On("IsActive").Return(true).Times(k)
			observers[i].On("IsActive").Return(false)
			for _, pred := range predicates {
				observers[i].On("PredicateUpdated", pred).Return().Times(k)
			}
			err := updateDispatcher.GetUpdateStream(predicates, observers[i])
			require.NoError(t, err)
		}
	}

	go test(0, n/2)
	go test(n/2, n)

	for i := 0; i < m; i++ {
		go func() {
			updateDispatcher.Update("a")
			updateDispatcher.Update("c")
			updateDispatcher.Update("d")
			updateDispatcher.Update("b")
		}()
		go func() {
			if m%n == 0 {
				obs := new(mockObserver)
				obs.On("IsActive").Return(true).Times(n)
				obs.On("IsActive").Return(false)
				for _, pred := range predicates {
					obs.On("PredicateUpdated", pred).Return().Times(n)
				}

				updateDispatcher.GetUpdateStream(predicates, obs)
			}
		}()
	}
}
