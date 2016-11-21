package cache

import (
	"fmt"
)

const (
	MAX_CACHE     = 1000  //
	EXPIRED_TIME  = 86400 // default expire time of cacahe
	INTERVAL_TIME = 30    // interval of gc time
	DELAY_TIME    = 10
)

// Cache interface contains all behaviors for cache adapter.
// usage:
//	cache.Register("file",cache.NewFileCache()) // this operation is run in init method of file.go.
//	c := cache.NewCache("file","{....}")
//	c.Put("key",value,3600)
//	v := c.Get("key")
//
//	c.Incr("counter")  // now is 1
//	c.Incr("counter")  // now is 2
//	count := c.Get("counter").(int)
type ICache interface {
	Active(open ...bool) bool
	// get cached value by key.
	Get(key string) interface{}
	// set cached value with key and expire time.
	Put(key string, val interface{}, timeout ...int64) error
	// delete cached value by key.
	//Delete(key string) error
	Remove(key string) error
	// increase cached int value by key, as a counter.
	Incr(key string) error
	// decrease cached int value by key, as a counter.
	Decr(key string) error
	// check if cached value exists or not.
	IsExist(key string) bool
	// clear all cache.
	Clear() error
	// get all items
	All() []interface{}
	// start gc routine based on config string settings.
	GC(config string) error
	Max(max ...int) int
	Len() int

	//*** List Attr ***
	// get first one
	Front() interface{}
	Back() interface{}
	//MoveToFront()
	//MoveToBack()

	//*** Stack Attr ***
	Push(value interface{}, expired ...int64) error //方法可向数组的末尾添加一个或多个元素，并返回新的长度。
	Shift() interface{}                             //方法用于把数组的第一个元素从其中删除，并返回第一个元素的值。
	Pop() interface{}                               // 移出最后一个入栈的并返回它

}

var adapters = make(map[string]func() ICache)

// Register makes a cache adapter available by the adapter name.
// If Register is called twice with the same name or if driver is nil,
// it panics.
func Register(name string, adapter func() ICache) {
	if adapter == nil {
		panic("cache: Register adapter is nil")
	}
	if _, dup := adapters[name]; dup {
		panic("cache: Register called twice for adapter " + name)
	}
	adapters[name] = adapter
}

//
// Create a new cache driver by adapter name and config string.
// config need to be correct JSON as string: {"interval":360}.
// it will start gc automatically.
func NewCacher(adapterName, config string) (cacher ICache, e error) {
	adapter, ok := adapters[adapterName]
	if !ok {
		e = fmt.Errorf("cache: unknown adapter name %q (forgot to import?)", adapterName)
		return
	}
	cacher = adapter()
	err := cacher.GC(config)
	if err != nil {
		cacher = nil
	}
	return
}
