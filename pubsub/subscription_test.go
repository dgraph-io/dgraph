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
	"testing"
	"time"
	//	"github.com/stretchr/testify/require"

	"golang.org/x/net/context"
)

func TestSubscription(t *testing.T) {

	hub := NewUpdateHub()
	go hub.Run()
	fmt.Println("tzdybal: hub started!")

	var cancel []context.CancelFunc
	var ctxs []context.Context
	for i := 0; i < 100; i++ {
		ctx1, cancel1 := context.WithCancel(context.Background())
		ctx2, cancel2 := context.WithCancel(context.Background())
		subscriber1 := NewUpdateSubscriber(ctx1)
		subscriber2 := NewUpdateSubscriber(ctx2)
		ctxs = append(ctxs, ctx1, ctx2)
		cancel = append(cancel, cancel1, cancel2)
		go subscriber1.Run()
		go subscriber2.Run()
		hub.Subscribe([]string{"a", "b"}, subscriber1)
		hub.Subscribe([]string{"b", "c"}, subscriber2)
	}

	done := make(chan bool)
	go func() {
		for _, f := range cancel {
			time.Sleep(25 * time.Millisecond)
			f()
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

	select {
	case _ = <-done:
	}
}
