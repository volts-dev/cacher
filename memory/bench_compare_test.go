// bench_compare_test.go
// 横向性能对比: volts memory vs go-cache vs bigcache vs freecache vs ristretto
//
// 运行:
//   go test ./memory/... -bench=. -benchmem -benchtime=5s -count=1
//   go test ./memory/... -bench=. -benchmem -benchtime=5s -count=3 | benchstat /dev/stdin
//
// 场景:
//   Set         - 单 goroutine 写入
//   Get_Hit     - 单 goroutine 读命中
//   Get_Miss    - 单 goroutine 读未命中
//   Parallel    - GOMAXPROCS 并发混合读写

package memory

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	bigcache "github.com/allegro/bigcache/v3"
	"github.com/coocood/freecache"
	"github.com/dgraph-io/ristretto"
	gocache "github.com/patrickmn/go-cache"
	"github.com/volts-dev/cacher"
)

// ─── 公共 key 生成 ─────────────────────────────────────────────────────────────

const benchSize = 100_000

var prebuiltKeys [benchSize]string

func init() {
	for i := 0; i < benchSize; i++ {
		prebuiltKeys[i] = "key:" + strconv.Itoa(i)
	}
}

func key(i int) string { return prebuiltKeys[i%benchSize] }
func val(i int) []byte { return []byte(fmt.Sprintf("value-%d", i)) }

// ─── volts memory ─────────────────────────────────────────────────────────────

func BenchmarkVolts_Set(b *testing.B) {
	c := New(WithSize(b.N + 1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(&cacher.CacheBlock{Key: key(i), Value: val(i)})
	}
}

func BenchmarkVolts_Get_Hit(b *testing.B) {
	c := New(WithSize(benchSize + 1))
	for i := 0; i < benchSize; i++ {
		c.Set(&cacher.CacheBlock{Key: key(i), Value: val(i)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkVolts_Get_Miss(b *testing.B) {
	c := New(WithSize(benchSize + 1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkVolts_Parallel(b *testing.B) {
	c := New(WithSize(benchSize + 1))
	for i := 0; i < benchSize; i++ {
		c.Set(&cacher.CacheBlock{Key: key(i), Value: val(i)})
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%4 == 0 {
				c.Set(&cacher.CacheBlock{Key: key(i), Value: val(i)})
			} else {
				c.Get(key(i))
			}
			i++
		}
	})
}

// ─── go-cache (patrickmn) ─────────────────────────────────────────────────────

func BenchmarkGoCache_Set(b *testing.B) {
	c := gocache.New(5*time.Minute, 10*time.Minute)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(key(i), val(i), gocache.DefaultExpiration)
	}
}

func BenchmarkGoCache_Get_Hit(b *testing.B) {
	c := gocache.New(5*time.Minute, 10*time.Minute)
	for i := 0; i < benchSize; i++ {
		c.Set(key(i), val(i), gocache.DefaultExpiration)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkGoCache_Get_Miss(b *testing.B) {
	c := gocache.New(5*time.Minute, 10*time.Minute)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkGoCache_Parallel(b *testing.B) {
	c := gocache.New(5*time.Minute, 10*time.Minute)
	for i := 0; i < benchSize; i++ {
		c.Set(key(i), val(i), gocache.DefaultExpiration)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%4 == 0 {
				c.Set(key(i), val(i), gocache.DefaultExpiration)
			} else {
				c.Get(key(i))
			}
			i++
		}
	})
}

// ─── bigcache ─────────────────────────────────────────────────────────────────

func newBigCache(b testing.TB) *bigcache.BigCache {
	b.Helper()
	cfg := bigcache.DefaultConfig(5 * time.Minute)
	cfg.Verbose = false
	c, err := bigcache.New(context.Background(), cfg)
	if err != nil {
		b.Fatal(err)
	}
	return c
}

func BenchmarkBigCache_Set(b *testing.B) {
	c := newBigCache(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(key(i), val(i))
	}
}

func BenchmarkBigCache_Get_Hit(b *testing.B) {
	c := newBigCache(b)
	for i := 0; i < benchSize; i++ {
		c.Set(key(i), val(i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkBigCache_Get_Miss(b *testing.B) {
	c := newBigCache(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkBigCache_Parallel(b *testing.B) {
	c := newBigCache(b)
	for i := 0; i < benchSize; i++ {
		c.Set(key(i), val(i))
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%4 == 0 {
				c.Set(key(i), val(i))
			} else {
				c.Get(key(i))
			}
			i++
		}
	})
}

// ─── freecache ────────────────────────────────────────────────────────────────

func BenchmarkFreeCache_Set(b *testing.B) {
	c := freecache.NewCache(256 * 1024 * 1024) // 256 MB
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set([]byte(key(i)), val(i), 300)
	}
}

func BenchmarkFreeCache_Get_Hit(b *testing.B) {
	c := freecache.NewCache(256 * 1024 * 1024)
	for i := 0; i < benchSize; i++ {
		c.Set([]byte(key(i)), val(i), 300)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get([]byte(key(i)))
	}
}

func BenchmarkFreeCache_Get_Miss(b *testing.B) {
	c := freecache.NewCache(256 * 1024 * 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get([]byte(key(i)))
	}
}

func BenchmarkFreeCache_Parallel(b *testing.B) {
	c := freecache.NewCache(256 * 1024 * 1024)
	for i := 0; i < benchSize; i++ {
		c.Set([]byte(key(i)), val(i), 300)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%4 == 0 {
				c.Set([]byte(key(i)), val(i), 300)
			} else {
				c.Get([]byte(key(i)))
			}
			i++
		}
	})
}

// ─── ristretto ────────────────────────────────────────────────────────────────

func newRistretto(b testing.TB) *ristretto.Cache {
	b.Helper()
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: benchSize * 10,
		MaxCost:     1 << 28, // 256 MB
		BufferItems: 64,
	})
	if err != nil {
		b.Fatal(err)
	}
	return c
}

func BenchmarkRistretto_Set(b *testing.B) {
	c := newRistretto(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Set(key(i), val(i), 1)
	}
}

func BenchmarkRistretto_Get_Hit(b *testing.B) {
	c := newRistretto(b)
	for i := 0; i < benchSize; i++ {
		c.Set(key(i), val(i), 1)
	}
	c.Wait() // 等待异步写入完成
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkRistretto_Get_Miss(b *testing.B) {
	c := newRistretto(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(key(i))
	}
}

func BenchmarkRistretto_Parallel(b *testing.B) {
	c := newRistretto(b)
	for i := 0; i < benchSize; i++ {
		c.Set(key(i), val(i), 1)
	}
	c.Wait()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%4 == 0 {
				c.Set(key(i), val(i), 1)
			} else {
				c.Get(key(i))
			}
			i++
		}
	})
}
