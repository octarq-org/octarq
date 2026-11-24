package help

import (
	"container/list"
	"sync"
)

type lruCache[K comparable, V any] struct {
	mu    sync.Mutex
	max   int
	ll    *list.List
	items map[K]*list.Element
}

type lruItem[K comparable, V any] struct {
	key   K
	value V
}

func newLRU[K comparable, V any](max int) *lruCache[K, V] {
	return &lruCache[K, V]{
		max:   max,
		ll:    list.New(),
		items: make(map[K]*list.Element),
	}
}

func (c *lruCache[K, V]) Get(key K) (V, bool) {
	var zero V
	if c == nil {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return zero, false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*lruItem[K, V]).value, true
}

func (c *lruCache[K, V]) Put(key K, value V) {
	if c == nil || c.max <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*lruItem[K, V]).value = value
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&lruItem[K, V]{key: key, value: value})
	c.items[key] = el
	for c.ll.Len() > c.max {
		back := c.ll.Back()
		if back == nil {
			return
		}
		item := back.Value.(*lruItem[K, V])
		c.ll.Remove(back)
		delete(c.items, item.key)
	}
}
