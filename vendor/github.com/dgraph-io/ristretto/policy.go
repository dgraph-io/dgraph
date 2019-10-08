/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package ristretto

import (
	"math"
	"sync"

	"github.com/dgraph-io/ristretto/z"
)

const (
	// lfuSample is the number of items to sample when looking at eviction
	// candidates. 5 seems to be the most optimal number [citation needed].
	lfuSample = 5
)

// policy is the interface encapsulating eviction/admission behavior.
//
// TODO: remove this interface and just rename defaultPolicy to policy, as we
//       are probably only going to use/implement/maintain one policy.
type policy interface {
	ringConsumer
	// Add attempts to Add the key-cost pair to the Policy. It returns a slice
	// of evicted keys and a bool denoting whether or not the key-cost pair
	// was added. If it returns true, the key should be stored in cache.
	Add(uint64, int64) ([]*item, bool)
	// Has returns true if the key exists in the Policy.
	Has(uint64) bool
	// Del deletes the key from the Policy.
	Del(uint64)
	// Cap returns the available capacity.
	Cap() int64
	// Close stops all goroutines and closes all channels.
	Close()
	// Update updates the cost value for the key.
	Update(uint64, int64)
	// Cost returns the cost value of a key or -1 if missing.
	Cost(uint64) int64
	// Optionally, set stats object to track how policy is performing.
	CollectMetrics(stats *metrics)
	// Clear zeroes out all counters and clears hashmaps.
	Clear()
}

func newPolicy(numCounters, maxCost int64) policy {
	return newDefaultPolicy(numCounters, maxCost)
}

type defaultPolicy struct {
	sync.Mutex
	admit   *tinyLFU
	evict   *sampledLFU
	itemsCh chan []uint64
	stop    chan struct{}
	stats   *metrics
}

func newDefaultPolicy(numCounters, maxCost int64) *defaultPolicy {
	p := &defaultPolicy{
		admit:   newTinyLFU(numCounters),
		evict:   newSampledLFU(maxCost),
		itemsCh: make(chan []uint64, 3),
		stop:    make(chan struct{}),
	}
	go p.processItems()
	return p
}

func (p *defaultPolicy) CollectMetrics(stats *metrics) {
	p.stats = stats
	p.evict.stats = stats
}

type policyPair struct {
	key  uint64
	cost int64
}

func (p *defaultPolicy) processItems() {
	for {
		select {
		case items := <-p.itemsCh:
			p.Lock()
			p.admit.Push(items)
			p.Unlock()
		case <-p.stop:
			return
		}
	}
}

func (p *defaultPolicy) Push(keys []uint64) bool {
	if len(keys) == 0 {
		return true
	}
	select {
	case p.itemsCh <- keys:
		p.stats.Add(keepGets, keys[0], uint64(len(keys)))
		return true
	default:
		p.stats.Add(dropGets, keys[0], uint64(len(keys)))
		return false
	}
}

func (p *defaultPolicy) Add(key uint64, cost int64) ([]*item, bool) {
	p.Lock()
	defer p.Unlock()
	// can't add an item bigger than entire cache
	if cost > p.evict.maxCost {
		return nil, false
	}
	// we don't need to go any further if the item is already in the cache
	if has := p.evict.updateIfHas(key, cost); has {
		return nil, true
	}
	// if we got this far, this key doesn't exist in the cache
	//
	// calculate the remaining room in the cache (usually bytes)
	room := p.evict.roomLeft(cost)
	if room >= 0 {
		// there's enough room in the cache to store the new item without
		// overflowing, so we can do that now and stop here
		p.evict.add(key, cost)
		return nil, true
	}
	// incHits is the hit count for the incoming item
	incHits := p.admit.Estimate(key)
	// sample is the eviction candidate pool to be filled via random sampling
	//
	// TODO: perhaps we should use a min heap here. Right now our time
	// complexity is N for finding the min. Min heap should bring it down to
	// O(lg N).
	sample := make([]*policyPair, 0, lfuSample)
	// as items are evicted they will be appended to victims
	victims := make([]*item, 0)
	// delete victims until there's enough space or a minKey is found that has
	// more hits than incoming item.
	for ; room < 0; room = p.evict.roomLeft(cost) {
		// fill up empty slots in sample
		sample = p.evict.fillSample(sample)
		// find minimally used item in sample
		minKey, minHits, minId, minCost := uint64(0), int64(math.MaxInt64), 0, int64(0)
		for i, pair := range sample {
			// look up hit count for sample key
			if hits := p.admit.Estimate(pair.key); hits < minHits {
				minKey, minHits, minId, minCost = pair.key, hits, i, pair.cost
			}
		}
		// if the incoming item isn't worth keeping in the policy, reject.
		if incHits < minHits {
			p.stats.Add(rejectSets, key, 1)
			return victims, false
		}
		// delete the victim from metadata
		p.evict.del(minKey)
		// delete the victim from sample
		sample[minId] = sample[len(sample)-1]
		sample = sample[:len(sample)-1]
		// store victim in evicted victims slice
		victims = append(victims, &item{
			key:  minKey,
			cost: minCost,
		})
	}
	p.evict.add(key, cost)
	return victims, true
}

func (p *defaultPolicy) Has(key uint64) bool {
	p.Lock()
	_, exists := p.evict.keyCosts[key]
	p.Unlock()
	return exists
}

func (p *defaultPolicy) Del(key uint64) {
	p.Lock()
	p.evict.del(key)
	p.Unlock()
}

func (p *defaultPolicy) Cap() int64 {
	p.Lock()
	capacity := int64(p.evict.maxCost - p.evict.used)
	p.Unlock()
	return capacity
}

func (p *defaultPolicy) Update(key uint64, cost int64) {
	p.Lock()
	p.evict.updateIfHas(key, cost)
	p.Unlock()
}

func (p *defaultPolicy) Cost(key uint64) int64 {
	p.Lock()
	defer p.Unlock()
	if cost, found := p.evict.keyCosts[key]; found {
		return cost
	}
	return -1
}

func (p *defaultPolicy) Clear() {
	p.Lock()
	defer p.Unlock()
	p.admit.clear()
	p.evict.clear()
}

func (p *defaultPolicy) Close() {
	// block until p.processItems goroutine is returned
	p.stop <- struct{}{}
	close(p.stop)
	close(p.itemsCh)
}

// sampledLFU is an eviction helper storing key-cost pairs.
type sampledLFU struct {
	keyCosts map[uint64]int64
	maxCost  int64
	used     int64
	stats    *metrics
}

func newSampledLFU(maxCost int64) *sampledLFU {
	return &sampledLFU{
		keyCosts: make(map[uint64]int64),
		maxCost:  maxCost,
	}
}

func (p *sampledLFU) roomLeft(cost int64) int64 {
	return p.maxCost - (p.used + cost)
}

func (p *sampledLFU) fillSample(in []*policyPair) []*policyPair {
	if len(in) >= lfuSample {
		return in
	}
	for key, cost := range p.keyCosts {
		in = append(in, &policyPair{key, cost})
		if len(in) >= lfuSample {
			return in
		}
	}
	return in
}

func (p *sampledLFU) del(key uint64) {
	cost, ok := p.keyCosts[key]
	if !ok {
		return
	}
	p.stats.Add(keyEvict, key, 1)
	p.stats.Add(costEvict, key, uint64(cost))
	p.used -= cost
	delete(p.keyCosts, key)
}

func (p *sampledLFU) add(key uint64, cost int64) {
	p.stats.Add(keyAdd, key, 1)
	p.stats.Add(costAdd, key, uint64(cost))
	p.keyCosts[key] = cost
	p.used += cost
}

func (p *sampledLFU) updateIfHas(key uint64, cost int64) bool {
	if prev, found := p.keyCosts[key]; found {
		// update the cost of an existing key, but don't worry about evicting,
		// evictions will be handled the next time a new item is added
		p.stats.Add(keyUpdate, key, 1)
		p.used += cost - prev
		p.keyCosts[key] = cost
		return true
	}
	return false
}

func (p *sampledLFU) clear() {
	p.used = 0
	p.keyCosts = make(map[uint64]int64)
}

// tinyLFU is an admission helper that keeps track of access frequency using
// tiny (4-bit) counters in the form of a count-min sketch.
// tinyLFU is NOT thread safe.
type tinyLFU struct {
	freq    *cmSketch
	door    *z.Bloom
	incrs   int64
	resetAt int64
}

func newTinyLFU(numCounters int64) *tinyLFU {
	return &tinyLFU{
		freq:    newCmSketch(numCounters),
		door:    z.NewBloomFilter(float64(numCounters), 0.01),
		resetAt: numCounters,
	}
}

func (p *tinyLFU) Push(keys []uint64) {
	for _, key := range keys {
		p.Increment(key)
	}
}

func (p *tinyLFU) Estimate(key uint64) int64 {
	hits := p.freq.Estimate(key)
	if p.door.Has(key) {
		hits += 1
	}
	return hits
}

func (p *tinyLFU) Increment(key uint64) {
	// flip doorkeeper bit if not already
	if added := p.door.AddIfNotHas(key); !added {
		// increment count-min counter if doorkeeper bit is already set.
		p.freq.Increment(key)
	}
	p.incrs++
	if p.incrs >= p.resetAt {
		p.reset()
	}
}

func (p *tinyLFU) reset() {
	// Zero out incrs.
	p.incrs = 0
	// clears doorkeeper bits
	p.door.Clear()
	// halves count-min counters
	p.freq.Reset()
}

func (p *tinyLFU) clear() {
	p.incrs = 0
	p.door.Clear()
	p.freq.Clear()
}
