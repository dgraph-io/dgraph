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

package pubsub

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"golang.org/x/net/context"
)

func TestSubscriptionCorrectness(t *testing.T) {
	hub := NewUpdateHub()
	go hub.Run()
	require.Equal(t, false, hub.HasSubscribers())

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	subscriber1 := NewTestSubscriber(ctx1)
	subscriber2 := NewTestSubscriber(ctx2)
	go subscriber1.Run()
	go subscriber2.Run()
	hub.Subscribe([]string{"a", "b"}, subscriber1)
	hub.Subscribe([]string{"b", "c"}, subscriber2)
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, true, hub.HasSubscribers())

	// notify both subscribers
	hub.PredicatesUpdated([]string{"a", "c"})
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, true, hub.HasSubscribers())
	require.Equal(t, 1, subscriber1.cnt)
	require.Equal(t, 1, subscriber2.cnt)

	// only one notification per batch update
	hub.PredicatesUpdated([]string{"a", "a", "a"})
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, true, hub.HasSubscribers())
	require.Equal(t, 2, subscriber1.cnt)
	require.Equal(t, 1, subscriber2.cnt)

	// notify only about interesting updates
	hub.PredicatesUpdated([]string{"d"})
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, true, hub.HasSubscribers())
	require.Equal(t, 2, subscriber1.cnt)
	require.Equal(t, 1, subscriber2.cnt)

	// remove one subscriber, notify another
	cancel1()
	hub.PredicatesUpdated([]string{"a", "b", "c", "d"})
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, true, hub.HasSubscribers())
	require.Equal(t, 2, subscriber1.cnt)
	require.Equal(t, 2, subscriber2.cnt)

	// remove second subscriber
	cancel2()
	hub.PredicatesUpdated([]string{"a", "b", "c", "d"})
	time.Sleep(10 * time.Millisecond)
	require.Equal(t, false, hub.HasSubscribers())
	require.Equal(t, 2, subscriber1.cnt)
	require.Equal(t, 2, subscriber2.cnt)

}

func TestSubscriptionThreadSafety(t *testing.T) {
	hub := NewUpdateHub()
	go hub.Run()
	require.Equal(t, false, hub.HasSubscribers())

	var subscribers []*TestSubscriber
	var cancel []context.CancelFunc
	for i := 0; i < 100; i++ {
		ctx1, cancel1 := context.WithCancel(context.Background())
		ctx2, cancel2 := context.WithCancel(context.Background())
		subscriber1 := NewTestSubscriber(ctx1)
		subscriber2 := NewTestSubscriber(ctx2)
		subscribers = append(subscribers, subscriber1, subscriber2)
		cancel = append(cancel, cancel1, cancel2)
		go subscriber1.Run()
		go subscriber2.Run()
		hub.Subscribe([]string{"a", "b"}, subscriber1)
		hub.Subscribe([]string{"b", "c"}, subscriber2)
	}

	done := make(chan bool)
	go func() {
		for i, f := range cancel {
			time.Sleep(25 * time.Millisecond)
			f()
			require.True(t, subscribers[i].cnt > 0)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 1000; i++ {
			fmt.Println("-----")
			time.Sleep(5 * time.Millisecond)
			hub.PredicatesUpdated([]string{"a", "c"})
			hub.PredicatesUpdated([]string{"b"})
			hub.PredicatesUpdated([]string{"d"})
		}
	}()
	require.Equal(t, true, hub.HasSubscribers())

	select {
	case _ = <-done:
	}
	hub.PredicatesUpdated([]string{"a", "b", "c", "d"})
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, false, hub.HasSubscribers())
}

type TestSubscriber struct {
	basicSubscriber
	ctx context.Context
	cnt int
}

func NewTestSubscriber(ctx context.Context) *TestSubscriber {
	return &TestSubscriber{basicSubscriber{false, make(chan bool)}, ctx, 0}
}

func (s *TestSubscriber) Run() {
	for {
		select {
		case <-s.updatesChan:
			s.cnt++
		}
	}
}

func (s *TestSubscriber) Context() context.Context {
	return s.ctx
}

func (s *TestSubscriber) NeedsUpdate() bool {
	return s.needsUpdate
}

func (s *TestSubscriber) RequireUpdate(update bool) {
	s.needsUpdate = update
}

func (s *TestSubscriber) UpdatesChan() chan bool {
	return s.updatesChan
}
