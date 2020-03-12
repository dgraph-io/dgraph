/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package subscription

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgryski/go-farm"
)

// Poller is used to poll user subscription query.
type Poller struct {
	sync.Mutex
	resolver       *resolve.RequestResolver
	pollRegistry   map[uint64]map[uint64]chan interface{}
	subscriptionID uint64
	globalEpoch    *uint64
}

// NewPoller returns Poller.
func NewPoller(globalEpoch *uint64, resolver *resolve.RequestResolver) *Poller {
	return &Poller{
		resolver:     resolver,
		pollRegistry: make(map[uint64]map[uint64]chan interface{}),
		globalEpoch:  globalEpoch,
	}
}

// SubscriberResponse holds the meta data about subscriber.
type SubscriberResponse struct {
	BucketID       uint64
	SubscriptionID uint64
	UpdateCh       chan interface{}
}

// AddSubscriber try to add subscription into to existing polling go routine. If it's exist.
// If not it's create new polling go rotuine for the given request.
func (p *Poller) AddSubscriber(req *schema.Request) (*SubscriberResponse, error) {
	bucketID := farm.Fingerprint64([]byte(req.Query))
	p.Lock()
	defer p.Unlock()

	res := p.resolver.Resolve(context.TODO(), req)
	if len(res.Errors) != 0 {
		return nil, res.Errors
	}

	prevHash := farm.Fingerprint64(res.Data.Bytes())

	updateCh := make(chan interface{}, 10)
	updateCh <- res.Output()

	subscriptionID := p.subscriptionID
	// Increment ID for next subscription.
	p.subscriptionID++
	subscriptions, ok := p.pollRegistry[bucketID]

	if !ok {
		subscriptions = make(map[uint64]chan interface{})
	}

	subscriptions[subscriptionID] = updateCh
	p.pollRegistry[bucketID] = subscriptions

	if len(subscriptions) != 1 {
		// Already there is subscription for this bucket. So,no need to poll the server. We can
		// use the existing polling routine to publish the update.

		return &SubscriberResponse{
			BucketID:       bucketID,
			SubscriptionID: subscriptionID,
			UpdateCh:       updateCh,
		}, nil
	}

	localEpoch := atomic.LoadUint64(p.globalEpoch)
	resolver := p.resolver
	// There is no go rountine running to poll the server. So, run one to publish the updates.
	go func() {
		pollID := uint64(0)
		for {
			pollID++
			time.Sleep(time.Second)

			globalEpoch := atomic.LoadUint64(p.globalEpoch)
			if localEpoch != globalEpoch || globalEpoch == math.MaxUint64 {
				// There is a schema change since local epoch is diffrent from global schema epoch.
				// We'll terminate all the subscription for this bucket. So, that all client can
				// reconnect and listen for new schema.
				p.TerminateSusbcriptions(bucketID)
				return
			}

			res := resolver.Resolve(context.TODO(), req)

			currentHash := farm.Fingerprint64(res.Data.Bytes())

			if prevHash == currentHash {
				if pollID%30 != 0 {
					// Don't update if there is no change in response.
					continue
				}
				// Every thirty poll. We'll check there is any active subscription for the
				// current poll. If not we'll terminate this poll.
				p.Lock()
				subscribers, ok := p.pollRegistry[bucketID]
				if !ok || len(subscribers) == 0 {
					p.Unlock()
					return
				}
				p.Unlock()
				continue
			}
			prevHash = currentHash

			p.Lock()
			subscribers, ok := p.pollRegistry[bucketID]
			if !ok || len(subscribers) == 0 {
				// There is no subscribers to push the update. So, kill the current polling
				// go routine.
				p.Unlock()
				return
			}
			for _, updateCh := range subscribers {
				updateCh <- res.Output()
			}
			p.Unlock()
		}
	}()
	return &SubscriberResponse{
		BucketID:       bucketID,
		SubscriptionID: subscriptionID,
		UpdateCh:       updateCh,
	}, nil
}

// UpdateResolver will update the resolver.
func (p *Poller) UpdateResolver(resolver *resolve.RequestResolver) {
	p.Lock()
	defer p.Unlock()
	p.resolver = resolver
}

// TerminateSusbcriptions will terminate all the subscriptions of the given bucketID.
func (p *Poller) TerminateSusbcriptions(bucketID uint64) {
	p.Lock()
	defer p.Unlock()
	subscriptions, ok := p.pollRegistry[bucketID]
	if !ok {
		return
	}
	for _, updateCh := range subscriptions {
		// Closing the channel will close the graphQL websocket connection as well.
		close(updateCh)
	}
	p.pollRegistry[bucketID] = make(map[uint64]chan interface{})
}

// TerminateSubscription will terminate the polling subscription.
func (p *Poller) TerminateSubscription(bucketID, subscriptionID uint64) {
	p.Lock()
	defer p.Unlock()
	subscriptions, ok := p.pollRegistry[bucketID]
	if !ok {
		return
	}
	updateCh, ok := subscriptions[subscriptionID]
	if ok {
		close(updateCh)
	}
	delete(subscriptions, subscriptionID)
	p.pollRegistry[bucketID] = subscriptions
}
