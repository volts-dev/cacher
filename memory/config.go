package memory

import (
	"fmt"
	"time"

	"github.com/volts-dev/cacher"
)

type (
	Option func(*Config)

	Config struct {
		cacher.Config
		Active    bool
		SecretKey []byte
		Interval  time.Duration
		Expire    time.Duration
		prefix    string
		Size      int
		GC        bool
	}
)

func (self *Config) Init(opts ...cacher.Option) {
	self.Config.Init(self, opts...)
}

func WithSize(size int) cacher.Option {
	return func(cfg *cacher.Config) {
		cfg.SetByField("size", size)
	}
}

func WithInterval(ticker int) cacher.Option {
	return func(cfg *cacher.Config) {
		v, err := time.ParseDuration(fmt.Sprintf("%ds", ticker))
		if err != nil {
			return
		}
		cfg.SetByField("interval", v)
	}
}

func WithExpire(ticker int) cacher.Option {
	return func(cfg *cacher.Config) {
		v, err := time.ParseDuration(fmt.Sprintf("%ds", ticker))
		if err != nil {
			return
		}
		cfg.SetByField("expire", v)
	}
}
