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
func TestGetSkippingLocalCache(t *testing.T) {
	local := memory.New()
	rc := New(WithLocalCacher(local))

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

// TestWithRedis_WiresClient guards against a regression where WithRedis/WithLocalCacher
// silently failed to wire self.cli/self.LocalCache: those are unexported fields, and
// Config.Init's generic path (SetByField into a dataset record, then AsStruct/mapstructure
// decoding it back onto the concrete struct) only ever writes exported fields — so cli
// stayed nil forever and every Set() hit the "both Redis and LocalCache are nil" error,
// with no visible symptom other than data silently never reaching Redis. Found 2026-07-24
// debugging cross-process session sharing: SetSession reported success while Redis stayed
// empty. Config.Init now pulls "cli"/"local_cache" back out of the record explicitly.
func TestWithRedis_WiresClient(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	r := New(WithRedis(rdb))

	key := "TestWithRedis_WiresClient"
	defer r.Delete(key)

	if err := r.Set(&cacher.CacheBlock{Key: key, Value: []byte("hello")}); err != nil {
		t.Fatalf("Set returned error (cli likely nil): %v", err)
	}

	b, err := r.GetRaw(key)
	if err != nil {
		t.Fatalf("GetRaw returned error: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("GetRaw = %q, want %q", b, "hello")
	}
}
