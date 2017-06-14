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
	"fmt"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/pubsub"
	"github.com/dgraph-io/dgraph/x"
)

func (w *grpcWorker) Subscribe(req *protos.SubscribeRequest, server protos.Worker_SubscribeServer) error {
	fmt.Println("tzdybal: Subscribing to predicates: ", req.Predicates)
	subscriber := pubsub.NewNetworkSubscriber(server)
	go subscriber.Run()
	groups().dispatcher.Subscribe(req.Predicates, subscriber)

	return nil
}

func SubscribeOverNetwork(ctx context.Context, attrs []string) error {
	attrMap := make(map[uint32][]string)

	for _, attr := range attrs {
		gid := group.BelongsTo(attr)
		attrMap[gid] = append(attrMap[gid], attr)
	}

	fmt.Println("tzdybal: ", attrMap)

	for gid, attr := range attrMap {
		if groups().ServesGroup(gid) {
			// No need for a network call, as this should be run from within this instance.
			fmt.Println("tzdybal: Subscribing to predicates: ", attrs)
			subscriber := pubsub.NewLocalSubscriber(context.Background())
			go subscriber.Run()
			groups().dispatcher.Subscribe(attrs, subscriber)
			return nil
		}

		// Send this over the network.
		// TODO: Send the request to multiple servers as described in Jeff Dean's talk.
		addr := groups().AnyServer(gid)
		pl := pools().get(addr)

		conn, err := pl.Get()
		if err != nil {
			return x.Wrapf(err, "SubscribeOverNetwork: while retrieving connection.")
		}
		defer pl.Put(conn)
		x.Trace(ctx, "Sending request to %v", addr)

		fmt.Println("tzdybal: sending Subscribe request (attrs: ", attrs, ")")
		c := protos.NewWorkerClient(conn)
		client, err := c.Subscribe(ctx, &protos.SubscribeRequest{attr})
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while calling Worker.Subscribe"))
			return err
		}

		for {
			_, err := client.Recv()

			if err != nil {
				return x.Wrapf(err, "SubscribeOverNetwork: while receiving message.")

			}
			fmt.Println("tzdybal: predicate updated!")
		}
	}
	return nil
}
