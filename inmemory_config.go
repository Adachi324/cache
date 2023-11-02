package cache

import (
	"fmt"
	"go-eCache/codec"
	"go-eCache/internal/client/inmemory"
	"go-eCache/size"
	"time"
)

const (
	// defaultInMemoryExpiration defines default expiration for in memory cache if not set by client explicitly
	defaultInMemoryExpiration = time.Duration(86400) * time.Second
)

// InMemoryCacheType is used in InMemoryCacheConfig to indicate in-memory cache inner implementation
type InMemoryCacheType int

const (
	// Ristretto represents inner implementation is Ristretto
	Ristretto InMemoryCacheType = 1
)

// RistrettoCacheConfig defines config used to build InMemoryCache backed by Ristretto cache
type RistrettoCacheConfig inmemory.RistrettoCacheConfig

// Validate checks if the RistrettoCacheConfig is valid.
func (c *RistrettoCacheConfig) Validate() error {
	if c.Capacity < 0 {
		return cacheErr(fmt.Sprintf("invalid_config_capacity: %v", c.Capacity))
	}
	if c.NumCounters < 0 {
		return cacheErr(fmt.Sprintf("invalid_config_num_counters: %v", c.NumCounters))
	}
	return nil
}

const (
	defaultCapacity = 1000000

	// use 10,000,000 as default value, which is not too large for memory footprint (5MB), not too little as 10M can accommodate for 1M cache items.
	defaultNumCounters = 10000000
)

func (c *RistrettoCacheConfig) setDefaultValue() {
	if c.Capacity == 0 {
		c.Capacity = defaultCapacity
	}
	if c.NumCounters == 0 {
		c.NumCounters = defaultNumCounters
	}
	if c.CostFunc == nil {
		c.CostFunc = size.CostOne
	}
}

// InMemoryCacheConfig defines config used to construct a inMemoryCache
type InMemoryCacheConfig struct {
	// DefaultExpirationSecs defines default cache data expiration in seconds. If zero, default value 86400 is used.
	DefaultExpirationSecs int `yaml:"default_expiration_secs" json:"default_expiration_secs"`

	// MaxExpirationSecs defines the max expiration of a key in cache
	// Default value is 0, same as not set, means no max expiration
	// It has higher priority than DefaultExpirationSecs, but lower priority than NoExpiration.
	MaxExpirationSecs int `yaml:"max_expiration_secs" json:"max_expiration_secs"`

	// CacheType defines the supported cache types e.g. SyncMap, Ristretto
	// Default value is SyncMap
	CacheType InMemoryCacheType `yaml:"cache_type" json:"cache_type"`

	// CodecConfig is the config to control default codec type
	CodecConfig codec.Config `yaml:"codec_config" json:"codec_config"`

	// ManufacturerConfig defines the behavior for `Load` and `LoadMany`
	ManufacturerConfig ManufacturerConfig `yaml:"manufacturer_config" json:"manufacturer_config"`

	// RistrettoCacheConfig defines config for ristretto inmemory cache
	// Only take effect when CacheType is Ristretto
	RistrettoCacheConfig RistrettoCacheConfig `yaml:"ristretto_cache_config" json:"ristretto_cache_config"`
}

// Validate checks if config is valid
func (c InMemoryCacheConfig) Validate() error {
	if c.DefaultExpirationSecs < 0 {
		return cacheErr(fmt.Sprintf("invalid_config_default_expiration_secs: %v", c.DefaultExpirationSecs))
	}
	if c.MaxExpirationSecs < 0 {
		return cacheErr(fmt.Sprintf("invalid_config_max_expiration_secs: %v", c.MaxExpirationSecs))
	}
	if c.CacheType != Ristretto {
		return errInMemoryConfigCacheTypeNotSupported
	}
	if err := c.RistrettoCacheConfig.Validate(); err != nil {
		return err
	}
	return nil
}

// Config wraps this InMemoryCacheConfig in a generic Config struct.
func (c InMemoryCacheConfig) Config() Config {
	return Config{
		Type:     InMemory,
		InMemory: c,
	}
}

func (c InMemoryCacheConfig) defaultExpiration() time.Duration {
	configDefaultExpire := time.Duration(c.DefaultExpirationSecs) * time.Second
	if configDefaultExpire == DefaultExpiration {
		return defaultInMemoryExpiration
	}
	return configDefaultExpire
}
