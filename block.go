package cacher

import (
	"context"
	"log"
	"time"
)

type (
	// Memory cache item.
	CacheBlock struct {
		Key        string
		Value      interface{}
		Ctx        context.Context
		LastAccess time.Time // last update time
		// TTL is the cache expiration time.
		// Default TTL is 1 hour.
		TTL time.Duration
		// SetOnlyExist only sets the key if it already exists.
		SetOnlyExist bool

		// SetOnlyNew only sets the key if it does not already exist.
		SetOnlyNew bool

		// SkipLocalCache skips local cache as if it is not set.
		SkipLocalCache bool
	}
)

func (self *CacheBlock) Clone() *CacheBlock {
	return &CacheBlock{
		Key:            self.Key,
		Value:          self.Value,
		Ctx:            self.Ctx,
		LastAccess:     self.LastAccess,
		TTL:            self.TTL,
		SetOnlyExist:   self.SetOnlyExist,
		SetOnlyNew:     self.SetOnlyNew,
		SkipLocalCache: self.SkipLocalCache,
	}
}
func (self *CacheBlock) Context() context.Context {
	if self.Ctx == nil {
		return context.Background()
	}
	return self.Ctx
}

func (self *CacheBlock) Ttl() time.Duration {
	const defaultTTL = time.Hour

	if self.TTL < 0 {
		return 0
	}

	if self.TTL != 0 {
		if self.TTL < time.Second {
			log.Printf("too short TTL for key=%q: %s", self.Key, self.TTL)
			return defaultTTL
		}
		return self.TTL
	}

	return defaultTTL
}
