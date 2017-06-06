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

	"golang.org/x/net/context"
)

// topic groups all subscribers interested in single predicate
type topic struct {
	subscribers []*UpdateSubscriber
}

type UpdateSubscriber struct {
	ctx               context.Context
	predicatesChannel chan string // TODO(tzdybal) - this also should be []string
}

type subscription struct {
	predicates []string
	subscriber *UpdateSubscriber
}

// one channel per subscriber
type UpdateHub struct {
	updates       chan []string
	subscriptions chan subscription
	topics        map[string]*topic
}

func NewUpdateHub() *UpdateHub {
	hub := &UpdateHub{
		updates:       make(chan []string, 100),
		subscriptions: make(chan subscription, 100),
		topics:        make(map[string]*topic),
	}
	return hub
}

func (d *UpdateHub) Run() {
	for {
		select {
		case preds := <-d.updates:
			d.doUpdate(preds)
		case sub := <-d.subscriptions:
			d.doSubscribe(sub)
		}
	}
}

func (d *UpdateHub) Subscribe(predicates []string, subscriber *UpdateSubscriber) {
	d.subscriptions <- subscription{predicates, subscriber}
}

func (d *UpdateHub) doSubscribe(sub subscription) {
	for _, pred := range sub.predicates {
		topic, ok := d.topics[pred]
		if !ok {
			topic = newTopic()
			d.topics[pred] = topic
		}
		topic.subscribers = append(topic.subscribers, sub.subscriber)
	}
}

func (d *UpdateHub) PredicatesUpdated(predicates []string) {
	d.updates <- predicates
}

func (d *UpdateHub) doUpdate(predicates []string) {
	for _, pred := range predicates {
		topic, ok := d.topics[pred]
		if ok {
			topic.predicateUpdated(pred)
		}
	}
}

func newTopic() *topic {
	return &topic{subscribers: make([]*UpdateSubscriber, 0, 10)}
}

func (t *topic) predicateUpdated(predicate string) {
	subs := t.subscribers
	last := len(subs)
	for i := 0; i < last; i++ {
		sub := subs[i]
		if sub.ctx.Err() == nil {
			sub.predicatesChannel <- predicate
		} else {
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

func NewUpdateSubscriber(ctx context.Context) *UpdateSubscriber {
	return &UpdateSubscriber{ctx, make(chan string)}
}

func (s *UpdateSubscriber) Run() {
	for {
		select {
		case pred, more := <-s.predicatesChannel:
			if more {
				fmt.Println("tzdybal:", pred)
			} else {
				fmt.Println("tzdybal: channel closed")
				break
			}
		}
	}
}
