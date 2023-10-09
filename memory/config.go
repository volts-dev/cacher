package memory

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/volts-dev/cacher"
)

type (
	Option func(*Config)

	Config struct {
		cacher.Config
		Active     bool
		SecretKey  []byte
		GcList     *list.List // 	// 垃圾回收 store all of sessions for gc
		GcListLock sync.RWMutex
		Interval   time.Duration
		Expire     time.Duration
		prefix     string
		Size       int // 最大上限缓存

	}
)

func (self *Config) Init(opts ...cacher.Option) {
	if self.GcList == nil {
		self.GcList = list.New()
	}

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
		}

		cfg.SetByField("interval", v)
	}
}

func WithExpire(ticker int) cacher.Option {
	return func(cfg *cacher.Config) {
		v, err := time.ParseDuration(fmt.Sprintf("%ds", ticker))
		if err != nil {
		}
		cfg.SetByField("expire", v)
	}
}
