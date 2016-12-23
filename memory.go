package cache

import (
	"container/list"
	"encoding/json"
	"sort"
	//	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	//	"webgo/utils"
)

var (
	// clock time of recycling the expired cache items in memory.
	DefaultEvery int = 360 // 1 minute
)

type (
	TIndex struct {
		ele     *list.Element
		block   *TCacheBlock
		expired time.Duration
	}

	TIndexList []TIndex

	// Memory cache item.
	TCacheBlock struct {
		key        string
		val        interface{}   // Value
		Lastaccess time.Time     // last update time
		expired    time.Duration // interval
	}

	// Memory cache adapter.
	// it contains a RW locker for safe map storage.
	TMemoryCache struct {
		dur         time.Duration            // GC 时间间隔
		expired     time.Duration            // #默认缓存过期时间
		time_       []time.Time              // #完全清空时间
		blocks      map[string]*list.Element //*TCacheBlock
		gcList      *list.List               // 	// 垃圾回收 store all of sessions for gc
		max         int
		active      bool
		new         func() interface{}
		Every       int //废弃 run an expiration check Every clock time
		lock        sync.RWMutex
		gcList_lock sync.RWMutex
	}
)

// NewMemoryCache returns a new MemoryCache.
func NewMemoryCache() ICacher {
	cacher := &TMemoryCache{
		dur:     INTERVAL_TIME * time.Second,
		expired: EXPIRED_TIME * time.Second,
		blocks:  make(map[string]*list.Element),
		max:     MAX_CACHE,
		gcList:  list.New(),
		active:  true,
	}

	return cacher
}
func (self *TMemoryCache) New(New func() interface{}) {
	self.new = New
}

// Get cache from memory.
// return slice
func (self *TMemoryCache) All() (list []interface{}) {
	if self.active {
		self.lock.RLock()
		for iter := self.gcList.Front(); iter != nil; iter = iter.Next() {
			//fmt.Println("item:", iter.Value)
			if itm, ok := iter.Value.(*TCacheBlock); ok {
				itm.Lastaccess = time.Now()
				list = append(list, itm.val)
			}
		}
		self.lock.RUnlock()
	}

	return
}

// delete all cache in memory.
func (self *TMemoryCache) Clear() error {
	self.gcList_lock.Lock()
	self.gcList.Init() // 初始化列表
	self.gcList_lock.Unlock()

	self.lock.Lock()
	self.blocks = make(map[string]*list.Element)
	self.lock.Unlock()
	return nil
}

// get first one
func (self *TMemoryCache) Front() interface{} {
	if self.active {
		self.gcList_lock.RLock()
		block := self.gcList.Front().Value.(*TCacheBlock)
		self.gcList_lock.RUnlock()
		block.Lastaccess = time.Now()
		return block.val
	}

	return nil
}

func (self *TMemoryCache) Back() interface{} {
	if self.active {
		self.gcList_lock.RLock()
		block := self.gcList.Back().Value.(*TCacheBlock)
		self.gcList_lock.RUnlock()
		block.Lastaccess = time.Now()
		return block.val
	}

	return nil
}

func (self *TMemoryCache) MoveToFront(key string) {
	self.lock.RLock()
	ele, _ := self.blocks[key]
	self.lock.RUnlock()

	if ele != nil {
		self.gcList_lock.Lock()
		self.gcList.MoveToFront(ele)
		self.gcList_lock.Unlock()
	}
}

func (self *TMemoryCache) MoveToBack(key string) {
	self.lock.RLock()
	ele, _ := self.blocks[key]
	self.lock.RUnlock()

	if ele != nil {
		self.gcList_lock.Lock()
		self.gcList.MoveToBack(ele)
		self.gcList_lock.Unlock()
	}
}

// Get cache from memory.
// if non-existed or expired, return nil.
func (self *TMemoryCache) Get(name string) interface{} {
	self.lock.RLock()
	ele, _ := self.blocks[name]
	self.lock.RUnlock()

	if ele != nil {
		if block, ok := ele.Value.(*TCacheBlock); ok {
			//fmt.Println(block, ok)
			block.Lastaccess = time.Now()

			if ele != nil {
				self.gcList_lock.Lock()
				self.gcList.MoveToFront(ele)
				self.gcList_lock.Unlock()
			}
			return block.val
		}

	}

	return nil

}

// Put cache to memory.
// if expired is 0, it will be cleaned by next gc operation ( default gc clock is 1 minute).
// expired is -1 mean never expire
func (self *TMemoryCache) Put(name string, value interface{}, expired ...int64) error {
	lExpired := self.expired
	if len(expired) > 0 {
		lExpired = time.Duration(expired[0]) * time.Second
	}

	self.lock.RLock()
	ele, ok := self.blocks[name]
	self.lock.RUnlock()

	var block *TCacheBlock
	if ok {
		if block, ok = ele.Value.(*TCacheBlock); ok {
			block.Lastaccess = time.Now()
			block.expired = lExpired
		}

	} else {
		block = &TCacheBlock{
			key:        name,
			val:        value,
			Lastaccess: time.Now(),
			expired:    lExpired,
		}

		self.gcList_lock.Lock()
		lElm := self.gcList.PushFront(block) //之前  self.gcList.PushBack(block)
		self.gcList_lock.Unlock()

		self.lock.Lock()
		self.blocks[name] = lElm
		self.lock.Unlock()
	}

	return nil
}

func (self *TMemoryCache) Shift() interface{} {
	if self.active {
		//fmt.Println(len(self.blocks))
		self.gcList_lock.Lock()
		ele := self.gcList.Front()
		if ele == nil {
			self.gcList_lock.Unlock()
			return nil
		}
		self.gcList.Remove(ele)
		self.gcList_lock.Unlock()

		if block, ok := ele.Value.(*TCacheBlock); ok {
			self.remove_block(block.key)
			return block.val
		}
	}
	return nil
}

// get and delete from the src
func (self *TMemoryCache) Pop() interface{} {
	if self.active {
		//fmt.Println(len(self.blocks))
		self.gcList_lock.Lock()
		ele := self.gcList.Back()
		if ele == nil {
			self.gcList_lock.Unlock()
			return nil
		}
		self.gcList.Remove(ele)
		self.gcList_lock.Unlock()

		if block, ok := ele.Value.(*TCacheBlock); ok {
			self.remove_block(block.key)
			return block.val
		}
	}
	return nil
}

// put to last of list
func (self *TMemoryCache) Push(value interface{}, expired ...int64) error {
	lExpired := self.expired
	if len(expired) > 0 {
		lExpired = time.Duration(expired[0]) * time.Second
	}

	// random name
	lName := fmt.Sprintf("%v", &value)
	lName = lName[2:]
	//fmt.Println(lName)
	block := &TCacheBlock{
		key:        lName,
		val:        value,
		Lastaccess: time.Now(),
		expired:    lExpired,
	}
	self.gcList_lock.Lock()
	lElm := self.gcList.PushBack(block)
	self.gcList_lock.Unlock()

	self.lock.Lock()
	self.blocks[lName] = lElm
	self.lock.Unlock()
	return nil
}

func (self *TMemoryCache) remove_list(ele *list.Element) {
	self.gcList_lock.Lock()
	self.gcList.Remove(ele)
	self.gcList_lock.Unlock()
}

func (self *TMemoryCache) remove_block(name string) {
	self.lock.Lock()
	delete(self.blocks, name)
	self.lock.Unlock()
}

/// Delete cache in memory.event a err
func (self *TMemoryCache) Remove(name string) (err error) {
	self.lock.RLock()
	ele, ok := self.blocks[name]
	self.lock.RUnlock()

	if ok {
		self.remove_list(ele)
		self.remove_block(name)
		//fmt.Print("aa ", name, ok)
	} else {
		return errors.New("key not exist" + name)
	}

	return
}

// Increase cache counter in memory.
// it supports int,int64,int32,uint,uint64,uint32.
func (self *TMemoryCache) Incr(key string) error {
	self.lock.RLock()
	ele, ok := self.blocks[key]
	self.lock.RUnlock()

	if !ok {
		return errors.New("key not exist " + key)
	}
	itm := ele.Value.(*TCacheBlock)
	itm.Lastaccess.Add(DELAY_TIME * time.Second)
	switch itm.val.(type) {
	case int:
		itm.val = itm.val.(int) + 1
	case int64:
		itm.val = itm.val.(int64) + 1
	case int32:
		itm.val = itm.val.(int32) + 1
	case uint:
		itm.val = itm.val.(uint) + 1
	case uint32:
		itm.val = itm.val.(uint32) + 1
	case uint64:
		itm.val = itm.val.(uint64) + 1
	default:
		return errors.New("item val is not int int64 int32")
	}
	return nil
}

// Count of cache size
func (self *TMemoryCache) Len() int {
	self.gcList_lock.RLock()
	defer self.gcList_lock.RUnlock()
	return self.gcList.Len() // len(self.blocks)
}

// max of cache size
func (self *TMemoryCache) Max(max ...int) int {
	if len(max) > 0 {
		self.max = max[0]
	}

	return self.max
}

// Decrease counter in memory.
func (self *TMemoryCache) Decr(key string) error {
	self.lock.RLock()
	ele, ok := self.blocks[key]
	self.lock.RUnlock()

	if !ok {
		return errors.New("key not exist")
	}
	itm := ele.Value.(*TCacheBlock)
	itm.Lastaccess.Add(DELAY_TIME * time.Second)
	switch itm.val.(type) {
	case int:
		itm.val = itm.val.(int) - 1
	case int64:
		itm.val = itm.val.(int64) - 1
	case int32:
		itm.val = itm.val.(int32) - 1
	case uint:
		if itm.val.(uint) > 0 {
			itm.val = itm.val.(uint) - 1
		} else {
			return errors.New("item val is less than 0")
		}
	case uint32:
		if itm.val.(uint32) > 0 {
			itm.val = itm.val.(uint32) - 1
		} else {
			return errors.New("item val is less than 0")
		}
	case uint64:
		if itm.val.(uint64) > 0 {
			itm.val = itm.val.(uint64) - 1
		} else {
			return errors.New("item val is less than 0")
		}
	default:
		return errors.New("item val is not int int64 int32")
	}
	return nil
}

// check cache exist in memory.
func (self *TMemoryCache) Contains(name string) bool {
	if self.active {
		self.lock.RLock()
		ele, _ := self.blocks[name]
		self.lock.RUnlock()

		if ele != nil {
			// # 更新访问日期
			if block, allowed := ele.Value.(*TCacheBlock); allowed && block != nil {
				block.Lastaccess.Add(DELAY_TIME * time.Second)
				return true
			}
		}

	}
	return false
}

// start memory cache. it will check expiration in every clock time.
func (self *TMemoryCache) GC(config string) error {
	cf := make(map[string]int)
	json.Unmarshal([]byte(config), &cf)
	if val, ok := cf["interval"]; !ok || val < 1 {
		//#必须防止0间隔
		cf["interval"] = INTERVAL_TIME
	}

	dur, err := time.ParseDuration(fmt.Sprintf("%ds", cf["interval"]))
	if err != nil {
		return err
	}

	if _, ok := cf["expired"]; !ok {
		cf["expired"] = EXPIRED_TIME
	}

	expired, err := time.ParseDuration(fmt.Sprintf("%ds", cf["expired"]))
	if err != nil {
		return err
	}

	self.Every = cf["interval"]
	self.dur = dur
	self.expired = expired
	//fmt.Println(self.dur, self.expired)
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
		block *TCacheBlock
		ok    bool
	)

	for {
		//fmt.Println("pretick", self.dur)
		list = make(TIndexList, 0)
		<-time.After(self.dur)

		//fmt.Println("tick")
		if !self.active || self.gcList.Len() == 0 {
			continue
		}

		// STEP:遍历GC表
		self.gcList_lock.RLock()
		iter := self.gcList.Front()
		self.gcList_lock.RUnlock()
		for iter != nil {
			if iter == nil {
				break // 结束回收
			}

			self.gcList_lock.RLock()
			next := iter.Next()
			self.gcList_lock.RUnlock()

			//fmt.Println(element)
			// #check
			if block, ok = iter.Value.(*TCacheBlock); !ok {
				fmt.Println("RRR", iter)
				self.remove_list(iter)

				iter = next
				continue
			}

			// -1 永不过期
			if block.expired == -1 {
				iter = next
				continue
			}

			// STEP:删除过期
			dur := time.Now().Sub(block.Lastaccess)
			//fmt.Println("expired %v ", block.expired, dur, self.expired)
			if dur >= block.expired || dur >= self.expired {
				//iter = iter.Next() // # before remove
				self.remove_list(iter)
				self.remove_block(block.key)
				//fmt.Print("aa ", block.key, ok)
				iter = next
				continue
			} else if dur < block.expired/3 || dur < self.expired/3 {
				// #对即将到期的进行标记
				list = append(list, TIndex{iter, block, dur})
			}

			// #jump to next
			iter = next
		}

		// # 删除即将到期
		if over := self.gcList.Len() - self.max; over > 0 {
			sort.Sort(list)

			for _, idex := range list {
				if over > 0 {
					self.remove_list(idex.ele)
					self.remove_block(idex.block.key)

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
func (self *TMemoryCache) Expired(name string) bool {
	self.lock.RLock()
	ele, ok := self.blocks[name]
	self.lock.RUnlock()

	if !ok {
		return true
	}

	itm := ele.Value.(*TCacheBlock)
	// -1 永不过期
	if itm.expired == -1 {
		return false
	}

	if time.Now().Sub(itm.Lastaccess) >= itm.expired {
		/*self.lock.Lock()
		delete(self.blocks, name)
		self.lock.Unlock()*/
		//self.Remove(name)
		return true
	}
	itm.Lastaccess.Add(DELAY_TIME * time.Second)
	return false
}
func (self *TMemoryCache) Type() string {
	return "memory"
}
func (self *TMemoryCache) Active(open ...bool) bool {
	if len(open) > 0 {
		self.active = open[0]
	}

	return self.active
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

func init() {
	Register("memory", NewMemoryCache)
}
