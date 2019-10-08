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

// Ristretto is a fast, fixed size, in-memory cache with a dual focus on
// throughput and hit ratio performance. You can easily add Ristretto to an
// existing system and keep the most valuable data where you need it.
package ristretto

import (
	"errors"
	"sync/atomic"

	"github.com/dgraph-io/ristretto/z"
)

const (
	// TODO: find the optimal value for this or make it configurable
	setBufSize = 32 * 1024
)

// Cache is a thread-safe implementation of a hashmap with a TinyLFU admission
// policy and a Sampled LFU eviction policy. You can use the same Cache instance
// from as many goroutines as you want.
type Cache struct {
	// store is the central concurrent hashmap where key-value items are stored
	store store
	// policy determines what gets let in to the cache and what gets kicked out
	policy policy
	// getBuf is a custom ring buffer implementation that gets pushed to when
	// keys are read
	getBuf *ringBuffer
	// setBuf is a buffer allowing us to batch/drop Sets during times of high
	// contention
	setBuf chan *item
	// stats contains a running log of important statistics like hits, misses,
	// and dropped items
	stats *metrics
	// onEvict is called for item evictions
	onEvict func(uint64, interface{}, int64)
	// KeyToHash function is used to customize the key hashing algorithm.
	// Each key will be hashed using the provided function. If keyToHash value
	// is not set, the default keyToHash function is used.
	keyToHash func(interface{}) uint64
	// stop is used to stop the processItems goroutine
	stop chan struct{}
	// cost calculates cost from a value
	cost func(value interface{}) int64
}

// Config is passed to NewCache for creating new Cache instances.
type Config struct {
	// NumCounters determines the number of counters (keys) to keep that hold
	// access frequency information. It's generally a good idea to have more
	// counters than the max cache capacity, as this will improve eviction
	// accuracy and subsequent hit ratios.
	//
	// For example, if you expect your cache to hold 1,000,000 items when full,
	// NumCounters should be 10,000,000 (10x). Each counter takes up 4 bits, so
	// keeping 10,000,000 counters would require 5MB of memory.
	NumCounters int64
	// MaxCost can be considered as the cache capacity, in whatever units you
	// choose to use.
	//
	// For example, if you want the cache to have a max capacity of 100MB, you
	// would set MaxCost to 100,000,000 and pass an item's number of bytes as
	// the `cost` parameter for calls to Set. If new items are accepted, the
	// eviction process will take care of making room for the new item and not
	// overflowing the MaxCost value.
	MaxCost int64
	// BufferItems determines the size of Get buffers.
	//
	// Unless you have a rare use case, using `64` as the BufferItems value
	// results in good performance.
	BufferItems int64
	// Metrics determines whether cache statistics are kept during the cache's
	// lifetime. There *is* some overhead to keeping statistics, so you should
	// only set this flag to true when testing or throughput performance isn't a
	// major factor.
	Metrics bool
	// OnEvict is called for every eviction and passes the hashed key, value,
	// and cost to the function.
	OnEvict func(key uint64, value interface{}, cost int64)
	// KeyToHash function is used to customize the key hashing algorithm.
	// Each key will be hashed using the provided function. If keyToHash value
	// is not set, the default keyToHash function is used.
	KeyToHash func(key interface{}) uint64
	// Cost evaluates a value and outputs a corresponding cost. This function
	// is ran after Set is called for a new item or an item update with a cost
	// param of 0.
	Cost func(value interface{}) int64
}

type itemFlag byte

const (
	itemNew itemFlag = iota
	itemDelete
	itemUpdate
)

// item is passed to setBuf so items can eventually be added to the cache
type item struct {
	flag  itemFlag
	key   uint64
	value interface{}
	cost  int64
}

// NewCache returns a new Cache instance and any configuration errors, if any.
func NewCache(config *Config) (*Cache, error) {
	switch {
	case config.NumCounters == 0:
		return nil, errors.New("NumCounters can't be zero.")
	case config.MaxCost == 0:
		return nil, errors.New("MaxCost can't be zero.")
	case config.BufferItems == 0:
		return nil, errors.New("BufferItems can't be zero.")
	}
	policy := newPolicy(config.NumCounters, config.MaxCost)
	cache := &Cache{
		store:     newStore(),
		policy:    policy,
		getBuf:    newRingBuffer(policy, config.BufferItems),
		setBuf:    make(chan *item, setBufSize),
		onEvict:   config.OnEvict,
		keyToHash: config.KeyToHash,
		stop:      make(chan struct{}),
		cost:      config.Cost,
	}
	if cache.keyToHash == nil {
		cache.keyToHash = z.KeyToHash
	}
	if config.Metrics {
		cache.collectMetrics()
	}
	// NOTE: benchmarks seem to show that performance decreases the more
	//       goroutines we have running cache.processItems(), so 1 should
	//       usually be sufficient
	go cache.processItems()
	return cache, nil
}

// Get returns the value (if any) and a boolean representing whether the
// value was found or not. The value can be nil and the boolean can be true at
// the same time.
func (c *Cache) Get(key interface{}) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	hash := c.keyToHash(key)
	c.getBuf.Push(hash)
	val, ok := c.store.Get(hash)
	if ok {
		c.stats.Add(hit, hash, 1)
	} else {
		c.stats.Add(miss, hash, 1)
	}
	return val, ok
}

// Set attempts to add the key-value item to the cache. If it returns false,
// then the Set was dropped and the key-value item isn't added to the cache. If
// it returns true, there's still a chance it could be dropped by the policy if
// its determined that the key-value item isn't worth keeping, but otherwise the
// item will be added and other items will be evicted in order to make room.
//
// To dynamically evaluate the items cost using the Config.Coster function, set
// the cost parameter to 0 and Coster will be ran when needed in order to find
// the items true cost.
func (c *Cache) Set(key, value interface{}, cost int64) bool {
	if c == nil {
		return false
	}
	i := &item{
		flag:  itemNew,
		key:   c.keyToHash(key),
		value: value,
		cost:  cost,
	}
	// attempt to immediately update hashmap value and set flag to update so the
	// cost is eventually updated
	if c.store.Update(i.key, i.value) {
		i.flag = itemUpdate
	}
	// attempt to send item to policy
	select {
	case c.setBuf <- i:
		return true
	default:
		c.stats.Add(dropSets, i.key, 1)
		return false
	}
}

// Del deletes the key-value item from the cache if it exists.
func (c *Cache) Del(key interface{}) {
	if c == nil {
		return
	}
	c.setBuf <- &item{
		flag: itemDelete,
		key:  c.keyToHash(key),
	}
}

// Close stops all goroutines and closes all channels.
func (c *Cache) Close() {
	// block until processItems goroutine is returned
	c.stop <- struct{}{}
	close(c.stop)
	close(c.setBuf)
	c.policy.Close()
}

// Clear empties the hashmap and zeroes all policy counters. Note that this is
// not an atomic operation (but that shouldn't be a problem as it's assumed that
// Set/Get calls won't be occurring until after this).
func (c *Cache) Clear() {
	// block until processItems goroutine is returned
	c.stop <- struct{}{}
	// swap out the setBuf channel
	c.setBuf = make(chan *item, setBufSize)
	// clear value hashmap and policy data
	c.policy.Clear()
	c.store.Clear()
	// only reset metrics if they're enabled
	if c.stats != nil {
		c.collectMetrics()
	}
	// restart processItems goroutine
	go c.processItems()
}

// processItems is ran by goroutines processing the Set buffer.
func (c *Cache) processItems() {
	for {
		select {
		case i := <-c.setBuf:
			// calculate item cost value if new or update
			if i.cost == 0 && c.cost != nil && i.flag != itemDelete {
				i.cost = c.cost(i.value)
			}
			switch i.flag {
			case itemNew:
				if victims, added := c.policy.Add(i.key, i.cost); added {
					// item was accepted by the policy, so add to the hashmap
					c.store.Set(i.key, i.value)
					// delete victims
					for _, victim := range victims {
						// TODO: make Get-Delete atomic
						if c.onEvict != nil {
							victim.value, _ = c.store.Get(victim.key)
							c.onEvict(victim.key, victim.value, victim.cost)
						}
						c.store.Del(victim.key)
					}
				}
			case itemUpdate:
				c.policy.Update(i.key, i.cost)
			case itemDelete:
				c.policy.Del(i.key)
				c.store.Del(i.key)
			}
		case <-c.stop:
			return
		}
	}
}

// collectMetrics just creates a new *metrics instance and adds the pointers
// to the cache and policy instances.
func (c *Cache) collectMetrics() {
	stats := newMetrics()
	c.stats = stats
	c.policy.CollectMetrics(stats)
}

// Metrics returns statistics about cache performance.
func (c *Cache) Metrics() *Metrics {
	if c == nil {
		return nil
	}
	return exportMetrics(c.stats)
}

// exportMetrics converts an internal metrics struct into a friendlier Metrics
// struct.
func exportMetrics(stats *metrics) *Metrics {
	return &Metrics{
		Hits:         stats.Get(hit),
		Misses:       stats.Get(miss),
		Ratio:        stats.Ratio(),
		KeysAdded:    stats.Get(keyAdd),
		KeysUpdated:  stats.Get(keyUpdate),
		KeysEvicted:  stats.Get(keyEvict),
		CostAdded:    stats.Get(costAdd),
		CostEvicted:  stats.Get(costEvict),
		SetsDropped:  stats.Get(dropSets),
		SetsRejected: stats.Get(rejectSets),
		GetsDropped:  stats.Get(dropGets),
		GetsKept:     stats.Get(keepGets),
	}
}

// Metrics is a snapshot of performance statistics for the lifetime of a cache
// instance.
type Metrics struct {
	// Hits is the number of Get calls where a value was found for the
	// corresponding key.
	Hits uint64 `json:"hits"`
	// Misses is the number of Get calls where a value was not found for the
	// corresponding key.
	Misses uint64 `json:"misses"`
	// Ratio is the number of Hits over all accesses (Hits + Misses). This is
	// the percentage of successful Get calls.
	Ratio float64 `json:"ratio"`
	// KeysAdded is the total number of Set calls where a new key-value item was
	// added.
	KeysAdded uint64 `json:"keysAdded"`
	// KeysUpdated is the total number of Set calls where the value was updated.
	KeysUpdated uint64 `json:"keysUpdated"`
	// KeysEvicted is the total number of keys evicted.
	KeysEvicted uint64 `json:"keysEvicted"`
	// CostAdded is the sum of all costs that have been added (successful Set
	// calls).
	CostAdded uint64 `json:"costAdded"`
	// CostEvicted is the sum of all costs that have been evicted.
	CostEvicted uint64 `json:"costEvicted"`
	// SetsDropped is the number of Set calls that don't make it into internal
	// buffers (due to contention or some other reason).
	SetsDropped uint64 `json:"setsDropped"`
	// SetsRejected is the number of Set calls rejected by the policy (TinyLFU).
	SetsRejected uint64 `json:"setsRejected"`
	// GetsDropped is the number of Get counter increments that are dropped
	// internally.
	GetsDropped uint64 `json:"getsDropped"`
	// GetsKept is the number of Get counter increments that are kept.
	GetsKept uint64 `json:"getsKept"`
}

type metricType int

const (
	// The following 2 keep track of hits and misses.
	hit = iota
	miss
	// The following 3 keep track of number of keys added, updated and evicted.
	keyAdd
	keyUpdate
	keyEvict
	// The following 2 keep track of cost of keys added and evicted.
	costAdd
	costEvict
	// The following keep track of how many sets were dropped or rejected later.
	dropSets
	rejectSets
	// The following 2 keep track of how many gets were kept and dropped on the
	// floor.
	dropGets
	keepGets
	// This should be the final enum. Other enums should be set before this.
	doNotUse
)

// metrics is the struct for hit ratio statistics. Padding is used to avoid
// false sharing in order to minimize the performance cost for those who track
// metrics outside of testing scenarios.
type metrics struct {
	all [doNotUse][]*uint64
}

func newMetrics() *metrics {
	s := &metrics{}
	for i := 0; i < doNotUse; i++ {
		s.all[i] = make([]*uint64, 256)
		slice := s.all[i]
		for j := range slice {
			slice[j] = new(uint64)
		}
	}
	return s
}

func (p *metrics) Add(t metricType, hash, delta uint64) {
	if p == nil {
		return
	}
	valp := p.all[t]
	// Avoid false sharing by padding at least 64 bytes of space between two
	// atomic counters which would be incremented.
	idx := (hash % 25) * 10
	atomic.AddUint64(valp[idx], delta)
}

func (p *metrics) Get(t metricType) uint64 {
	if p == nil {
		return 0
	}
	valp := p.all[t]
	var total uint64
	for i := range valp {
		total += atomic.LoadUint64(valp[i])
	}
	return total
}

func (p *metrics) Ratio() float64 {
	if p == nil {
		return 0.0
	}
	hits, misses := p.Get(hit), p.Get(miss)
	if hits == 0 && misses == 0 {
		return 0.0
	}
	return float64(hits) / float64(hits+misses)
}
