// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The implementation borrows heavily from SmallLRUCache (originally by Nathan
// Schrenk). The object maintains a doubly-linked list of elements in the
// When an element is accessed it is promoted to the head of the list, and when
// space is needed the element at the tail of the list (the least recently used
// element) is evicted.
// Keys can be of anytype for which equality is defined.
package cache

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

type LRUCache struct {
	mu sync.Mutex

	// list & table of *entry objects
	list  *list.List
	table map[interface{}]*list.Element

	// Our current size, in bytes. Obviously a gross simplification and low-grade
	// approximation.
	size uint64

	// How many bytes we are limiting the cache to.
	capacity uint64

	evictSize uint64
	evicts    []KV

	Evict     func([]KV)
	EvictSize uint64 // size accumulated before Evict is called.
}

type KV struct {
	K interface{}
	V Value
}

// Values that go into LRUCache need to satisfy this interface.
type Value interface {
	Size() int
}

type Item struct {
	Key   interface{}
	Value Value
}

type entry struct {
	key           interface{}
	value         Value
	size          int
	time_accessed time.Time
}

func NewLRUCache(capacity uint64) *LRUCache {
	return &LRUCache{
		list:     list.New(),
		table:    make(map[interface{}]*list.Element),
		capacity: capacity,
	}
}

func (self *LRUCache) Get(key interface{}) (v Value, ok bool) {
	self.mu.Lock()
	defer self.mu.Unlock()

	element := self.table[key]
	if element == nil {
		return nil, false
	}
	self.moveToFront(element)
	return element.Value.(*entry).value, true
}

func (self *LRUCache) Set(key interface{}, value Value) {
	var evicts []KV
	func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		if element := self.table[key]; element != nil {
			self.updateInplace(element, value)
		} else {
			self.addNew(key, value)
		}
		evicts = self.toEvict()
	}()
	self.evict(evicts)
}

func (self *LRUCache) SetIfAbsent(key interface{}, value Value) {
	var evicts []KV
	func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		if element := self.table[key]; element != nil {
			self.moveToFront(element)
		} else {
			self.addNew(key, value)
		}
		evicts = self.toEvict()
	}()
	self.evict(evicts)
}

func (self *LRUCache) Delete(key interface{}) bool {
	var evicts []KV
	ok := func() bool {
		self.mu.Lock()
		defer self.mu.Unlock()

		element := self.table[key]
		if element == nil {
			return false
		}

		entry := element.Value.(*entry)

		self.list.Remove(element)
		delete(self.table, key)
		self.size -= uint64(entry.size)

		self.evictSize += uint64(entry.size)
		self.evicts = append(self.evicts, KV{entry.key, entry.value})
		evicts = self.toEvict()

		return true
	}()
	self.evict(evicts)
	return ok
}

func (self *LRUCache) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.list.Init()
	self.table = make(map[interface{}]*list.Element)
	self.size = 0
}

func (self *LRUCache) SetCapacity(capacity uint64) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.capacity = capacity
	self.checkCapacity()
}

func (self *LRUCache) Stats() (length, size, capacity uint64, oldest time.Time) {
	self.mu.Lock()
	defer self.mu.Unlock()
	if lastElem := self.list.Back(); lastElem != nil {
		oldest = lastElem.Value.(*entry).time_accessed
	}
	return uint64(self.list.Len()), self.size, self.capacity, oldest
}

func (self *LRUCache) StatsJSON() string {
	if self == nil {
		return "{}"
	}
	l, s, c, o := self.Stats()
	return fmt.Sprintf("{\"Length\": %v, \"Size\": %v, \"Capacity\": %v, \"OldestAccess\": \"%v\"}", l, s, c, o)
}

func (self *LRUCache) Keys() []interface{} {
	self.mu.Lock()
	defer self.mu.Unlock()

	keys := make([]interface{}, 0, self.list.Len())
	for e := self.list.Front(); e != nil; e = e.Next() {
		keys = append(keys, e.Value.(*entry).key)
	}
	return keys
}

func (self *LRUCache) Items() []Item {
	self.mu.Lock()
	defer self.mu.Unlock()

	items := make([]Item, 0, self.list.Len())
	for e := self.list.Front(); e != nil; e = e.Next() {
		v := e.Value.(*entry)
		items = append(items, Item{Key: v.key, Value: v.value})
	}
	return items
}

func (self *LRUCache) updateInplace(element *list.Element, value Value) {
	valueSize := value.Size()
	sizeDiff := valueSize - element.Value.(*entry).size
	element.Value.(*entry).value = value
	element.Value.(*entry).size = valueSize
	self.size += uint64(sizeDiff)
	self.moveToFront(element)
	self.checkCapacity()
}

func (self *LRUCache) moveToFront(element *list.Element) {
	self.list.MoveToFront(element)
	element.Value.(*entry).time_accessed = time.Now()
}

func (self *LRUCache) addNew(key interface{}, value Value) {
	newEntry := &entry{key, value, value.Size(), time.Now()}
	element := self.list.PushFront(newEntry)
	self.table[key] = element
	self.size += uint64(newEntry.size)
	self.checkCapacity()
}

func (self *LRUCache) evict(kvs []KV) {
	if self.Evict != nil {
		self.Evict(kvs)
	}
}

// copy so we can evict out of the mutex. nil if not at size.
func (self *LRUCache) toEvict() []KV {
	if self.evictSize < self.EvictSize {
		return nil
	}
	v := append([]KV(nil), self.evicts...)
	self.evicts = self.evicts[:0]
	self.evictSize = 0
	return v
}

func (self *LRUCache) checkCapacity() {
	// Partially duplicated from Delete
	for self.size > self.capacity {
		delElem := self.list.Back()
		delValue := delElem.Value.(*entry)
		self.list.Remove(delElem)
		delete(self.table, delValue.key)
		self.size -= uint64(delValue.size)

		self.evictSize += uint64(delValue.size)
		self.evicts = append(self.evicts, KV{delValue.key, delValue.value})
	}
}
