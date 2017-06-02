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
)

type topic struct {
	subscribers map[*UpdateSubscriber]bool
}

type UpdateSubscriber struct {
	predicatesChannel chan string
}

type OperationType int

const (
	subscribe   OperationType = iota
	unsubscribe OperationType = iota
)

type Operation struct {
	opType     OperationType
	predicates []string
	subscriber *UpdateSubscriber
}

// one channel per subscriber
type UpdateDispatcher struct {
	updates    chan []string
	operations chan Operation
	topics     map[string]*topic
}

func NewUpdateDispatcher() *UpdateDispatcher {
	dispatcher := &UpdateDispatcher{
		updates:    make(chan []string, 100),
		operations: make(chan Operation),
		topics:     make(map[string]*topic),
	}
	return dispatcher
}

func (d *UpdateDispatcher) Run() {
	for {
		select {
		case oper := <-d.operations:
			d.execute(oper)
		case preds := <-d.updates:
			d.doUpdate(preds)
		}
	}
}

func (d *UpdateDispatcher) Subscribe(predicates []string, subscriber *UpdateSubscriber) {
	operation := Operation{opType: subscribe, predicates: predicates, subscriber: subscriber}
	d.operations <- operation
}

func (d *UpdateDispatcher) Unsubscribe(predicates []string, subscriber *UpdateSubscriber) {
	operation := Operation{opType: unsubscribe, predicates: predicates, subscriber: subscriber}
	d.operations <- operation
}

func (d *UpdateDispatcher) PredicatesUpdated(predicates []string) {
	d.updates <- predicates
}

func (d *UpdateDispatcher) execute(operation Operation) {
	switch operation.opType {
	case subscribe:
		d.doSubscribe(operation)
	case unsubscribe:
		d.doUnsubscribe(operation)
	}
}

func (d *UpdateDispatcher) doSubscribe(operation Operation) {
	for _, pred := range operation.predicates {
		topic, ok := d.topics[pred]
		if !ok {
			topic = newTopic()
			d.topics[pred] = topic
		}
		topic.subscribe(operation.subscriber)
	}
}

func (d *UpdateDispatcher) doUnsubscribe(operation Operation) {
	for _, pred := range operation.predicates {
		topic, ok := d.topics[pred]
		if ok {
			topic.unsubscribe(operation.subscriber)
			if topic.size() == 0 {
				delete(d.topics, pred)
			}
		}
	}
}

func (d *UpdateDispatcher) doUpdate(predicates []string) {
	for _, pred := range predicates {
		topic, ok := d.topics[pred]
		if ok {
			topic.predicateUpdated(pred)
		}
	}
}

func newTopic() *topic {
	return &topic{subscribers: make(map[*UpdateSubscriber]bool)}
}

func (t *topic) subscribe(subscriber *UpdateSubscriber) {
	t.subscribers[subscriber] = true
}

func (t *topic) size() int {
	return len(t.subscribers)
}

func (t *topic) unsubscribe(subscriber *UpdateSubscriber) {
	delete(t.subscribers, subscriber)
}

func (t *topic) predicateUpdated(predicate string) {
	for sub, _ := range t.subscribers {
		sub.predicatesChannel <- predicate
	}
}

func NewUpdateSubscriber() *UpdateSubscriber {
	return &UpdateSubscriber{predicatesChannel: make(chan string)}
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
