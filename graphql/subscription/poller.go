/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgrijalva/jwt-go/v4"
	"github.com/dgryski/go-farm"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

// Poller is used to poll user subscription query.
type Poller struct {
	sync.RWMutex
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
func (p *Poller) AddSubscriber(req *schema.Request) (*SubscriberResponse, error) {
	p.RLock()
	resolver := p.resolver
	p.RUnlock()

	localEpoch := atomic.LoadUint64(p.globalEpoch)
	if err := resolver.ValidateSubscription(req); err != nil {
		return nil, err
	}

	// find out the custom claims for auth, if any. As,
	// We also need to use authVariables in generating the hashed bucketID
	authMeta := resolver.Schema().Meta().AuthMeta()
	ctx, err := authMeta.AttachAuthorizationJwt(context.Background(), req.Header)
	if err != nil {
		return nil, err
	}
	customClaims, err := authMeta.ExtractCustomClaims(ctx)
	if err != nil {
		return nil, err
	}
	// for the cases when no expiry is given in jwt or subscription doesn't have any authorization,
	// we set their expiry to zero time
	if customClaims.StandardClaims.ExpiresAt == nil {
		customClaims.StandardClaims.ExpiresAt = jwt.At(time.Time{})
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

	res := resolver.Resolve(x.AttachAccessJwt(context.Background(),
		&http.Request{Header: req.Header}), req)
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

	if ok {
		// Already there is a running go routine for this bucket. So,no need to poll the server.
		// We can use the existing polling routine to publish the update.
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
	p.RLock()
	resolver := p.resolver
	p.RUnlock()

	pollID := uint64(0)
	for {
		pollID++
		time.Sleep(x.Config.GraphQL.GetDuration("poll-interval"))

		globalEpoch := atomic.LoadUint64(p.globalEpoch)
		if req.localEpoch != globalEpoch || globalEpoch == math.MaxUint64 {
			// There is a schema change since local epoch is diffrent from global schema epoch.
			// We'll terminate all the subscription for this bucket. So, that all client can
			// reconnect and listen for new schema.
			p.terminateSubscriptions(req.bucketID)
		}

		ctx := x.AttachAccessJwt(context.Background(), &http.Request{Header: req.graphqlReq.Header})
		res := resolver.Resolve(ctx, req.graphqlReq)

		currentHash := farm.Fingerprint64(res.Data.Bytes())

		if req.prevHash == currentHash {
			if pollID%2 != 0 {
				// Don't update if there is no change in response.
				continue
			}
			// Every second poll, we'll check if there is any active subscription for the
			// current goroutine. If not we'll terminate this poll.
			p.Lock()
			subscribers, ok := p.pollRegistry[req.bucketID]
			if !ok || len(subscribers) == 0 {
				delete(p.pollRegistry, req.bucketID)
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
			delete(p.pollRegistry, req.bucketID)
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
