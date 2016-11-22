# cache

it is a cache interface and demo.

Memory cacher API
```
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
```
