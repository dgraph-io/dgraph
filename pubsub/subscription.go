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
	"sort"
	"sync/atomic"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/protos"
)

// topic groups all subscribers interested in single predicate
type topic struct {
	subscribers []UpdateSubscriber
}

type UpdateSubscriber interface {
	NeedsUpdate() bool
	RequireUpdate(bool)
	UpdatesChan() chan bool
	Context() context.Context
}

type basicSubscriber struct {
	needsUpdate bool
	updatesChan chan bool
}

type NetworkSubscriber struct {
	basicSubscriber
	server protos.Worker_SubscribeServer
}

type LocalSubscriber struct {
	basicSubscriber
	ctx context.Context
}

type subscription struct {
	predicates []string
	subscriber UpdateSubscriber
}

// one channel per subscriber
type UpdateHub struct {
	updates       chan []string
	subscriptions chan subscription
	topics        map[string]*topic
	count         int64
}

func NewUpdateHub() *UpdateHub {
	hub := &UpdateHub{
		updates:       make(chan []string, 100),
		subscriptions: make(chan subscription, 100),
		topics:        make(map[string]*topic),
		count:         0,
	}
	return hub
}

func (h *UpdateHub) Run() {
	for {
		select {
		case preds := <-h.updates:
			h.doUpdate(preds)
		case sub := <-h.subscriptions:
			h.doSubscribe(sub)
		}
	}
}

func (h *UpdateHub) HasSubscribers() bool {
	cnt := atomic.LoadInt64(&h.count)
	fmt.Println("tzdybal:", cnt)
	return cnt != 0
}

func (h *UpdateHub) Subscribe(predicates []string, subscriber UpdateSubscriber) {
	h.subscriptions <- subscription{predicates, subscriber}
}

func (h *UpdateHub) doSubscribe(sub subscription) {
	for _, pred := range sub.predicates {
		topic, ok := h.topics[pred]
		if !ok {
			topic = newTopic()
			h.topics[pred] = topic
		}
		topic.subscribers = append(topic.subscribers, sub.subscriber)
		atomic.AddInt64(&h.count, 1)
	}
}

func (h *UpdateHub) PredicatesUpdated(predicates []string) {
	// sorting is time consuming, so do it in parallel
	go func() {
		sort.Strings(predicates)
		h.updates <- predicates
	}()
}

func (h *UpdateHub) doUpdate(predicates []string) {
	var updates []string

	fmt.Println("tzdybal: doUpdate(", predicates, ")")

	// find all subscribers that needs to be updated
	for pred, topic := range h.topics {
		i := sort.SearchStrings(predicates, pred)
		if i < len(predicates) && predicates[i] == pred {
			topic.requireUpdate()
			updates = append(updates, pred)
		}
	}

	fmt.Println("tzdybal: doUpdate:updates:", updates, ")")

	// notify subscribers about update - each subscrier is notified exaclty once
	for _, pred := range updates {
		topic := h.topics[pred]
		before := len(topic.subscribers)
		topic.predicateUpdated()
		after := len(topic.subscribers)
		atomic.AddInt64(&h.count, int64(after-before))
	}
}

func newTopic() *topic {
	return &topic{subscribers: make([]UpdateSubscriber, 0, 10)}
}

func (t *topic) requireUpdate() {
	for _, sub := range t.subscribers {
		if sub.NeedsUpdate() == false {
			sub.RequireUpdate(true)
		}
	}
}

func (t *topic) predicateUpdated() {
	subs := t.subscribers
	last := len(subs)
	for i := 0; i < last; i++ {
		sub := subs[i]
		if sub.Context().Err() == nil {
			fmt.Println("tzdybal: notifying!")
			if sub.NeedsUpdate() {
				sub.UpdatesChan() <- true
				sub.RequireUpdate(false)
			}
		} else {
			fmt.Println("tzdybal: removing!")
			// remove by swapping with last element and decreasing size
			subs[i] = subs[last-1]
			subs[last-1] = nil
			last--
		}
	}
	// trim to remove unsed elements
	subs = subs[:last]
	t.subscribers = subs
}

func NewNetworkSubscriber(server protos.Worker_SubscribeServer) NetworkSubscriber {
	return NetworkSubscriber{basicSubscriber{false, make(chan bool)}, server}
}

func (s *NetworkSubscriber) Run() {
	for {
		select {
		case <-s.updatesChan:
			fmt.Println("tzdybal: subscriber notified!")
			s.server.Send(&protos.PredicateUpdate{})
		}
	}
}

func (s NetworkSubscriber) Context() context.Context {
	return s.server.Context()
}

func (s NetworkSubscriber) NeedsUpdate() bool {
	return s.needsUpdate
}

func (s NetworkSubscriber) RequireUpdate(update bool) {
	s.needsUpdate = update
}

func (s NetworkSubscriber) UpdatesChan() chan bool {
	return s.updatesChan
}

func NewLocalSubscriber(ctx context.Context) LocalSubscriber {
	return LocalSubscriber{basicSubscriber{false, make(chan bool)}, ctx}
}

func (s *LocalSubscriber) Run() {
	for {
		fmt.Println("tzdybal: waiting for notification...")
		select {
		case <-s.updatesChan:
			fmt.Println("tzdybal: subscriber notified!")
		}
	}
}

func (s LocalSubscriber) Context() context.Context {
	return s.ctx
}

func (s LocalSubscriber) NeedsUpdate() bool {
	return s.needsUpdate
}

func (s LocalSubscriber) RequireUpdate(update bool) {
	s.needsUpdate = update
}

func (s LocalSubscriber) UpdatesChan() chan bool {
	return s.updatesChan
}
