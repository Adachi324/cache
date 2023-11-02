package cache

import (
	"context"
	"go-eCache/internal/client/inmemory"
	"reflect"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

// nolint:predeclared
type inMemoryCacheClient interface {
	Get(key string) (interface{}, bool)
	GetMany(keys ...string) []interface{}
	Set(key string, value interface{}, expire time.Duration)
	SetMany(valueMap map[string]interface{}, expire time.Duration, expirationMap map[string]time.Duration)
	Delete(key string) bool
	DeleteMany(keys ...string)
	Wait()
	Flush()
}

type closableInMemoryCacheClient interface {
	Close()
}

// inMemoryCacheInner is a wrapper of real in memory cache which impl innerCache Interface
type inMemoryCacheInner struct {
	cacheImpl unsafe.Pointer // of type *inMemoryCacheClient
}

func (c *inMemoryCacheInner) get(ctx context.Context, key string) (interface{}, error) {
	impl := c.loadInnerInMemoryCache()
	val, found := impl.Get(key)
	if !found {
		return nil, ErrCacheMiss
	}

	return val, nil
}

func (c *inMemoryCacheInner) getMany(ctx context.Context, keys ...string) ([]interface{}, error) {
	impl := c.loadInnerInMemoryCache()
	values := impl.GetMany(keys...)

	return values, nil
}

func (c *inMemoryCacheInner) set(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...innerOperationOption) error {
	options := newInnerCacheOperationOptions()
	for _, opt := range opts {
		opt(options)
	}

	impl := c.loadInnerInMemoryCache()

	if expire == NoExpiration {
		expire = 0
	}

	impl.Set(key, value, expire)

	if options.waitRistretto {
		impl.Wait()
	}

	return nil
}

func (c *inMemoryCacheInner) setMany(ctx context.Context, valueMap map[string]interface{}, expire time.Duration, opts ...innerOperationOption) error {
	options := newInnerCacheOperationOptions()
	for _, opt := range opts {
		opt(options)
	}

	impl := c.loadInnerInMemoryCache()
	impl.SetMany(valueMap, expire, options.expirationMap)

	if options.waitRistretto {
		impl.Wait()
	}

	return nil
}

func (c *inMemoryCacheInner) add(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...innerOperationOption) error {
	return nil
	//impl := c.loadInnerInMemoryCache()
	//
	//return impl.add(key, value, expire, opts...)
}

func (c *inMemoryCacheInner) replace(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...innerOperationOption) error {
	return nil
	//impl := c.loadInnerInMemoryCache()
	//
	//return impl.replace(key, value, expire, opts...)
}

// nolint:predeclared
func (c *inMemoryCacheInner) delete(ctx context.Context, key string) error {
	impl := c.loadInnerInMemoryCache()
	ok := impl.Delete(key)

	if !ok {
		return ErrCacheMiss
	}
	return nil
}

func (c *inMemoryCacheInner) deleteMany(ctx context.Context, keys []string, opts ...innerOperationOption) error {
	impl := c.loadInnerInMemoryCache()

	impl.DeleteMany(keys...)

	return nil
}

func (c *inMemoryCacheInner) increment(ctx context.Context, key string, delta uint64, opts ...innerOperationOption) (int64, error) {
	return 0, nil
	//impl := c.loadInnerInMemoryCache()
	//
	//return impl.increment(key, delta, opts...)
}

func (c *inMemoryCacheInner) decrement(ctx context.Context, key string, delta uint64, opts ...innerOperationOption) (int64, error) {
	return 0, nil
	//impl := c.loadInnerInMemoryCache()
	//
	//return impl.decrement(key, delta, opts...)
}

func (c *inMemoryCacheInner) expire(ctx context.Context, key string, expire time.Duration, opts ...innerOperationOption) error {
	return nil
	//impl := c.loadInnerInMemoryCache()
	//
	//return impl.expire(key, expire, opts...)
}

func (c *inMemoryCacheInner) ping(ctx context.Context) error {
	return nil
}

// nolint:predeclared
func (c *inMemoryCacheInner) close() error {
	curImpl := c.loadInnerInMemoryCache()

	if closableCache, ok := curImpl.(closableInMemoryCacheClient); ok {
		closableCache.Close()
	}
	return nil
}

func (c *inMemoryCacheInner) flush(ctx context.Context) error {
	impl := c.loadInnerInMemoryCache()

	impl.Flush()

	return nil
}

func (c *inMemoryCacheInner) rawClient() interface{} {
	return nil
}

func (c *inMemoryCacheInner) loadInnerInMemoryCache() inMemoryCacheClient {
	inner := *(*inMemoryCacheClient)(atomic.LoadPointer(&c.cacheImpl))

	return inner
}

func (c *inMemoryCacheInner) updateConfig(newConfig InMemoryCacheConfig) error {

	if needCreateNewRistrettoCache(c, newConfig.RistrettoCacheConfig) {
		return replaceCurImplWithNewRistrettoCache(c, newConfig)
	}
	return updateCurImplRistrettoMaxCost(c, newConfig.RistrettoCacheConfig.Capacity)

	return nil
}

func updateCurImplRistrettoMaxCost(c *inMemoryCacheInner, newCapacity int64) error {
	curImpl := c.loadInnerInMemoryCache()
	ristrettoImpl, ok := curImpl.(*inmemory.RistrettoCache)
	if !ok {
		return cacheErr("invalid ristretto implementation")
	}
	ristrettoImpl.UpdateMaxCost(newCapacity)
	return nil
}

func replaceCurImplWithNewRistrettoCache(c *inMemoryCacheInner, cfg InMemoryCacheConfig) error {
	curImpl := c.loadInnerInMemoryCache()

	if closableCache, ok := curImpl.(closableInMemoryCacheClient); ok {
		closableCache.Close()
	}

	var err error
	var newImpl inMemoryCacheClient

	ristrettoConfig := cfg.RistrettoCacheConfig
	ristrettoConfig.setDefaultValue()
	newImpl, err = inmemory.NewRistrettoCache(inmemory.RistrettoCacheConfig(ristrettoConfig))
	if err != nil {
		return cacheErr(err.Error())
	}

	atomic.StorePointer(&c.cacheImpl, unsafe.Pointer(&newImpl))

	return nil
}

func needCreateNewRistrettoCache(c *inMemoryCacheInner, ristrettoConfig RistrettoCacheConfig) bool {
	curImpl := c.loadInnerInMemoryCache()

	ristrettoImpl, ok := curImpl.(*inmemory.RistrettoCache)
	if !ok {
		return true
	}

	return ristrettoImpl.NumCounters != ristrettoConfig.NumCounters ||
		ristrettoImpl.UseInternalCost != ristrettoConfig.UseInternalCost ||
		reflect.ValueOf(ristrettoImpl.CostFunc).Pointer() != reflect.ValueOf(ristrettoConfig.CostFunc).Pointer()
}

func newInMemoryCache(config InMemoryCacheConfig) (c *inMemoryCacheInner, err error) {
	var impl inMemoryCacheClient
	if config.CacheType == Ristretto {
		ristrettoConfig := config.RistrettoCacheConfig
		ristrettoConfig.setDefaultValue()
		impl, err = inmemory.NewRistrettoCache(inmemory.RistrettoCacheConfig(ristrettoConfig))
		if err != nil {
			return nil, cacheErr(err.Error())
		}
	}

	inner := &inMemoryCacheInner{
		cacheImpl: unsafe.Pointer(&impl),
	}

	runtime.SetFinalizer(inner, stopInMemoryCache)

	return inner, nil
}

// stopInMemoryCache releases the resource hold by cacheImpl
func stopInMemoryCache(c *inMemoryCacheInner) {
	curImpl := c.loadInnerInMemoryCache()

	if closableCache, ok := curImpl.(closableInMemoryCacheClient); ok {
		closableCache.Close()
	}
}
