package redis

import (
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/volts-dev/cacher"
	"github.com/volts-dev/cacher/memory"
)

func TestBase(t *testing.T) {
	Key := "Test"
	rdb := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	r := New(
		WithRedis(rdb),
	)
	r.Set(&cacher.CacheBlock{
		Key:   Key,
		Value: "TestBase",
	})

	s, err := r.Get(Key)
	if err != nil {
		t.Fatal(err)
	}

	if s != "" {
		r.Delete(Key)
	}
	t.Log(s)
}

// TestGetSkippingLocalCache verifies that skipLocalCache=true bypasses the local cache
// and that the default Get path uses the local cache when available.
// Note: config.LocalCache is assigned directly because WithLocalCacher relies on
// reflection-based field mapping that does not handle interface types correctly;
// that issue is tracked separately and is out of scope here.
func TestGetSkippingLocalCache(t *testing.T) {
	local := memory.New()
	rc := New()
	rc.config.LocalCache = local // bypass reflection — see note above

	// Seed via rc.Set so the value goes through marshal→compress (adds compression byte).
	// String/[]byte values are stored raw without a compression byte and cannot be
	// round-tripped via *interface{} unmarshal; use an integer instead.
	// With cli==nil and LocalCache set, Set writes only to the local cache.
	if err := rc.Set(&cacher.CacheBlock{Key: "k1", Value: 42}); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	// Get (skipLocalCache=false) should read from local cache.
	val, err := rc.Get("k1")
	if err != nil {
		t.Fatalf("Get with local cache hit returned error: %v", err)
	}
	if val == nil {
		t.Fatal("expected non-nil value from local cache")
	}

	// GetSkippingLocalCache (skipLocalCache=true) bypasses local cache.
	// No Redis client configured → ErrCacheMiss.
	_, err = rc.GetSkippingLocalCache("k1")
	if err != cacher.ErrCacheMiss {
		t.Fatalf("GetSkippingLocalCache expected ErrCacheMiss, got: %v", err)
	}
}
