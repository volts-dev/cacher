package cacher

import (
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
		opt(self)
	}

	self.AsStruct(config) // mapping to config
}
