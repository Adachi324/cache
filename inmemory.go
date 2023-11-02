package cache

import (
	"context"
	"sync/atomic"
	"time"
)

// InMemoryCache implements Cache interface
type InMemoryCache struct {
	inner *cacheWrapper
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache(name string, config InMemoryCacheConfig) (*InMemoryCache, error) {
	sc, err := newCacheWrapper(name, config.Config())
	if err != nil {
		return nil, err
	}
	return &InMemoryCache{inner: sc}, nil
}

// Get (refer to Get of Cache interface)
func (c *InMemoryCache) Get(ctx context.Context, key string, receiver interface{}, opts ...OperationOption) error {
	return c.inner.get(ctx, key, receiver, opts...)
}

// GetMany (refer to GetMany of Cache interface)
func (c *InMemoryCache) GetMany(ctx context.Context, receiverMap map[string]interface{}, opts ...OperationOption) error {
	return c.inner.getMany(ctx, receiverMap, opts...)
}

// Set (refer to Set of Cache interface)
func (c *InMemoryCache) Set(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...OperationOption) error {
	return c.inner.set(ctx, key, value, expire, opts...)
}

// SetMany (refer to SetMany of Cache interface)
func (c *InMemoryCache) SetMany(ctx context.Context, valueMap map[string]interface{}, expire time.Duration, opts ...OperationOption) error {
	return c.inner.setMany(ctx, valueMap, expire, opts...)
}

// Delete (refer to Delete of Cache interface)
func (c *InMemoryCache) Delete(ctx context.Context, key string, opts ...OperationOption) error {
	return c.inner.delete(ctx, key, opts...)
}

// DeleteMany (refer to DeleteMany of Cache interface)
func (c *InMemoryCache) DeleteMany(ctx context.Context, keys []string, opts ...OperationOption) error {
	return c.inner.deleteMany(ctx, keys, opts...)
}

// Load (refer to Load of Cache interface)
func (c *InMemoryCache) Load(ctx context.Context, loader DataLoader, key string, receiver interface{}, expire time.Duration, opts ...OperationOption) error {
	return c.inner.load(ctx, loader, key, receiver, expire, opts...)
}

// LoadMany (refer to LoadMany of Cache interface)
func (c *InMemoryCache) LoadMany(ctx context.Context, loader DataLoader, receiverMap map[string]interface{}, expire time.Duration, opts ...OperationOption) error {
	return c.inner.loadMany(ctx, loader, receiverMap, expire, opts...)
}

// Flush (refer to Flush of Cache interface)
func (c *InMemoryCache) Flush(ctx context.Context) error {
	return c.inner.flush(ctx)
}

// Ping (refer to Ping of Cache interface)
func (c *InMemoryCache) Ping(ctx context.Context) error {
	return c.inner.ping(ctx)
}

// Close releases all open resources
func (c *InMemoryCache) Close(ctx context.Context) error {
	return c.inner.close()
}

// UpdateConfig updates current in-memory cache based on config
func (c *InMemoryCache) UpdateConfig(config InMemoryCacheConfig) error {
	return c.inner.updateConfig(Config{
		Type:     InMemory,
		InMemory: config,
	})
}

func (c *InMemoryCache) loadInner() *cacheWrapperInner {
	return (*cacheWrapperInner)(atomic.LoadPointer(&c.inner.inner))
}

func (c *InMemoryCache) loadWrapper() *cacheWrapper {
	return c.inner
}

func (c *InMemoryCache) getCacheType() cacheType {
	return inMemory
}
