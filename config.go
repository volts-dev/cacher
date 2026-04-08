package cacher

import (
	"log"

	"github.com/volts-dev/dataset"
)

type (
	Option func(*Config)

	Config struct {
		dataset.TRecordSet
	}
)

// config: the config struct with binding the options
func (self *Config) Init(config interface{}, opts ...Option) {
	if self.TRecordSet.IsEmpty() {
		// 初始化RecordSet
		self.TRecordSet.Reset()
	}

	for _, opt := range opts {
		safeApplyOption(opt, self)
	}

	self.AsStruct(config) // mapping to config
}

// safeApplyOption applies an Option and recovers from panics caused by
// invalid field names in SetByField reflection calls, logging instead of crashing.
func safeApplyOption(opt Option, cfg *Config) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("cacher: option apply failed: %v", r)
		}
	}()
	opt(cfg)
}
