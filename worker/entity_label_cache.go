/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import "sync"

// entityLabelCache is a concurrency-safe UID -> label cache.
// Used by the mutation routing layer to resolve entity labels without
// querying group 1 on every mutation.
type entityLabelCache struct {
	mu      sync.RWMutex
	entries map[uint64]string
	maxSize int
}

func newEntityLabelCache(maxSize int) *entityLabelCache {
	return &entityLabelCache{
		entries: make(map[uint64]string),
		maxSize: maxSize,
	}
}

// Get returns the cached label for a UID. Returns ("", false) on cache miss.
// An empty label with ok=true means the entity is explicitly unlabeled.
func (c *entityLabelCache) Get(uid uint64) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	label, ok := c.entries[uid]
	return label, ok
}

// Set stores a UID -> label mapping. If the cache exceeds maxSize, it is
// cleared (simple eviction strategy — revisit with LRU if needed).
func (c *entityLabelCache) Set(uid uint64, label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// Simple eviction: clear everything. This is acceptable because
		// cache misses just cause a read from group 1, not data loss.
		c.entries = make(map[uint64]string)
	}
	c.entries[uid] = label
}

// Invalidate removes a single UID from the cache.
func (c *entityLabelCache) Invalidate(uid uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, uid)
}

// Clear removes all entries. Used on DropAll.
func (c *entityLabelCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[uint64]string)
}
