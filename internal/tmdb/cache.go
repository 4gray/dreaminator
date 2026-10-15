package tmdb

import (
	"container/list"
	"context"
	"errors"
	"sync"
	"time"
)

// Cache is a small LRU with per-entry expiry and an optional byte budget. The
// receiver has little RAM next to Enigma2, so every cache here is bounded by
// entries and, where values are text or pictures, by bytes.
type Cache[V any] struct {
	mu       sync.Mutex
	max      int
	maxBytes int
	bytes    int
	size     func(V) int
	ll       *list.List
	items    map[string]*list.Element
	now      func() time.Time
}

type cacheEntry[V any] struct {
	key  string
	val  V
	exp  time.Time
	size int
}

// NewCache keeps at most maxEntries values; with a size function it also keeps
// their total size within maxBytes. A value larger than maxBytes is not kept.
func NewCache[V any](maxEntries, maxBytes int, size func(V) int) *Cache[V] {
	return &Cache[V]{max: max(1, maxEntries), maxBytes: maxBytes, size: size, ll: list.New(), items: map[string]*list.Element{}, now: time.Now}
}

func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero V
	el, ok := c.items[key]
	if !ok {
		return zero, false
	}
	e := el.Value.(*cacheEntry[V])
	if !c.now().Before(e.exp) {
		c.remove(el)
		return zero, false
	}
	c.ll.MoveToFront(el)
	return e.val, true
}

func (c *Cache[V]) Put(key string, v V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	size := 0
	if c.size != nil {
		size = c.size(v)
		if c.maxBytes > 0 && size > c.maxBytes {
			if el, ok := c.items[key]; ok {
				c.remove(el)
			}
			return
		}
	}
	if el, ok := c.items[key]; ok {
		c.remove(el)
	}
	c.items[key] = c.ll.PushFront(&cacheEntry[V]{key: key, val: v, exp: c.now().Add(ttl), size: size})
	c.bytes += size
	for c.ll.Len() > c.max || (c.maxBytes > 0 && c.bytes > c.maxBytes) {
		c.remove(c.ll.Back())
	}
}

func (c *Cache[V]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.remove(el)
	}
}

func (c *Cache[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = map[string]*list.Element{}
	c.bytes = 0
}

func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

func (c *Cache[V]) Bytes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

func (c *Cache[V]) remove(el *list.Element) {
	e := c.ll.Remove(el).(*cacheEntry[V])
	delete(c.items, e.key)
	c.bytes -= e.size
}

// Group runs one function per key at a time; callers asking for the same key
// meanwhile wait for that result instead of sending their own request.
type Group[V any] struct {
	mu    sync.Mutex
	calls map[string]*flight[V]
}

type flight[V any] struct {
	done chan struct{}
	val  V
	err  error
}

var errLookupFailed = errors.New("TMDB: внутренняя ошибка поиска")

// Do starts fn for key unless it is already running. fn gets a context that is
// detached from any single caller and bounded by timeout: a viewer leaving the
// screen does not cancel the lookup that others joined, and its result still
// lands in the caches. Every caller returns when its own ctx ends.
func (g *Group[V]) Do(ctx context.Context, key string, timeout time.Duration, fn func(context.Context) (V, error)) (V, error) {
	if err := ctx.Err(); err != nil {
		var zero V
		return zero, err
	}
	g.mu.Lock()
	if g.calls == nil {
		g.calls = map[string]*flight[V]{}
	}
	f, ok := g.calls[key]
	if !ok {
		f = &flight[V]{done: make(chan struct{})}
		g.calls[key] = f
		go g.run(context.WithoutCancel(ctx), key, timeout, f, fn)
	}
	g.mu.Unlock()
	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

func (g *Group[V]) run(ctx context.Context, key string, timeout time.Duration, f *flight[V], fn func(context.Context) (V, error)) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		// Decoration must never take the service down with it.
		if r := recover(); r != nil {
			var zero V
			f.val, f.err = zero, errLookupFailed
		}
		g.mu.Lock()
		delete(g.calls, key)
		g.mu.Unlock()
		close(f.done)
	}()
	f.val, f.err = fn(ctx)
}
