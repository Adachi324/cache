package cache

import (
	"context"
	"time"
)

// innerCache interface wraps internal cache client (e.g. redis.Pool of redigo, memcache.Client of gomemcache)
// nolint:predeclared
type innerCache interface {
	// get retrieves an item from the cache
	get(ctx context.Context, key string) (interface{}, error)

	// getMany retrieves multiple items from the cache.
	// The returned slice has same length with keys, for key not cached, its value will be nil
	getMany(ctx context.Context, keys ...string) ([]interface{}, error)

	// set sets an item to the cache, replacing any existing item.
	set(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...innerOperationOption) error

	// setMany sets multiple items to the cache, replacing any existing items.
	// opts determines if will apply `noReply`, which is only supported by memcached.
	setMany(ctx context.Context, valueMap map[string]interface{}, expire time.Duration, opts ...innerOperationOption) error

	// add adds an item to the cache only if an item doesn't exist for the given
	// key, or if the existing item is expired. Returns error otherwise.
	add(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...innerOperationOption) error

	// replace sets a new value for the cache key only if it already exists. Returns an
	// error if it does not.
	replace(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...innerOperationOption) error

	// delete removes an item from the cache. May return ErrCacheMiss (depends on concrete implementation)
	delete(ctx context.Context, key string) error

	// deleteMany deletes multiple items from the cache. May return ErrCacheMiss (depends on concrete implementation).
	// opts determines if will apply `noReply`, which is only supported by memcached.
	deleteMany(ctx context.Context, keys []string, opts ...innerOperationOption) error

	// increment increments a real number, and returns error if the value is not real
	// `initNonExistKey` in opts determines if will create value with 0 before perform operation if key not exists
	// `initNonExistKey` by default will be applied
	increment(ctx context.Context, key string, delta uint64, opts ...innerOperationOption) (int64, error)

	// decrement decrements a real number, and returns error if the value is not real
	// `initNonExistKey` in opts determines if will create value with 0 before perform operation if key not exists
	decrement(ctx context.Context, key string, delta uint64, opts ...innerOperationOption) (int64, error)

	// expire updates the cache expire time
	expire(ctx context.Context, key string, expire time.Duration, opts ...innerOperationOption) error

	// flush deletes all items from the cache.
	flush(ctx context.Context) error

	// ping checks the accessibility to the cache.
	ping(ctx context.Context) error

	// rawClient will return raw client of current library (e.g. redis.Pool of redigo, memcache.Client of gomemcache)
	// inMemoryCache not support it, will return nil instead
	rawClient() interface{}

	// close releases open resources
	close() error
}

// innerCacheOperationOptions defines operation options are applicable for innerCache
type innerCacheOperationOptions struct {
	noReply         bool                     // default to true
	initNonExistKey bool                     // default to true
	waitRistretto   bool                     // default to false, applies to ristretto in-memory cache only. set to true means will waitRistretto for all item to pass through buffer
	expirationMap   map[string]time.Duration // used to specify a specific key's expiration, have higher priority than `expire` in setMany
}

func newInnerCacheOperationOptions() *innerCacheOperationOptions {
	return &innerCacheOperationOptions{
		noReply:         true,
		initNonExistKey: true,
		waitRistretto:   false,
	}
}

type innerOperationOption func(*innerCacheOperationOptions)

func withNoReply(noReply bool) innerOperationOption {
	return func(opts *innerCacheOperationOptions) {
		opts.noReply = noReply
	}
}

func withInitNonExistKey(initNonExistKey bool) innerOperationOption {
	return func(opts *innerCacheOperationOptions) {
		opts.initNonExistKey = initNonExistKey
	}
}

func withWaitRistretto(waitRistretto bool) innerOperationOption {
	return func(opts *innerCacheOperationOptions) {
		opts.waitRistretto = waitRistretto
	}
}

func withExpirationMap(expirationMap map[string]time.Duration) innerOperationOption {
	return func(opts *innerCacheOperationOptions) {
		opts.expirationMap = expirationMap
	}
}
