package cache

import (
	"context"
	"go-eCache/codec"
	"go-eCache/internal/utils"
	"go-eCache/size"
	"time"
)

var _ InternalUnifiedCacheCache = &internalUnifiedCacheClient{}
var internalUnifiedCacheClientImp InternalUnifiedCacheCache

// unifiedCache
var (
	inMemoryCache *InMemoryCache
)

type internalUnifiedCacheClient struct {
	CacheClient Cache
}

// GetUnifiedCache returns data cache store
func GetUnifiedCache() InternalUnifiedCacheCache {
	return internalUnifiedCacheClientImp
}

// InitUnifiedCache is a function to initial cache
func InitUnifiedCache(maxCapacity int64, maxNumCounters int64) {
	initInMemoryCache(maxCapacity, maxNumCounters)
	internalUnifiedCacheClientImp = &internalUnifiedCacheClient{
		CacheClient: inMemoryCache,
	}
}

func initInMemoryCache(maxCapacity int64, maxNumCounters int64) {
	if maxCapacity == 0 {
		maxCapacity = 268435456
	}
	if maxNumCounters == 0 {
		maxNumCounters = 100000
	}
	unifiedConfig := InMemoryCacheConfig{
		ManufacturerConfig: ManufacturerConfig{
			CacheStampedeMitigation: InProcessSignal,
		},
		CacheType: Ristretto,
		RistrettoCacheConfig: RistrettoCacheConfig{
			Capacity:        maxCapacity,    //bytes , max mem ,default 256M
			NumCounters:     maxNumCounters, //max keys,default 100000
			UseInternalCost: true,
			CostFunc:        size.CostMemoryUsage,
		},
		CodecConfig: codec.Config{
			Type: codec.Jsoniter,
		},
	}
	var err error
	inMemoryCache, err = NewInMemoryCache("in_memory_ristretto_cache", unifiedConfig)
	if err != nil {
		panic(err)
	}

}

// InternalUnifiedCacheCache is the interface of a cache store
type InternalUnifiedCacheCache interface {
	// LoadWithDefaultExpiration is similar LoadWithExpiration,but has default expiration value
	LoadWithDefaultExpiration(ctx context.Context, loader DataLoader, key string, receiver interface{}) error
	// LoadWithExpiration is similar like Get, but if the key doesn't exist, it will invoke loader to load the data and store to cache
	LoadWithExpiration(ctx context.Context, loader DataLoader, key string, receiver interface{}, softExpirationSecTime int64, hardExpirationSecTime int64) error
}

// LoadWithDefaultExpiration ....
func (icc *internalUnifiedCacheClient) LoadWithDefaultExpiration(ctx context.Context, loader DataLoader, key string, receiver interface{}) error {
	return icc.LoadWithExpiration(ctx, loader, key, receiver, 10, 60)
}

// LoadWithExpiration ....
func (icc *internalUnifiedCacheClient) LoadWithExpiration(ctx context.Context, loader DataLoader, key string, receiver interface{}, softExpirationSecTime int64, hardExpirationSecTime int64) error {
	hardExpiration := time.Duration(hardExpirationSecTime) * time.Second
	softExpirationOpt := WithSoftExpiration(time.Duration(softExpirationSecTime) * time.Second)
	if key == "" {
		value, err := loader(ctx, []string{""})
		if err == nil && len(value) > 0 && value[0] != nil {
			err := utils.SetValueByJSON(value[0], receiver)
			if err != nil {
				return err
			}
		}
		return nil
	}
	return icc.CacheClient.Load(ctx, loader, key, receiver, hardExpiration, softExpirationOpt)
}
