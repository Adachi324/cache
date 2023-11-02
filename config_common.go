package cache

import (
	"fmt"
	"go-eCache/internal/compression"
)

const (
	// defaultDNSResolverPoolSize will apply when the dns resolution config's DNSResolverPoolSize is 0.
	defaultDNSResolverPoolSize = 20

	// defaultElapseThresholdForSlowLogMillis will apply when the observation config's ElapseThresholdMillis is 0
	defaultElapseThresholdForSlowLogMillis = 100
)

const (
	// NoPoolPattern means that there will not be a dns resolver pool, and it will use the shared dns results by default
	NoPoolPattern = -1
)

// StampedeMitigationStrategy determines the strategy to mitigate cache stampede
type StampedeMitigationStrategy int

const (
	// NoProtection means there is no protection or synchronization, every goroutine will load data without checking if others are loading same data
	NoProtection StampedeMitigationStrategy = 0
	// InProcessSignal means only one goroutine load data, and then broadcast to other waiting goroutines through in-process signal
	InProcessSignal StampedeMitigationStrategy = 1
	// AcrossInstanceSignal means that only one goroutine in one instance loads data and then broadcasts to other waiting instances.
	//
	// It supports memcached, redis, migration, and multi-layer.
	// It does not support in-memory, or the migration with primary layer as in-memory, or the multi-layer with the outermost layer as in-memory.
	// Besides, it may not work as expected for cache clusters (e.g., Memcached HA).
	//
	// This strategy applies to cold storage as DataLoader. It will reduce the QPS in DataLoader, but may increase the QPS in cache.
	AcrossInstanceSignal StampedeMitigationStrategy = 2
)

// EncodingConfig is the config to control built in bytes protocol
type EncodingConfig struct {
	// DisableEncoding when set to true, will disable compression and bytes protocol
	// and will send the raw bytes to cache server. Default value is false.
	DisableEncoding bool `yaml:"disable_encoding" json:"disable_encoding"`
	// CompressionConfig defines compression behavior
	CompressionConfig compression.Config `yaml:"compression_config" json:"compression_config"`
}

// Validate check if config is valid
func (c EncodingConfig) Validate() error {
	if err := c.CompressionConfig.Validate(); err != nil {
		return err
	}
	return nil
}

// DisableConfig defines whether current cache is disabled
type DisableConfig struct {
	// Disable is used to disable current cache
	Disable bool `yaml:"disable" json:"disable"`
}

// ManufacturerConfig defines how to produce data when multiple goroutines are dealing with same keys.
type ManufacturerConfig struct {
	// CacheStampedeMitigation strategy can be `NoProtection`, `InProcessSignal` or `AcrossInstanceSignal`.
	// Default value is `NoProtection`.
	CacheStampedeMitigation StampedeMitigationStrategy `yaml:"cache_stampede_mitigation" json:"cache_stampede_mitigation"`

	// AcrossInstanceSignalConfig is the extra config for the strategy `AcrossInstanceSignal` of the `CacheStampedeMitigation`
	AcrossInstanceSignalConfig AcrossInstanceSignalConfig `yaml:"across_instance_signal_config" json:"across_instance_signal_config"`
}

type AcrossInstanceSignalConfig struct {
	// RetryIntervalMillis is the interval time retrying to get value from the cache that is (being) loaded by the other instance
	// when stampede mitigation strategy is `AcrossInstanceSignal`.
	// Default as 100 ms.
	RetryIntervalMillis int64 `yaml:"retry_interval_millis" json:"retry_interval_millis"`

	// DlockUnitExpirationMillis is the unit time for dlock expiration, and dlock will be extended regularly while loading data
	// when stampede mitigation strategy is `AcrossInstanceSignal`. Dlock is applied here to ensure that only one instance loads data.
	//
	// If service suddenly crashes/restarts, the dlock may remain in the cache for the duration of DlockUnitExpirationMillis.
	// However, Users must be cautious to lower the value, this may lead to frequent QPS in cache and perhaps pre-mature dlock release.
	// Default as 5000 ms.
	DlockUnitExpirationMillis int64 `yaml:"dlock_unit_expiration_millis" json:"dlock_unit_expiration_millis"`
}

// Validate checks if config is valid
func (c *ManufacturerConfig) Validate(cacheType Type) error {
	if c != nil {
		if c.CacheStampedeMitigation != NoProtection &&
			c.CacheStampedeMitigation != InProcessSignal &&
			c.CacheStampedeMitigation != AcrossInstanceSignal {
			return cacheErr(fmt.Sprintf("manufacturer_config_strategy_invalid: %v", c.CacheStampedeMitigation))
		}
		return c.validateAcrossInstanceSignalConfig(cacheType)
	}
	return nil
}

func (c *ManufacturerConfig) validateAcrossInstanceSignalConfig(cacheType Type) error {
	if c.CacheStampedeMitigation != AcrossInstanceSignal {
		if c.AcrossInstanceSignalConfig.RetryIntervalMillis != 0 || c.AcrossInstanceSignalConfig.DlockUnitExpirationMillis != 0 {
			return cacheErr(fmt.Sprintf("across_instance_signal_config_invalid_for_the_other_strategy: %v", c.CacheStampedeMitigation))
		}
		return nil
	}
	if cacheType == InMemory {
		return cacheErr("across_instance_strategy_does_not_support_inmemory_or_multilayer_with_outermost_inmemory")
	}
	if c.AcrossInstanceSignalConfig.RetryIntervalMillis < 0 {
		return cacheErr(fmt.Sprintf("manufacturer_config_retry_interval_millis_invalid: %v", c.AcrossInstanceSignalConfig.RetryIntervalMillis))
	}
	if c.AcrossInstanceSignalConfig.DlockUnitExpirationMillis < 0 {
		return cacheErr(fmt.Sprintf("manufacturer_config_dlock_unit_expiration_millis_invalid: %v", c.AcrossInstanceSignalConfig.DlockUnitExpirationMillis))
	}
	return nil
}
