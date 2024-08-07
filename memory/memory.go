package memory

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
	"unsafe"

	"github.com/jinzhu/copier"
	"github.com/volts-dev/cacher"
)

var Memory = cacher.Register("Memory", func() cacher.ICacher {
	return New()
})

type (
	emptyAny struct {
		typ, val unsafe.Pointer
	}

	TIndex struct {
		ele     *list.Element
		block   *cacher.CacheBlock
		expired time.Duration
	}

	TIndexList []TIndex

	ListCache interface {
		cacher.ICacher
		//*** List Attr ***
		// get first one
		Front() *cacher.CacheBlock
		Back() *cacher.CacheBlock
		MoveToFront(key string)
		MoveToBack(key string)
	}
	StackCache interface {
		//cacher.ICacher
		//*** Stack Attr ***
		Push(any) error //方法可向数组的末尾添加一个或多个元素，并返回新的长度。
		Shift() any     //方法用于把数组的第一个元素从其中删除，并返回第一个元素的值。
		Pop() any       // 移出最后一个入栈的并返回它
		//New(New func() interface{})
	}

	// Memory cache adapter.
	// it contains a RW locker for safe map storage.
	TMemoryCache struct {
		sync.RWMutex
		config *Config
		//dur     time.Duration            // GC 时间间隔
		//expired time.Duration            // #默认缓存过期时间
		time_  []time.Time              // #完全清空时间
		blocks map[string]*list.Element //*cacher.CacheBlock
		new    func() interface{}
		Every  int //废弃 run an expiration check Every clock time

		blockPool sync.Pool
	}
)

// New returns a new MemoryCache.
func New(opts ...cacher.Option) *TMemoryCache {
	cfg := &Config{
		Active:   true,
		Interval: cacher.INTERVAL_TIME * time.Second, //#必须防止0间隔
		Expire:   cacher.EXPIRED_TIME * time.Second,
		Size:     cacher.MAX_CACHE,
	}
	cfg.Init(opts...)

	c := &TMemoryCache{
		config: cfg,
		//dur:     cacher.INTERVAL_TIME * time.Second,
		//expired: cacher.EXPIRED_TIME * time.Second,
		blocks: make(map[string]*list.Element),
	}

	c.blockPool.New = func() any { return &cacher.CacheBlock{} }

	/* Interval等于0不回收 */
	if cfg.Interval > 0 {
		err := c.gc()
		if err != nil {
			c = nil
		}
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

func (self *TMemoryCache) ___New(fn func() interface{}) {
	self.new = fn
}

// Get cache from memory.
// return slice
func (self *TMemoryCache) Keys(ctx ...context.Context) []string {
	if self.config.Active {
		self.RLock()
		defer self.RUnlock()

		len := len(self.blocks)
		keys := make([]string, len)
		idx := 0
		for k := range self.blocks {
			if idx >= len {
				break
			}
			keys[idx] = k
			idx++
		}

		return keys
	}

	return nil
}

func (self *TMemoryCache) Close() error {
	return self.Clear()
}

// delete all cache in memory.
func (self *TMemoryCache) Clear() error {
	self.config.GcListLock.Lock()
	self.config.GcList.Init() // 初始化列表
	self.config.GcListLock.Unlock()

	self.Lock()
	self.blocks = make(map[string]*list.Element)
	self.Unlock()
	return nil
}

// get first one
func (self *TMemoryCache) Front() *cacher.CacheBlock {
	if self.config.Active {
		self.config.GcListLock.RLock()
		block := self.config.GcList.Front().Value.(*cacher.CacheBlock)
		self.config.GcListLock.RUnlock()
		block.LastAccess = time.Now()
		return block
	}

	return nil
}

func (self *TMemoryCache) Back() *cacher.CacheBlock {
	if self.config.Active {
		self.config.GcListLock.RLock()
		block := self.config.GcList.Back().Value.(*cacher.CacheBlock)
		self.config.GcListLock.RUnlock()
		block.LastAccess = time.Now()
		return block
	}

	return nil
}

func (self *TMemoryCache) MoveToFront(key string) {
	self.RLock()
	ele, _ := self.blocks[key]
	self.RUnlock()

	if ele != nil {
		self.config.GcListLock.Lock()
		self.config.GcList.MoveToFront(ele)
		self.config.GcListLock.Unlock()
	}
}

func (self *TMemoryCache) MoveToBack(key string) {
	self.RLock()
	ele, _ := self.blocks[key]
	self.RUnlock()

	if ele != nil {
		self.config.GcListLock.Lock()
		self.config.GcList.MoveToBack(ele)
		self.config.GcListLock.Unlock()
	}
}

type emptyInterface struct {
	typ  *struct{}
	word unsafe.Pointer
}

// Get cache from memory.
// if non-existed or expired, return nil.
func (self *TMemoryCache) Get(name string, value any, ctx ...context.Context) error {
	if !self.config.Active {
		return cacher.ErrInactive
	}

	self.RLock()
	ele, ok := self.blocks[name]
	self.RUnlock()

	if ok && ele != nil {
		if block, ok := ele.Value.(*cacher.CacheBlock); ok {
			block.LastAccess = time.Now()

			self.config.GcListLock.Lock()
			self.config.GcList.MoveToFront(ele)
			self.config.GcListLock.Unlock()

			// 实现任何类型复制
			/*
				// TODO 需要更多验证稳定性
				// FIXME 无法复制结构体内的字符串

				dst := (*emptyAny)(unsafe.Pointer(&value)).val
				src := (*emptyAny)(unsafe.Pointer(&block.Value)).val
				*(*emptyAny)(unsafe.Pointer(dst)) = *(*emptyAny)(unsafe.Pointer(src))
			*/
			err := copier.Copy(value, block.Value)
			return err
		}
	}

	return cacher.ErrCacheMiss
}

// Put cache to memory.
// if expired is 0, it will be cleaned by next gc operation ( default gc clock is 1 minute).
// expired is -1 mean never expire
func (self *TMemoryCache) Set(block *cacher.CacheBlock) error {
	if !self.config.Active || self.config.GcList.Len() >= self.config.Size {
		return nil
	}
	block.LastAccess = time.Now()

	self.Lock()

	ele, has := self.blocks[block.Key]
	if has {

		ele.Value = block
	} else {
		self.config.GcListLock.Lock()
		item := self.config.GcList.PushFront(block) //之前  self.config.GcList.PushBack(block)
		self.config.GcListLock.Unlock()

		self.blocks[block.Key] = item
	}

	self.Unlock()
	return nil
}

// 删除第一个元素
func (self *TMemoryCache) Shift() any {
	if !self.config.Active {
		return nil
	}

	self.config.GcListLock.Lock()
	ele := self.config.GcList.Front()
	if ele == nil {
		self.config.GcListLock.Unlock()
		return nil
	}
	self.config.GcList.Remove(ele)
	self.config.GcListLock.Unlock()

	if block, ok := ele.Value.(*cacher.CacheBlock); ok {
		value := block.Value
		block.Key = ""
		block.Value = nil
		self.blockPool.Put(block)
		return value
	}

	return nil
}

// get and delete last item from the src
func (self *TMemoryCache) Pop() any {
	if !self.config.Active {
		return nil
	}

	self.config.GcListLock.Lock()
	ele := self.config.GcList.Back()
	if ele == nil {
		self.config.GcListLock.Unlock()
		return nil
	}
	self.config.GcList.Remove(ele)
	self.config.GcListLock.Unlock()

	if block, ok := ele.Value.(*cacher.CacheBlock); ok {
		value := block.Value
		block.Key = ""
		block.Value = nil
		self.blockPool.Put(block)
		return value
	}

	return nil
}

// put to last of list
func (self *TMemoryCache) Push(value any) error {
	if !self.config.Active || self.config.GcList.Len() >= self.config.Size {
		return nil
	}
	/*if block.Key == "" {
		// random name
		lName := fmt.Sprintf("%v", &block.Value)
		block.Key = lName[2:]

	}*/

	block := self.blockPool.Get().(*cacher.CacheBlock)
	block.LastAccess = time.Now()
	block.Value = value

	self.config.GcListLock.Lock()
	self.config.GcList.PushBack(block)
	self.config.GcListLock.Unlock()
	/*
		self.Lock()
		self.blocks[block.Key] = elm
		self.Unlock()*/
	return nil
}

func (self *TMemoryCache) remove_list(ele *list.Element) {
	self.config.GcListLock.Lock()
	self.config.GcList.Remove(ele)
	self.config.GcListLock.Unlock()
}

func (self *TMemoryCache) remove_block(name string) {
	self.Lock()
	delete(self.blocks, name)
	self.Unlock()
}

// / Delete cache in memory.event a err
func (self *TMemoryCache) Delete(key string, ctx ...context.Context) (err error) {
	self.RLock()
	ele, ok := self.blocks[key]
	self.RUnlock()

	if ok {
		self.remove_list(ele)
		self.remove_block(key)
		//fmt.Print("aa ", name, ok)
	} else {
		return errors.New(fmt.Sprintf("delete key %s is not exist!", key))
	}

	return
}

// Increase cache counter in memory.
// it supports int,int64,int32,uint,uint64,uint32.
func (self *TMemoryCache) Incr(key string) error {
	self.RLock()
	ele, ok := self.blocks[key]
	self.RUnlock()

	if !ok {
		return errors.New(fmt.Sprintf("Incr key %s is not exist!", key))
	}
	itm := ele.Value.(*cacher.CacheBlock)
	itm.LastAccess.Add(cacher.DELAY_TIME * time.Second)
	switch itm.Value.(type) {
	case int:
		itm.Value = itm.Value.(int) + 1
	case int64:
		itm.Value = itm.Value.(int64) + 1
	case int32:
		itm.Value = itm.Value.(int32) + 1
	case uint:
		itm.Value = itm.Value.(uint) + 1
	case uint32:
		itm.Value = itm.Value.(uint32) + 1
	case uint64:
		itm.Value = itm.Value.(uint64) + 1
	default:
		return errors.New("item val is not int int64 int32")
	}
	return nil
}

// Count of cache size
func (self *TMemoryCache) Len() int {
	self.config.GcListLock.RLock()
	defer self.config.GcListLock.RUnlock()
	return len(self.blocks)
}

// max of cache size
func (self *TMemoryCache) Size(max ...int) int {
	if len(max) > 0 {
		self.config.Size = max[0]
	}

	return self.config.Size
}

// Decrease counter in memory.
func (self *TMemoryCache) Decr(key string) error {
	self.RLock()
	ele, ok := self.blocks[key]
	self.RUnlock()

	if !ok {
		return errors.New("key not exist")
	}
	itm := ele.Value.(*cacher.CacheBlock)
	itm.LastAccess.Add(cacher.DELAY_TIME * time.Second)
	switch itm.Value.(type) {
	case int:
		itm.Value = itm.Value.(int) - 1
	case int64:
		itm.Value = itm.Value.(int64) - 1
	case int32:
		itm.Value = itm.Value.(int32) - 1
	case uint:
		if itm.Value.(uint) > 0 {
			itm.Value = itm.Value.(uint) - 1
		} else {
			return errors.New("item val is less than 0")
		}
	case uint32:
		if itm.Value.(uint32) > 0 {
			itm.Value = itm.Value.(uint32) - 1
		} else {
			return errors.New("item val is less than 0")
		}
	case uint64:
		if itm.Value.(uint64) > 0 {
			itm.Value = itm.Value.(uint64) - 1
		} else {
			return errors.New("item val is less than 0")
		}
	default:
		return errors.New("item val is not int int64 int32")
	}
	return nil
}

// check cache exist in memory.
func (self *TMemoryCache) Exists(name string, ctx ...context.Context) bool {
	if self.config.Active {
		self.RLock()
		ele, _ := self.blocks[name]
		self.RUnlock()

		if ele != nil {
			// # 更新访问日期
			//if block, allowed := ele.Value.(*cacher.CacheBlock); allowed && block != nil {
			//	block.LastAccess.Add(cacher.DELAY_TIME * time.Second)
			//}
			return true

		}

	}
	return false
}

// start memory cache. it will check expiration in every clock time.
func (self *TMemoryCache) gc() error {
	//self.Every = self.config.Interval // 废弃
	//self.dur = dur
	//self.expired = expired

	go self.vaccuum()
	return nil
}

func (self *TMemoryCache) next_time() time.Duration {
	//for self.
	return 0
}

// check expiration.
func (self *TMemoryCache) vaccuum() {
	var (
		list  TIndexList
		block *cacher.CacheBlock
		ok    bool
	)

	for {
		<-time.After(self.config.Interval)

		//fmt.Println("tick")
		if !self.config.Active || self.config.GcList.Len() == 0 {
			continue
		}

		list = make(TIndexList, 0)
		// STEP:遍历GC表
		self.config.GcListLock.RLock()
		iter := self.config.GcList.Front()
		self.config.GcListLock.RUnlock()
		for iter != nil {
			if iter == nil {
				break // 结束回收
			}

			self.config.GcListLock.RLock()
			next := iter.Next()
			self.config.GcListLock.RUnlock()

			/* stack类非block类定时*/
			if block, ok = iter.Value.(*cacher.CacheBlock); !ok {
				self.remove_list(iter)
				iter = next
				continue
			}

			// -1 永不过期
			TTL := block.Ttl()
			if TTL <= 0 {
				iter = next
				continue
			}

			// STEP:删除过期
			//dur := time.Now().Sub(block.LastAccess)
			//fmt.Println("expired %v ", block.expired, dur, self.expired)
			//if dur >= block.TTL || dur >= self.expired {
			if time.Now().After(block.LastAccess.Add(TTL)) {
				//iter = iter.Next() // # before remove
				self.remove_list(iter)
				self.remove_block(block.Key)
				//fmt.Print("aa ", block.key, ok)
				iter = next
				continue
			} else {
				dur := time.Now().Sub(block.LastAccess)
				//if dur < TTL/3 || dur < self.expired/3 {
				if dur < TTL/3 {
					// #因为设置会插入到前端，对即将到期的进行标记
					list = append(list, TIndex{iter, block, dur})
				}
			}

			// #jump to next
			iter = next
		}

		// # 删除即将到期
		if over := self.config.GcList.Len() - self.config.Size; over > 0 {
			sort.Sort(list)

			for _, idex := range list {
				if over > 0 {
					self.remove_list(idex.ele)
					self.remove_block(idex.block.Key)

					//#继续
					over--
					continue
				}

				break
			}
		}
	}
}

// IsExpired returns true if an item is expired.
func (self *TMemoryCache) __Expired(name string) bool {
	self.RLock()
	ele, ok := self.blocks[name]
	self.RUnlock()

	if !ok {
		return true
	}

	itm := ele.Value.(*cacher.CacheBlock)
	// -1 永不过期
	if itm.TTL == -1 {
		return false
	}

	if time.Now().Sub(itm.LastAccess) >= itm.TTL {
		/*self.Lock()
		delete(self.blocks, name)
		self.Unlock()*/
		//self.Remove(name)
		return true
	}
	itm.LastAccess.Add(cacher.DELAY_TIME * time.Second)
	return false
}

func (self *TMemoryCache) String() string {
	return "memory"
}

func (self *TMemoryCache) Active(on ...bool) bool {
	if len(on) > 0 {
		self.config.Active = on[0]
	}

	return self.config.Active
}

func (self *TMemoryCache) Refresh(key string) {

}

func (self TIndexList) Len() int {
	return len(self)
}

func (self TIndexList) Swap(i, j int) {
	self[i], self[j] = self[j], self[i]
}

func (self TIndexList) Less(i, j int) bool {
	return self[i].expired < self[j].expired
}
