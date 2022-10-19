package cacher

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
)

const (
	MAX_CACHE     = 1000  //
	EXPIRED_TIME  = 86400 // default expire time of cacahe
	INTERVAL_TIME = 30    // interval of gc time
	DELAY_TIME    = 10
)

type CacherType byte

func (self CacherType) String() string {
	return typeString[self]
}

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
type (
	ICacher interface {
		String() string // type of cacher memory,file etc.
		Init(opts ...Option)
		Active(open ...bool) bool

		// increase cached int value by key, as a counter.
		//Incr(key string) error
		// decrease cached int value by key, as a counter.
		//Decr(key string) error

		//Expired(name string) bool

		// get all items
		//All() []interface{}
		// start gc routine based on config string settings.
		//GC(config string) error
		//Max(max ...int) int

		//*** Std Attr ***
		// below method will change element's order to front of the list
		Get(key string, value interface{}, ctx ...context.Context) error // get cached value by key.
		Set(block *CacheBlock) error                                     // set cached value with key and expire time.
		Exists(key string, ctx ...context.Context) bool                  // check if cached value exists or not.
		Delete(key string, ctx ...context.Context) error                 // delete cached value by key.
		//Refresh(key string)
		Keys(ctx ...context.Context) []string
		Len() int
		Clear() error // clear all cache.
		Close() error
	}
)

var adapters = make(map[CacherType]func() ICacher)
var names = make(map[string]CacherType)
var typeString = make(map[CacherType]string)

// Register makes a cache adapter available by the adapter name.
// If Register is called twice with the same name or if driver is nil,
// it panics.
func Register(name string, adapter func() ICacher) CacherType {
	if adapter == nil {
		panic("cache: Register adapter is nil")
	}
	name = strings.ToLower(name)
	h := hashName(name)
	adapters[h] = adapter
	typeString[h] = name
	names[name] = h
	return h
}

//
// Create a new cache driver by adapter name and config string.
// config need to be correct JSON as string: {"interval":360}.
// it will start gc automatically.
func New(name string) (cacher ICacher, e error) {
	name = strings.ToLower(name)
	typ, ok := names[name]
	if !ok {
		return nil, fmt.Errorf("unknown adapter name of cacher %s which the adapter pkg must imported!", name)
	}
	adapter := adapters[typ]
	return adapter(), nil
}

func hashName(s string) CacherType {
	v := int(crc32.ChecksumIEEE([]byte(s))) // 输入一个字符等到唯一标识
	if v >= 0 {
		return CacherType(v)
	}
	if -v >= 0 {
		return CacherType(-v)
	}
	// v == MinInt
	return CacherType(0)
}
