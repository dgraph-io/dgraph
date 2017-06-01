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
	//	"github.com/stretchr/testify/require"
)

func TestSubscription(t *testing.T) {
	subscriber1 := NewUpdateSubscriber()
	subscriber2 := NewUpdateSubscriber()
	go subscriber1.Run()
	go subscriber2.Run()

	dispatcher := NewUpdateDispatcher()
	go dispatcher.Run()

	for i := 0; i < 1000; i++ {
		dispatcher.Subscribe([]string{"a", "b"}, subscriber1)
		dispatcher.Subscribe([]string{"b", "c"}, subscriber2)

		dispatcher.PredicateUpdated("a")
		dispatcher.PredicateUpdated("b")
		dispatcher.PredicateUpdated("c")
		dispatcher.PredicateUpdated("d")

		fmt.Println("-------")
		dispatcher.Unsubscribe([]string{"b"}, subscriber1)

		dispatcher.PredicateUpdated("a")
		dispatcher.PredicateUpdated("b")
		dispatcher.PredicateUpdated("c")
		dispatcher.PredicateUpdated("d")

		fmt.Println("-------")
		dispatcher.Unsubscribe([]string{"b"}, subscriber2)
		dispatcher.Unsubscribe([]string{"a", "b", "c"}, subscriber2)

		dispatcher.PredicateUpdated("a")
		dispatcher.PredicateUpdated("b")
		dispatcher.PredicateUpdated("c")
		dispatcher.PredicateUpdated("d")
	}
}
