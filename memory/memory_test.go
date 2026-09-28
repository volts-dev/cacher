package memory

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/volts-dev/cacher"
	"github.com/volts-dev/utils"
)

func TestStackCache(t *testing.T) {
	c := NewStack(WithExpire(30), WithSize(2))
	c.Push("你")
	c.Push("好")
	c.Push("啊!")
	fmt.Println(c.Pop(), c.Pop(), c.Pop())
}

func TestWithStruct(t *testing.T) {
	type A struct {
		Int    int
		String string
	}
	chr := New()
	chr.Set(&cacher.CacheBlock{Key: "A", Value: &A{Int: 123456789, String: "fsdf"}})

	a, err := chr.Get("A")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(a)
}

func TestConfig(t *testing.T) {
	cfg := Config{}
	cfg.Init(
		WithInterval(15),
	)
	fmt.Println(cfg)
}

func TestStd(t *testing.T) {
	chr := New()
	count := 5
	var w sync.WaitGroup
	w.Add(count)
	for i := 0; i < count; i++ {
		go func(idx int, ck cacher.ICacher) {
			for k := 0; k < 1000; k++ {
				str := utils.ToString(idx) + "Name" + utils.ToString(k)
				ck.Set(&cacher.CacheBlock{Key: utils.ToString(idx) + "+" + utils.ToString(k), Value: str})
			}
			w.Done()
		}(i, chr)
	}

	w.Wait() //# 等待GO程初始化
	if chr.Len() != count*1000 {
		t.Logf("Put was error %v/%d", chr.Len(), count*1000)
	}

	for i := 0; i < count; i++ {
		for k := 0; k < 1000; k++ {
			ele, err := chr.Get(utils.ToString(i) + "+" + utils.ToString(k))
			if err != nil {
				t.Fatal(err)
			}
			fmt.Printf("%d=%v\n", i, ele)
		}
	}
	/*
		for i := 0; i < 100; i++ {
			go func(idx int, ck cacher.ICacher) {
				for {
					<-time.After(1 * time.Second)
					for k := 0; k < 10000; k++ {
						str := ""
						var ok bool
						itf := ck.Pop()
						if itf != nil {
							if str, ok = itf.(string); str == "" || !ok {
								//fmt.Println("Hello World")
								str = "Name"
							}
						}

						//ck.Put(fmt.Sprintf("%v%v", idx, k), str)
						ck.Push(str, int64(k))
					}
				}

			}(i, chr)
		}
	*/

	fmt.Println(chr.Keys())
	//fmt.Printf("active=%v  max=%v", cacher.Active(), cacher.Max())
	fmt.Println()
	//fmt.Printf("Pop %v,%v/5000 ", cacher.Pop(), cacher.Len())
	fmt.Println()
	//	fmt.Printf("Shift %v,%v/5000 ", cacher.Shift(), cacher.Len())
	fmt.Println()
	fmt.Println(chr.Active())
}

// TestConcurrentDelete verifies that concurrent deletes of the same key do not panic or deadlock.
func TestConcurrentDelete(t *testing.T) {
	c := New(WithSize(100))
	key := "shared-key"
	c.Set(&cacher.CacheBlock{Key: key, Value: "v"})

	var wg sync.WaitGroup
	const workers = 20
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			c.Delete(key) // only one succeeds, rest get error — both are valid
		}()
	}
	wg.Wait()
	if c.Exists(key) {
		t.Fatal("key still exists after concurrent deletes")
	}
}

// TestGCAndSet_Concurrent runs GC and Set concurrently to detect deadlocks or races.
func TestGCAndSet_Concurrent(t *testing.T) {
	c := New(WithSize(50), WithInterval(1))
	const workers = 10
	const ops = 100

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			for k := 0; k < ops; k++ {
				key := fmt.Sprintf("gc-%d-%d", id, k)
				c.Set(&cacher.CacheBlock{Key: key, Value: id})
				c.Get(key)
			}
		}(i)
	}
	wg.Wait()
}

// Expire 只改存活期，不碰值 —— 会话续期就是靠这一点避开"读-改-写盖掉并发改写"，
// 那条路的代价是用户随机被登出（见 redis/redis.go 里 Expire 上方的说明）。
func TestExpireExtendsWithoutTouchingTheValue(t *testing.T) {
	c := New()
	if err := c.Set(&cacher.CacheBlock{Key: "sid", Value: "payload", TTL: time.Minute}); err != nil {
		t.Fatal(err)
	}

	if err := c.Expire("sid", time.Hour); err != nil {
		t.Fatalf("Expire: %v", err)
	}

	v, err := c.Get("sid")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "payload" {
		t.Fatalf("值被改了: %v", v)
	}

	c.RLock()
	block := c.blocks["sid"].Value.(*cacher.CacheBlock)
	c.RUnlock()
	if block.TTL != time.Hour {
		t.Fatalf("TTL=%s，没有被延长", block.TTL)
	}
}

// 键不存在必须报得出来：redis 的 EXPIRE 对不存在的键回 0（不是错误），
// 调用方要能分辨"续上了"和"这条已经没了"。
func TestExpireOnMissingKey(t *testing.T) {
	if err := New().Expire("nobody", time.Hour); err != cacher.ErrCacheMiss {
		t.Fatalf("期待 ErrCacheMiss，得到 %v", err)
	}
}

// 非正的 TTL 在 redis 那边等于"立刻删掉"，而调用 Expire 的意图永远是续期。
func TestExpireRejectsNonPositiveTTL(t *testing.T) {
	c := New()
	c.Set(&cacher.CacheBlock{Key: "sid", Value: "x", TTL: time.Minute})
	if err := c.Expire("sid", 0); err == nil {
		t.Fatal("TTL=0 应该报错，而不是把这条会话删掉")
	}
}
