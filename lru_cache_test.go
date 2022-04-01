// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"testing"
)

type CacheValue struct {
	size int
}

type CacheKey struct {
	k int
}

func (self *CacheValue) Size() int {
	return self.size
}

func TestInitialState(t *testing.T) {
	cache := NewLRUCache(5)
	l, sz, c, _ := cache.Stats()
	if l != 0 {
		t.Errorf("length = %v, want 0", l)
	}
	if sz != 0 {
		t.Errorf("size = %v, want 0", sz)
	}
	if c != 5 {
		t.Errorf("capacity = %v, want 5", c)
	}
}

func TestSetInsertsValue(t *testing.T) {
	cache := NewLRUCache(100)
	data := &CacheValue{0}
	key := "key"
	cache.Set(key, data)

	v, ok := cache.Get(key)
	if !ok || v.(*CacheValue) != data {
		t.Errorf("Cache has incorrect value: %v != %v", data, v)
	}
}

func TestGetValueWithMultipleTypes(t *testing.T) {
	cache := NewLRUCache(100)
	data := &CacheValue{0}
	key := "key"
	cache.Set(key, data)

	v, ok := cache.Get("key")
	if !ok || v.(*CacheValue) != data {
		t.Errorf("Cache has incorrect value for \"key\": %v != %v", data, v)
	}

	v, ok = cache.Get(string([]byte{'k', 'e', 'y'}))
	if !ok || v.(*CacheValue) != data {
		t.Errorf("Cache has incorrect value for []byte {'k','e','y'}: %v != %v", data, v)
	}
}

func TestGetWithDifferntKeyTypes(t *testing.T) {
	cache := NewLRUCache(100)
	data := &CacheValue{0}
	cache.Set("key", data)

	if v, ok := cache.Get("key"); !ok {
		t.Errorf("Cache has incorrect value for \"key\": %v != %v", data, v)
	}
	cache.Set(CacheKey{0}, data)
	if v, ok := cache.Get(CacheKey{0}); !ok {
		t.Errorf("Cache has incorrect value for \"key\": %v != %v", data, v)
	}
}

func TestSetUpdatesSize(t *testing.T) {
	cache := NewLRUCache(100)
	emptyValue := &CacheValue{0}
	key := "key1"
	cache.Set(key, emptyValue)
	if _, sz, _, _ := cache.Stats(); sz != 0 {
		t.Errorf("cache.Size() = %v, expected 0", sz)
	}
	someValue := &CacheValue{20}
	key = "key2"
	cache.Set(key, someValue)
	if _, sz, _, _ := cache.Stats(); sz != 20 {
		t.Errorf("cache.Size() = %v, expected 20", sz)
	}
}

func TestSetWithOldKeyUpdatesValue(t *testing.T) {
	cache := NewLRUCache(100)
	emptyValue := &CacheValue{0}
	key := "key1"
	cache.Set(key, emptyValue)
	someValue := &CacheValue{20}
	cache.Set(key, someValue)

	v, ok := cache.Get(key)
	if !ok || v.(*CacheValue) != someValue {
		t.Errorf("Cache has incorrect value: %v != %v", someValue, v)
	}
}

func TestSetWithOldKeyUpdatesSize(t *testing.T) {
	cache := NewLRUCache(100)
	emptyValue := &CacheValue{0}
	key := "key1"
	cache.Set(key, emptyValue)

	if _, sz, _, _ := cache.Stats(); sz != 0 {
		t.Errorf("cache.Size() = %v, expected %v", sz, 0)
	}

	someValue := &CacheValue{20}
	cache.Set(key, someValue)
	expected := uint64(someValue.size)
	if _, sz, _, _ := cache.Stats(); sz != expected {
		t.Errorf("cache.Size() = %v, expected %v", sz, expected)
	}
}

func TestGetNonExistent(t *testing.T) {
	cache := NewLRUCache(100)

	if _, ok := cache.Get("crap"); ok {
		t.Error("Cache returned a crap value after no inserts.")
	}
}

func TestDelete(t *testing.T) {
	cache := NewLRUCache(100)
	value := &CacheValue{1}
	key := "key"

	if cache.Delete(key) {
		t.Error("Item unexpectedly already in cache.")
	}

	cache.Set(key, value)

	if !cache.Delete(key) {
		t.Error("Expected item to be in cache.")
	}

	if _, sz, _, _ := cache.Stats(); sz != 0 {
		t.Errorf("cache.Size() = %v, expected 0", sz)
	}

	if _, ok := cache.Get(key); ok {
		t.Error("Cache returned a value after deletion.")
	}
}

func TestClear(t *testing.T) {
	cache := NewLRUCache(100)
	value := &CacheValue{1}
	key := "key"

	cache.Set(key, value)
	cache.Clear()

	if _, sz, _, _ := cache.Stats(); sz != 0 {
		t.Errorf("cache.Size() = %v, expected 0 after Clear()", sz)
	}
}

func TestCapacityIsObeyed(t *testing.T) {
	size := uint64(3)
	cache := NewLRUCache(size)
	value := &CacheValue{1}

	// Insert up to the cache's capacity.
	cache.Set("key1", value)
	cache.Set("key2", value)
	cache.Set("key3", value)
	if _, sz, _, _ := cache.Stats(); sz != size {
		t.Errorf("cache.Size() = %v, expected %v", sz, size)
	}
	// Insert one more; something should be evicted to make room.
	cache.Set("key4", value)
	if _, sz, _, _ := cache.Stats(); sz != size {
		t.Errorf("post-evict cache.Size() = %v, expected %v", sz, size)
	}
}

func TestLRUIsEvictedSingle(t *testing.T) {
	size := uint64(4)
	cache := NewLRUCache(size)

	evicts := map[interface{}]struct{}{"key3": {}, "key4": {}}
	cache.Evict = func(kvs []KV) {
		for _, kv := range kvs {
			delete(evicts, kv.K)
		}
	}

	cache.Set("key1", &CacheValue{1})
	cache.Set("key2", &CacheValue{1})
	cache.Set("key3", &CacheValue{1})
	cache.Set("key4", &CacheValue{1})
	// lru: [key4, key3, key2, key1]

	// Look up the elements. This will rearrange the LRU ordering.
	cache.Get("key4")
	cache.Get("key3")
	cache.Get("key2")
	cache.Get("key1")
	// lru: [key1, key2, key3, key4]

	cache.Set("keyA", &CacheValue{1})
	// lru: [keyA, key1, key2, key3]

	if len(evicts) != 1 {
		t.Fatalf("Expected not evicted: %+v", evicts)
	}

	cache.Set("keyB", &CacheValue{1})
	// lru: [keyB, keyA, key1, key2]

	if _, ok := cache.Get("key3"); ok {
		t.Error("Least recently used key3 was not evicted.")
	}
	if _, ok := cache.Get("key4"); ok {
		t.Error("Least recently used key4 was not evicted.")
	}

	if len(evicts) != 0 {
		t.Errorf("Expected not evicted: %+v", evicts)
	}
}

func TestLRUIsEvictedBatch(t *testing.T) {
	size := uint64(4)
	cache := NewLRUCache(size)

	evicts := map[interface{}]struct{}{"key3": {}, "key4": {}}
	cache.Evict = func(kvs []KV) {
		for _, kv := range kvs {
			delete(evicts, kv.K)
		}
	}
	cache.EvictSize = 2

	cache.Set("key1", &CacheValue{1})
	cache.Set("key2", &CacheValue{1})
	cache.Set("key3", &CacheValue{1})
	cache.Set("key4", &CacheValue{1})
	// lru: [key4, key3, key2, key1]

	// Look up the elements. This will rearrange the LRU ordering.
	cache.Get("key4")
	cache.Get("key3")
	cache.Get("key2")
	cache.Get("key1")
	// lru: [key1, key2, key3, key4]

	cache.Set("keyA", &CacheValue{1})
	// lru: [keyA, key1, key2, key3]

	if len(evicts) != 2 {
		t.Fatalf("Unexpected eviction: %+v", evicts)
	}

	cache.Set("keyB", &CacheValue{1})
	// lru: [keyB, keyA, key1, key2]

	if _, ok := cache.Get("key3"); ok {
		t.Error("Least recently used key3 was not evicted.")
	}
	if _, ok := cache.Get("key4"); ok {
		t.Error("Least recently used key4 was not evicted.")
	}

	if len(evicts) != 0 {
		t.Errorf("Expected not evicted: %+v", evicts)
	}
}
