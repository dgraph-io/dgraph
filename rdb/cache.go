package rdb

// #include <stdint.h>
// #include <stdlib.h>
// #include "rdbc.h"
import "C"

// Cache is a cache used to store data read from data in memory.
type Cache struct {
	c *C.rdb_cache_t
}

// NewLRUCache creates a new LRU Cache object with the capacity given.
func NewLRUCache(capacity int) *Cache {
	return NewNativeCache(C.rdb_cache_create_lru(C.size_t(capacity)))
}

// NewNativeCache creates a Cache object.
func NewNativeCache(c *C.rdb_cache_t) *Cache {
	return &Cache{c}
}

// Destroy deallocates the Cache object.
func (c *Cache) Destroy() {
	C.rdb_cache_destroy(c.c)
	c.c = nil
}
