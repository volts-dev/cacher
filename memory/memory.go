package memory

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/volts-dev/cacher"
)

var Memory = cacher.Register("Memory", func() cacher.ICacher {
	return New()
})

// 锁规则：TMemoryCache.RWMutex 是唯一锁。
// 读操作使用 RLock/RUnlock，写操作使用 Lock/Unlock。
// 任何路径不得在持有此锁的同时请求另一把锁（无嵌套锁）。
// gcOnce 使用完整的 Lock → 修改 → Unlock，不分段。
type (
	TIndex struct {
		ele     *list.Element
		block   *cacher.CacheBlock
		expired time.Duration
	}

	TIndexList []TIndex

	ListCache interface {
		cacher.ICacher
		Front() *cacher.CacheBlock
		Back() *cacher.CacheBlock
		MoveToFront(key string)
		MoveToBack(key string)
	}

	StackCache interface {
		Push(any) error
		Shift() any
		Pop() any
	}

	// Memory cache adapter.
	// it contains a RW locker for safe map storage.
	TMemoryCache struct {
		sync.RWMutex
		config    *Config
		blocks    map[string]*list.Element
		gcList    *list.List
		blockPool sync.Pool
		stopCh    chan struct{}
	}
)

// New returns a new MemoryCache.
func New(opts ...cacher.Option) *TMemoryCache {
	cfg := &Config{
		Active:   true,
		Interval: cacher.INTERVAL_TIME * time.Second,
		Expire:   cacher.EXPIRED_TIME * time.Second,
		Size:     cacher.MAX_CACHE,
	}
	cfg.Init(opts...)

	c := &TMemoryCache{
		config: cfg,
		blocks: make(map[string]*list.Element),
		gcList: list.New(),
		stopCh: make(chan struct{}),
	}
	c.blockPool.New = func() any { return &cacher.CacheBlock{} }

	if cfg.Interval > 0 {
		go c.vaccuum()
	}

	return c
}

// NewStack returns a new StackCacher.
func NewStack(opts ...cacher.Option) StackCache {
	opts = append([]cacher.Option{
		WithExpire(60),
		WithInterval(30),
		WithSize(2000),
	}, opts...)
	return New(opts...)
}

func (self *TMemoryCache) Init(opts ...cacher.Option) {
	self.config.Init(opts...)
}

// Keys returns all cache keys.
func (self *TMemoryCache) Keys(ctx ...context.Context) []string {
	if !self.config.Active {
		return nil
	}
	self.RLock()
	defer self.RUnlock()

	keys := make([]string, 0, len(self.blocks))
	for k := range self.blocks {
		keys = append(keys, k)
	}
	return keys
}

// Close stops the GC goroutine and clears all cache.
func (self *TMemoryCache) Close() error {
	select {
	case <-self.stopCh:
		// already closed
	default:
		close(self.stopCh)
	}
	return self.Clear()
}

// Clear deletes all cache entries.
func (self *TMemoryCache) Clear() error {
	self.Lock()
	self.gcList.Init()
	self.blocks = make(map[string]*list.Element)
	self.Unlock()
	return nil
}

// Front returns the first element.
func (self *TMemoryCache) Front() *cacher.CacheBlock {
	if !self.config.Active {
		return nil
	}
	self.RLock()
	front := self.gcList.Front()
	self.RUnlock()
	if front == nil {
		return nil
	}
	block := front.Value.(*cacher.CacheBlock)
	block.LastAccess = time.Now()
	return block
}

// Back returns the last element.
func (self *TMemoryCache) Back() *cacher.CacheBlock {
	if !self.config.Active {
		return nil
	}
	self.RLock()
	back := self.gcList.Back()
	self.RUnlock()
	if back == nil {
		return nil
	}
	block := back.Value.(*cacher.CacheBlock)
	block.LastAccess = time.Now()
	return block
}

func (self *TMemoryCache) MoveToFront(key string) {
	self.Lock()
	ele := self.blocks[key]
	if ele != nil {
		self.gcList.MoveToFront(ele)
	}
	self.Unlock()
}

func (self *TMemoryCache) MoveToBack(key string) {
	self.Lock()
	ele := self.blocks[key]
	if ele != nil {
		self.gcList.MoveToBack(ele)
	}
	self.Unlock()
}

// Get returns cached value by key. Returns nil if not found or expired.
// Uses write lock because MoveToFront modifies gcList.
func (self *TMemoryCache) Get(name string, ctx ...context.Context) (any, error) {
	if !self.config.Active {
		return nil, cacher.ErrInactive
	}

	self.Lock()
	ele, ok := self.blocks[name]
	if !ok || ele == nil {
		self.Unlock()
		return nil, cacher.ErrCacheMiss
	}
	block, ok := ele.Value.(*cacher.CacheBlock)
	if !ok {
		self.Unlock()
		return nil, cacher.ErrCacheMiss
	}
	block.LastAccess = time.Now()
	self.gcList.MoveToFront(ele)
	self.Unlock()

	return block.Value, nil
}

// Set stores a value in the cache.
func (self *TMemoryCache) Set(block *cacher.CacheBlock) error {
	if !self.config.Active {
		return nil
	}
	block.LastAccess = time.Now()

	self.Lock()
	if len(self.blocks) >= self.config.Size {
		self.Unlock()
		return nil
	}
	ele, has := self.blocks[block.Key]
	if has {
		ele.Value = block
		self.Unlock()
		return nil
	}
	item := self.gcList.PushFront(block)
	self.blocks[block.Key] = item
	self.Unlock()
	return nil
}

// Shift removes and returns the first element (FIFO dequeue).
func (self *TMemoryCache) Shift() any {
	if !self.config.Active {
		return nil
	}

	self.Lock()
	ele := self.gcList.Front()
	if ele == nil {
		self.Unlock()
		return nil
	}
	self.gcList.Remove(ele)
	self.Unlock()

	block, ok := ele.Value.(*cacher.CacheBlock)
	if !ok {
		return nil
	}
	value := block.Value
	block.Key = ""
	block.Value = nil
	self.blockPool.Put(block)
	return value
}

// Pop removes and returns the last element (LIFO dequeue).
func (self *TMemoryCache) Pop() any {
	if !self.config.Active {
		return nil
	}

	self.Lock()
	ele := self.gcList.Back()
	if ele == nil {
		self.Unlock()
		return nil
	}
	self.gcList.Remove(ele)
	self.Unlock()

	block, ok := ele.Value.(*cacher.CacheBlock)
	if !ok {
		return nil
	}
	value := block.Value
	block.Key = ""
	block.Value = nil
	self.blockPool.Put(block)
	return value
}

// Push appends a value to the back of the list (stack/queue use).
func (self *TMemoryCache) Push(value any) error {
	if !self.config.Active {
		return nil
	}

	self.Lock()
	if self.gcList.Len() >= self.config.Size {
		self.Unlock()
		return nil
	}
	block := self.blockPool.Get().(*cacher.CacheBlock)
	block.LastAccess = time.Now()
	block.Value = value
	self.gcList.PushBack(block)
	self.Unlock()
	return nil
}

// Delete removes a cache entry by key.
// Atomic: both gcList and blocks map are updated under a single Lock.
func (self *TMemoryCache) Delete(key string, ctx ...context.Context) error {
	self.Lock()
	defer self.Unlock()
	ele, ok := self.blocks[key]
	if !ok {
		return fmt.Errorf("key %s does not exist", key)
	}
	self.gcList.Remove(ele)
	delete(self.blocks, key)
	return nil
}

// Incr increments a numeric cache value.
func (self *TMemoryCache) Incr(key string) error {
	self.RLock()
	ele, ok := self.blocks[key]
	self.RUnlock()

	if !ok {
		return fmt.Errorf("key %s does not exist", key)
	}
	itm := ele.Value.(*cacher.CacheBlock)
	itm.LastAccess = itm.LastAccess.Add(cacher.DELAY_TIME * time.Second)
	switch v := itm.Value.(type) {
	case int:
		itm.Value = v + 1
	case int64:
		itm.Value = v + 1
	case int32:
		itm.Value = v + 1
	case uint:
		itm.Value = v + 1
	case uint32:
		itm.Value = v + 1
	case uint64:
		itm.Value = v + 1
	default:
		return errors.New("value is not a supported integer type")
	}
	return nil
}

// Decr decrements a numeric cache value.
func (self *TMemoryCache) Decr(key string) error {
	self.RLock()
	ele, ok := self.blocks[key]
	self.RUnlock()

	if !ok {
		return errors.New("key not exist")
	}
	itm := ele.Value.(*cacher.CacheBlock)
	itm.LastAccess = itm.LastAccess.Add(cacher.DELAY_TIME * time.Second)
	switch v := itm.Value.(type) {
	case int:
		itm.Value = v - 1
	case int64:
		itm.Value = v - 1
	case int32:
		itm.Value = v - 1
	case uint:
		if v == 0 {
			return errors.New("item val is less than 0")
		}
		itm.Value = v - 1
	case uint32:
		if v == 0 {
			return errors.New("item val is less than 0")
		}
		itm.Value = v - 1
	case uint64:
		if v == 0 {
			return errors.New("item val is less than 0")
		}
		itm.Value = v - 1
	default:
		return errors.New("item val is not int int64 int32")
	}
	return nil
}

// Exists returns true if the key is present in the cache.
func (self *TMemoryCache) Exists(name string, ctx ...context.Context) bool {
	if !self.config.Active {
		return false
	}
	self.RLock()
	ele := self.blocks[name]
	self.RUnlock()
	return ele != nil
}

// Len returns the number of cached items.
func (self *TMemoryCache) Len() int {
	self.RLock()
	defer self.RUnlock()
	return len(self.blocks)
}

// Size gets or sets the max cache size.
func (self *TMemoryCache) Size(max ...int) int {
	if len(max) > 0 {
		self.config.Size = max[0]
	}
	return self.config.Size
}

// vaccuum runs periodic GC using a ticker and exits on stopCh.
func (self *TMemoryCache) vaccuum() {
	ticker := time.NewTicker(self.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if self.config.Active {
				self.gcOnce()
			}
		case <-self.stopCh:
			return
		}
	}
}

// gcOnce performs one GC pass under a single Lock.
func (self *TMemoryCache) gcOnce() {
	self.Lock()
	if self.gcList.Len() == 0 {
		self.Unlock()
		return
	}

	var keysToDelete []string
	var nearExpiry TIndexList
	now := time.Now()

	for iter := self.gcList.Front(); iter != nil; {
		next := iter.Next()
		block, ok := iter.Value.(*cacher.CacheBlock)
		if !ok {
			// non-block entry (stack mode): remove on expiry cycle
			self.gcList.Remove(iter)
			iter = next
			continue
		}

		TTL := block.Ttl()
		if TTL > 0 && now.After(block.LastAccess.Add(TTL)) {
			self.gcList.Remove(iter)
			keysToDelete = append(keysToDelete, block.Key)
		} else if TTL > 0 {
			if dur := now.Sub(block.LastAccess); dur < TTL/3 {
				nearExpiry = append(nearExpiry, TIndex{iter, block, dur})
			}
		}
		iter = next
	}

	// Evict overflow entries, preferring recently-accessed ones.
	if over := self.gcList.Len() - self.config.Size; over > 0 {
		sort.Sort(nearExpiry)
		for _, idx := range nearExpiry {
			if over <= 0 {
				break
			}
			self.gcList.Remove(idx.ele)
			keysToDelete = append(keysToDelete, idx.block.Key)
			over--
		}
	}

	for _, key := range keysToDelete {
		delete(self.blocks, key)
	}
	self.Unlock()
}

func (self *TMemoryCache) String() string { return "memory" }

func (self *TMemoryCache) Active(on ...bool) bool {
	if len(on) > 0 {
		self.config.Active = on[0]
	}
	return self.config.Active
}

func (self *TMemoryCache) Refresh(key string) {}

func (self TIndexList) Len() int           { return len(self) }
func (self TIndexList) Swap(i, j int)      { self[i], self[j] = self[j], self[i] }
func (self TIndexList) Less(i, j int) bool { return self[i].expired < self[j].expired }
