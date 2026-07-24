package redis

import (
	"context"

	"github.com/volts-dev/cacher"
)

const (
	compressionThreshold = 64
	timeLen              = 4
)

const (
	noCompression = 0x0
	s2Compression = 0x1
)

type (
	MarshalFunc   func(interface{}) ([]byte, error)
	UnmarshalFunc func([]byte, interface{}) error

	Option func(*Config)

	Config struct {
		cacher.Config
		Active       bool
		SecretKey    []byte
		LocalCache   cacher.ICacher
		prefix       string
		cli          rediser
		context      context.Context
		// StatsEnabled must be set before first use; it is not safe to change concurrently.
		StatsEnabled bool
		hits         uint64
		misses       uint64
		Marshal      MarshalFunc
		Unmarshal    UnmarshalFunc
	}
)

func (self *Config) Init(opts ...cacher.Option) {
	self.Config.Init(self, opts...)

	// cli/LocalCache 是未导出字段：cacher.Config.Init 末尾的 AsStruct(self) 走的是
	// mapstructure 风格的 map→struct 解码（见 dataset.decode），只会写导出字段，
	// 未导出字段永远保持零值——WithRedis/WithLocalCacher 存进 TRecordSet 的值因此
	// 从未真正落到 self.cli/self.LocalCache 上，Set/Get 于是走进「cli 和 LocalCache
	// 都是 nil」的错误分支，redis 后端形同虚设（2026-07-24 debug session store 时
	// 真栈撞出：SetSession 报 "cache: both Redis and LocalCache are nil"）。
	// 在此直接从记录里取回并赋值——本方法就在 redis 包内，能直接碰未导出字段，
	// 不需要也不能指望通用的反射解码路径替我们做这件事。
	if v := self.Config.GetByField("cli"); v != nil {
		if c, ok := v.(rediser); ok {
			self.cli = c
		}
	}
	if v := self.Config.GetByField("local_cache"); v != nil {
		if c, ok := v.(cacher.ICacher); ok {
			self.LocalCache = c
		}
	}
}

func WithRedis(rds rediser) cacher.Option {
	return func(cfg *cacher.Config) {
		cfg.SetByField("cli", rds)
	}
}

func WithLocalCacher(chr cacher.ICacher) cacher.Option {
	return func(cfg *cacher.Config) {
		cfg.SetByField("local_cache", chr)
	}
}
