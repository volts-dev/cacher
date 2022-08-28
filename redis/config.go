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
		StatsEnabled bool
		hits         uint64
		misses       uint64
		Marshal      MarshalFunc
		Unmarshal    UnmarshalFunc
	}
)

func (self *Config) Init(opts ...cacher.Option) {
	self.Config.Init(self, opts...)
}

func WithRedis(rds rediser) cacher.Option {
	return func(cfg *cacher.Config) {
		cfg.FieldByName("cli").AsInterface(rds)
	}
}

func WithLocalCacher(chr cacher.ICacher) cacher.Option {
	return func(cfg *cacher.Config) {
		cfg.FieldByName("local_cache").AsInterface(chr)
	}
}
