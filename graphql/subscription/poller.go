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
	"encoding/json"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
	"github.com/golang/glog"
)

// Poller is used to poll user subscription query.
type Poller struct {
	sync.Mutex
	resolver       *resolve.RequestResolver
	pollRegistry   map[uint64]map[uint64]subscriber
	subscriptionID uint64
	globalEpoch    *uint64
}

// NewPoller returns Poller.
func NewPoller(globalEpoch *uint64, resolver *resolve.RequestResolver) *Poller {
	return &Poller{
		resolver:     resolver,
		pollRegistry: make(map[uint64]map[uint64]subscriber),
		globalEpoch:  globalEpoch,
	}
}

// SubscriberResponse holds the meta data about subscriber.
type SubscriberResponse struct {
	BucketID       uint64
	SubscriptionID uint64
	UpdateCh       chan interface{}
}

type subscriber struct {
	expiry   time.Time
	updateCh chan interface{}
}

// AddSubscriber tries to add subscription into the existing polling goroutine if it exists.
// If it doesn't exist, then it creates a new polling goroutine for the given request.
func (p *Poller) AddSubscriber(
	req *schema.Request, customClaims *authorization.CustomClaims) (*SubscriberResponse, error) {

	localEpoch := atomic.LoadUint64(p.globalEpoch)
	err := p.resolver.ValidateSubscription(req)
	if err != nil {
		return nil, err
	}

	buf, err := json.Marshal(req)
	x.Check(err)
	var bucketID uint64
	if customClaims.AuthVariables != nil {

		// TODO - Add custom marshal function that marshal's the json in sorted order.
		authvariables, err := json.Marshal(customClaims.AuthVariables)
		if err != nil {
			return nil, err
		}
		bucketID = farm.Fingerprint64(append(buf, authvariables...))
	} else {
		bucketID = farm.Fingerprint64(buf)
	}
	p.Lock()
	defer p.Unlock()

	ctx := context.WithValue(context.Background(), authorization.AuthVariables, customClaims.AuthVariables)
	res := p.resolver.Resolve(ctx, req)
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
		subscriptions = make(map[uint64]subscriber)
	}
	glog.Infof("Subscription polling is started for the ID %d", subscriptionID)

	subscriptions[subscriptionID] = subscriber{
		expiry: customClaims.StandardClaims.ExpiresAt.Time, updateCh: updateCh}
	p.pollRegistry[bucketID] = subscriptions

	if len(subscriptions) != 1 {
		// Already there is subscription for this bucket. So,no need to poll the server. We can
		// use the existing polling routine to publish the update.

		return &SubscriberResponse{
			BucketID:       bucketID,
			SubscriptionID: subscriptionID,
			UpdateCh:       subscriptions[subscriptionID].updateCh,
		}, nil
	}

	// There is no goroutine running to check updates for this query. So, run one to publish
	// the updates.
	pollR := &pollRequest{
		bucketID:      bucketID,
		prevHash:      prevHash,
		graphqlReq:    req,
		authVariables: customClaims.AuthVariables,
		localEpoch:    localEpoch,
	}
	go p.poll(pollR)

	return &SubscriberResponse{
		BucketID:       bucketID,
		SubscriptionID: subscriptionID,
		UpdateCh:       subscriptions[subscriptionID].updateCh,
	}, nil
}

type pollRequest struct {
	prevHash      uint64
	graphqlReq    *schema.Request
	bucketID      uint64
	localEpoch    uint64
	authVariables map[string]interface{}
}

func (p *Poller) poll(req *pollRequest) {
	resolver := p.resolver
	pollID := uint64(0)
	for {
		pollID++
		time.Sleep(x.Config.PollInterval)

		globalEpoch := atomic.LoadUint64(p.globalEpoch)
		if req.localEpoch != globalEpoch || globalEpoch == math.MaxUint64 {
			// There is a schema change since local epoch is diffrent from global schema epoch.
			// We'll terminate all the subscription for this bucket. So, that all client can
			// reconnect and listen for new schema.
			p.terminateSubscriptions(req.bucketID)
		}

		ctx := context.WithValue(context.Background(), authorization.AuthVariables, req.authVariables)
		res := resolver.Resolve(ctx, req.graphqlReq)

		currentHash := farm.Fingerprint64(res.Data.Bytes())

		if req.prevHash == currentHash {
			if pollID%2 != 0 {
				// Don't update if there is no change in response.
				continue
			}
			// Every thirty poll. We'll check there is any active subscription for the
			// current poll. If not we'll terminate this poll.
			p.Lock()
			subscribers, ok := p.pollRegistry[req.bucketID]
			if !ok || len(subscribers) == 0 {
				p.Unlock()
				return
			}
			for subscriberID, subscriber := range subscribers {
				if !subscriber.expiry.IsZero() && time.Now().After(subscriber.expiry) {
					p.terminateSubscription(req.bucketID, subscriberID)
				}

			}
			p.Unlock()
			continue
		}
		req.prevHash = currentHash

		p.Lock()
		subscribers, ok := p.pollRegistry[req.bucketID]
		if !ok || len(subscribers) == 0 {
			// There is no subscribers to push the update. So, kill the current polling
			// go routine.
			p.Unlock()
			return
		}

		for subscriberID, subscriber := range subscribers {
			if !subscriber.expiry.IsZero() && time.Now().After(subscriber.expiry) {
				p.terminateSubscription(req.bucketID, subscriberID)
			}

		}
		for _, subscriber := range subscribers {
			subscriber.updateCh <- res.Output()
		}
		p.Unlock()
	}
}

// UpdateResolver will update the resolver.
func (p *Poller) UpdateResolver(resolver *resolve.RequestResolver) {
	p.Lock()
	defer p.Unlock()
	p.resolver = resolver
}

// TerminateSubscriptions will terminate all the subscriptions of the given bucketID.
func (p *Poller) terminateSubscriptions(bucketID uint64) {
	p.Lock()
	defer p.Unlock()
	subscriptions, ok := p.pollRegistry[bucketID]
	if !ok {
		return
	}
	for _, subscriber := range subscriptions {
		// Closing the channel will close the graphQL websocket connection as well.
		close(subscriber.updateCh)
	}
	delete(p.pollRegistry, bucketID)
}

func (p *Poller) TerminateSubscription(bucketID, subscriptionID uint64) {
	p.Lock()
	defer p.Unlock()
	p.terminateSubscription(bucketID, subscriptionID)
}

func (p *Poller) terminateSubscription(bucketID, subscriptionID uint64) {
	subscriptions, ok := p.pollRegistry[bucketID]
	if !ok {
		return
	}
	subscriber, ok := subscriptions[subscriptionID]
	if ok {
		glog.Infof("Terminating subscription for the subscription ID %d", subscriptionID)
		close(subscriber.updateCh)

	}
	delete(subscriptions, subscriptionID)
	p.pollRegistry[bucketID] = subscriptions
}
