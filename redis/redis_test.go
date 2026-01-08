package redis

import (
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/volts-dev/cacher"
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
