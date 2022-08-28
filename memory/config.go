package memory

import (
	"container/list"
	"sync"

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
		Interval   int
		Expire     int
		prefix     string
		max        int // 最大上限缓存

	}
)

func (self *Config) Init(opts ...cacher.Option) {
	if self.GcList == nil {
		self.GcList = list.New()
	}

	self.Config.Init(self, opts...)
}

func WithInterval(ticker int) cacher.Option {
	return func(cfg *cacher.Config) {
		cfg.FieldByName("interval").AsInterface(ticker)
	}
}

func WithExpire(ticker int) cacher.Option {
	return func(cfg *cacher.Config) {
		cfg.FieldByName("expire").AsInterface(ticker)
	}
}
