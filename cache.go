package cache

import (
	"context"
	"time"
)

const (
	// NoExpiration will make cached key never expire
	NoExpiration time.Duration = -1

	// DefaultExpiration will use the default expiration value set at cache level
	DefaultExpiration time.Duration = 0
)

// DataLoader will return data from downstream service.
// When return result is nil, will not set it to cache stores.
//
// Downstream service MUST set TIMEOUT machanism for DataLoader to prevent hanging requests.
type DataLoader func(ctx context.Context, keys []string) ([]interface{}, error)

// OperationOption defines cache operation level options
type OperationOption func(*cacheOperationOptions)

// Cache is the interface of a cache store
type Cache interface {
	// Load is similar like Get, but if the key doesn't exist, it will invoke loader to load the data and store to cache
	// If expire is DefaultExpiration, it will use the default expiration of the cache.
	Load(ctx context.Context, loader DataLoader, key string, receiver interface{}, expire time.Duration, opts ...OperationOption) error
}

type composableCacheInner interface {
	loadInner() *cacheWrapperInner
	loadWrapper() *cacheWrapper
}

// ComposableCache defines cache which can be used to composite multilayer or migration cache
type ComposableCache interface {
	composableCacheInner
}
