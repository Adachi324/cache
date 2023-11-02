package inmemory

import (
	"time"

	"github.com/dgraph-io/ristretto"
)

// RistrettoCacheConfig defines config used to build InMemoryCache backed by Ristretto cache
type RistrettoCacheConfig struct {
	// Capacity is the maximum of total cache_item's cost. It's unit can be anything depend on how you define the CostFunc.
	// If your cache_item's cost is 1 for all cache_item, then  the Capacity is the number of keys in the cache.
	// If your cache_item's cost is the memory size of the cache_item, then the unit of Capacity should be bytes.
	// Default to 1,000,000 (and the costFunc is CostOne, which mean the default is the total of key)
	Capacity int64 `yaml:"capacity" json:"capacity"`

	// NumCounters determines the number of counters (keys) to keep that hold
	// access frequency information. It's generally a good idea to have more
	// counters than the max cache capacity, as this will improve eviction
	// accuracy and subsequent hit ratios.
	// If expect the cache to hold around x items when full, set NumCounters to (x * 10)
	// Default value is 10,000,000
	NumCounters int64 `yaml:"num_counters" json:"num_counters"`

	// UseInternalCost set to true indicates to the cache that the cost of
	// internally storing the value should be included in item cost. Setting
	// it to true will reduce memory usage.
	// Default value is false
	UseInternalCost bool `yaml:"use_internal_cost" json:"use_internal_cost"`

	// CostFunc for calculating the cost of cache item.
	// Default CostFunc always return 1 (the Capacity of Ristretto is the total of key).
	// User can pass function `size.CostMemoryUsage()` so the Capacity is the total memory usage,  this case
	// user should enable UseInternalCost for accurate results.
	CostFunc func(val interface{}) int64 `yaml:"-" json:"-"`
}

// RistrettoCache is a in-memory cache, internally it uses Ristretto
type RistrettoCache struct {
	inner           *ristretto.Cache
	NumCounters     int64                       // used for updateConfig only
	UseInternalCost bool                        // used for updateConfig only
	CostFunc        func(val interface{}) int64 // used for updateConfig only
}

// Get retrieves an item from the cache
// Returns true if the cache key exists, returns false otherwise
func (c *RistrettoCache) Get(key string) (interface{}, bool) {
	return c.inner.Get(key)
}

// GetMany retrieves multiple items from the cache.
// If a key does not exist, a `nil` will be returned.
func (c *RistrettoCache) GetMany(keys ...string) []interface{} {
	res := make([]interface{}, len(keys))
	for idx := range keys {
		if val, found := c.inner.Get(keys[idx]); found {
			res[idx] = val
		} else {
			res[idx] = nil
		}
	}

	return res
}

func (c *RistrettoCache) setInner(key string, value interface{}, expire time.Duration) {
	cost := int64(0) // because we use the CostFunc, and ristretto only call CostFunc if cost of item is 0.

	// 0 make ristretto cache no expire, but -1 (NoExpiration) will do no-op
	if expire <= 0 {
		expire = 0
	}
	_ = c.inner.SetWithTTL(key, value, cost, expire)
}

// Set sets an item to the cache, replacing any existing item.
func (c *RistrettoCache) Set(key string, value interface{}, expire time.Duration) {
	c.setInner(key, value, expire)
}

// SetMany sets multiple items to the cache, replacing any existing items.
func (c *RistrettoCache) SetMany(valueMap map[string]interface{}, expire time.Duration, expirationMap map[string]time.Duration) {
	for key, value := range valueMap {
		var expiration time.Duration
		if exp, ok := expirationMap[key]; ok {
			expiration = exp
		} else {
			expiration = expire
		}
		c.setInner(key, value, expiration)
	}
}

// Delete removes an item from the cache
// Returns true if the cache key exists, returns false otherwise
// nolint:predeclared
func (c *RistrettoCache) Delete(key string) bool {
	_, found := c.inner.Get(key)
	if !found {
		return false
	}

	c.inner.Del(key)

	return true
}

// DeleteMany deletes multiple items from the cache.
func (c *RistrettoCache) DeleteMany(keys ...string) {
	for idx := range keys {
		c.inner.Del(keys[idx])
	}
}

// Flush deletes all items from the cache.
func (c *RistrettoCache) Flush() {
	c.inner.Clear()
}

// UpdateMaxCost updates the maxCost of an existing cache.
func (c *RistrettoCache) UpdateMaxCost(maxCost int64) {
	c.inner.UpdateMaxCost(maxCost)
}

// Wait blocks until all items in the buffer have been processed.
func (c *RistrettoCache) Wait() {
	c.inner.Wait()
}

// Close closes all goroutines and channels
func (c *RistrettoCache) Close() {
	c.inner.Close()
}

// NewRistrettoCache creates a new RistrettoCache
func NewRistrettoCache(ristrettoConfig RistrettoCacheConfig) (*RistrettoCache, error) {
	inner, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: ristrettoConfig.NumCounters,
		MaxCost:     ristrettoConfig.Capacity,
		// ristretto doc: Unless you have a rare use case, using `64` as the BufferItems value results in good performance.
		BufferItems: 64,
		// ristretto release log v0.1.0: By default, include the cost of the storeItem in the cost calculation.
		// before this version, we always used a constant cost of 1. For backward compatibility, we use UseInternalCost
		// in our config, which will only be true when explicitly set.
		IgnoreInternalCost: !ristrettoConfig.UseInternalCost,
		Cost:               ristrettoConfig.CostFunc,
	})
	if err != nil {
		return nil, err
	}

	return &RistrettoCache{
		inner:           inner,
		NumCounters:     ristrettoConfig.NumCounters,
		UseInternalCost: ristrettoConfig.UseInternalCost,
		CostFunc:        ristrettoConfig.CostFunc,
	}, nil
}
