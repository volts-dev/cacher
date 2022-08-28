package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	redis "github.com/go-redis/redis/v8"
	"github.com/klauspost/compress/s2"
	"github.com/vmihailenco/msgpack"
	"github.com/volts-dev/cacher"
)

var (
	errRedisLocalCacheNil = errors.New("cache: both Redis and LocalCache are nil")
)

var Redis cacher.CacherType = cacher.Register("Redis", func() cacher.ICacher {
	return New()
})

type (
	rediser interface {
		Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd
		SetXX(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.BoolCmd
		SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.BoolCmd

		Get(ctx context.Context, key string) *redis.StringCmd
		Del(ctx context.Context, keys ...string) *redis.IntCmd
	}

	RedisCache struct {
		sync.RWMutex
		config *Config
	}
)

func New(opts ...cacher.Option) *RedisCache {
	cfg := &Config{}

	cfg.Init(opts...)
	cacher := &RedisCache{
		config: cfg,
	}

	if cfg.Marshal == nil {
		cfg.Marshal = cacher.marshal
	}

	if cfg.Unmarshal == nil {
		cfg.Unmarshal = cacher.unmarshal
	}
	return cacher
}

func (self *RedisCache) Init(opts ...cacher.Option) {
	self.config.Init(opts...)
}

func (self *RedisCache) String() string {
	return "redis"
}

func (self *RedisCache) Active(open ...bool) bool {
	if len(open) > 0 {
		self.config.Active = open[0]
	}

	return self.config.Active
}

// Count of cache size
func (self *RedisCache) Len() int {
	return 0
}

// Exists reports whether value for the given key exists.
func (self *RedisCache) Exists(key string, ctx ...context.Context) bool {
	return self.Get(key, nil, ctx...) == nil
}

func (self *RedisCache) getKey(key string) string {
	return self.config.prefix + key
}

func (self *RedisCache) getValue(ctx context.Context, sid string) (string, error) {
	cmd := self.config.cli.Get(ctx, self.getKey(sid))
	if err := cmd.Err(); err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}

	return cmd.Val(), nil
}

// Get gets the value for the given key skipping local cache.
func (self *RedisCache) GetSkippingLocalCache(key string, value interface{}, ctx ...context.Context) error {
	return self.get(key, value, true, ctx...)
}

// Get cache from memory.
// if non-existed or expired, return nil.
func (self *RedisCache) Get(key string, value interface{}, ctx ...context.Context) error {
	return self.get(key, value, false, ctx...)
}

func (self *RedisCache) get(key string, value interface{}, skipLocalCache bool, ctx ...context.Context) error {
	var c context.Context
	if ctx != nil {
		c = ctx[0]
	} else {
		c = context.Background()
	}

	b, err := self.getBytes(c, key, true)
	if err != nil {
		return err
	}
	return self.config.Unmarshal(b, value)
}

func (self *RedisCache) Set(block *cacher.CacheBlock) error {
	b, err := self.marshal(block.Value)
	if err != nil {
		return err
	}

	bb := block.Clone()
	bb.Value = b
	if self.config.LocalCache != nil && !block.SkipLocalCache {
		self.config.LocalCache.Set(bb)
	}

	if self.config.cli == nil {
		if self.config.LocalCache == nil {
			return errRedisLocalCacheNil
		}
		return nil
	}

	ttl := block.Ttl()
	if ttl == 0 {
		return nil
	}

	if block.SetOnlyExist {
		return self.config.cli.SetXX(block.Context(), block.Key, b, ttl).Err()
	}
	if block.SetOnlyNew {
		return self.config.cli.SetNX(block.Context(), block.Key, b, ttl).Err()
	}
	return self.config.cli.Set(block.Context(), block.Key, b, ttl).Err()
}

func (self *RedisCache) getBytes(ctx context.Context, key string, skipLocalCache bool) ([]byte, error) {
	if !skipLocalCache && self.config.LocalCache != nil {
		var buf []byte
		err := self.config.LocalCache.Get(key, &buf)
		if err != nil {
			return nil, err
		}
		return buf, nil
	}

	if self.config.cli == nil {
		if self.config.LocalCache == nil {
			return nil, errRedisLocalCacheNil
		}
		return nil, cacher.ErrCacheMiss
	}

	b, err := self.config.cli.Get(ctx, key).Bytes()
	if err != nil {
		if self.config.StatsEnabled {
			atomic.AddUint64(&self.config.misses, 1)
		}
		if err == redis.Nil {
			return nil, cacher.ErrCacheMiss
		}
		return nil, err
	}

	if self.config.StatsEnabled {
		atomic.AddUint64(&self.config.hits, 1)
	}

	if !skipLocalCache && self.config.LocalCache != nil {
		self.config.LocalCache.Set(&cacher.CacheBlock{
			Key:   key,
			Value: b,
		})
	}
	return b, nil
}

func (self *RedisCache) Clear() error {
	err := self.config.LocalCache.Clear()
	return err
}

func (self *RedisCache) Close() error {
	err := self.Clear()
	return err
}

func (self *RedisCache) Delete(key string, ctx ...context.Context) error {
	if self.config.LocalCache != nil {
		self.config.LocalCache.Delete(key)
	}

	if self.config.cli == nil {
		if self.config.LocalCache == nil {
			return errRedisLocalCacheNil
		}
		return nil
	}

	var c context.Context
	if ctx != nil {
		c = ctx[0]
	} else {
		c = context.Background()
	}
	_, err := self.config.cli.Del(c, key).Result()
	return err
}

func (self *RedisCache) DeleteFromLocalCache(key string) {
	if self.config.LocalCache != nil {
		self.config.LocalCache.Delete(key)
	}
}

func (self *RedisCache) marshal(value interface{}) ([]byte, error) {
	switch value := value.(type) {
	case nil:
		return nil, nil
	case []byte:
		return value, nil
	case string:
		return []byte(value), nil
	}

	b, err := msgpack.Marshal(value)
	if err != nil {
		return nil, err
	}

	return compress(b), nil
}

func (self *RedisCache) unmarshal(b []byte, value interface{}) error {
	if len(b) == 0 {
		return nil
	}

	switch value := value.(type) {
	case nil:
		return nil
	case *[]byte:
		clone := make([]byte, len(b))
		copy(clone, b)
		*value = clone
		return nil
	case *string:
		*value = string(b)
		return nil
	}

	switch c := b[len(b)-1]; c {
	case noCompression:
		b = b[:len(b)-1]
	case s2Compression:
		b = b[:len(b)-1]

		var err error
		b, err = s2.Decode(nil, b)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown compression method: %x", c)
	}

	return msgpack.Unmarshal(b, value)
}

func compress(data []byte) []byte {
	if len(data) < compressionThreshold {
		n := len(data) + 1
		b := make([]byte, n, n+timeLen)
		copy(b, data)
		b[len(b)-1] = noCompression
		return b
	}

	n := s2.MaxEncodedLen(len(data)) + 1
	b := make([]byte, n, n+timeLen)
	b = s2.Encode(b, data)
	b = append(b, s2Compression)
	return b
}
