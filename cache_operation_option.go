package cache

import (
	"sync"
	"time"

	"go-eCache/codec"
)

// NonExistKeyStrategy defines strategy available when key not exist in cache for GetMany()
type NonExistKeyStrategy int

const (
	// FillNil is default type of none exist key strategy, which will fill `nil` for keys not exist in cache in receiver map
	FillNil NonExistKeyStrategy = 0
	// RemoveKey will remove the key in receiver map if it does not exist in cache
	RemoveKey NonExistKeyStrategy = 1
)

type cacheOperationOptions struct {
	noReply          bool       // default to True
	initNonExistKey  bool       // default to True
	skipEncodeDecode bool       // default to False
	skipCodec        bool       // default to False, applicable to in-memory cache only
	waitRistretto    bool       // default to False, applicable to in-memory ristretto cache only
	codecType        codec.Type // default to JSON
	customCodec      codec.CustomCodec

	nonExistKeyStrategy      NonExistKeyStrategy      // default to FillNil
	onErrExpiration          time.Duration            // default to loadDefaultOnErrExpiration 3s，applicable for Load/LoadMany
	hardExpiration           time.Duration            // expiration duration for hard timeout
	softExpiration           time.Duration            // expiration duration for soft timeout
	hardTimeoutTs            int64                    // expiration timestamp for hard timeout
	softTimeoutTs            int64                    // expiration timestamp for soft timeout
	expirationMap            map[string]time.Duration // used to specify a key's hard expiration, with the highest priority in Set/Load(Many)
	hardExpirationMultiLayer []time.Duration          // used to specify a layer's hard expiration, with the medium priority between `expireMap` (high) and `expire` variable (low), applicable to Set/Load(Many) in MultiLayerCache only
	softExpirationMultiLayer []time.Duration          // used to specify a layer's soft expiration, with the higher priority than `softExpiration`, applicable to Set/Load(Many) in MultiLayerCache only
}

var operationOptionsPool = &sync.Pool{
	New: func() interface{} {
		return &cacheOperationOptions{
			skipCodec:                false,
			waitRistretto:            false,
			nonExistKeyStrategy:      FillNil,
			noReply:                  true,
			initNonExistKey:          true,
			onErrExpiration:          loadDefaultOnErrExpiration,
			expirationMap:            nil,
			softExpirationMultiLayer: nil,
			hardExpirationMultiLayer: nil,
		}
	},
}

func (p *cacheOperationOptions) reset() {
	p.noReply = true
	p.initNonExistKey = true
	p.skipEncodeDecode = false
	p.skipCodec = false
	p.waitRistretto = false
	p.codecType = codec.Jsoniter
	p.nonExistKeyStrategy = FillNil
	p.hardExpiration = 0
	p.softExpiration = 0
	p.onErrExpiration = loadDefaultOnErrExpiration
	p.softTimeoutTs = 0
	p.hardTimeoutTs = 0
	p.expirationMap = nil
	p.softExpirationMultiLayer = nil
	p.hardExpirationMultiLayer = nil
}

func newCacheOperationOptions() *cacheOperationOptions {
	option, _ := operationOptionsPool.Get().(*cacheOperationOptions)
	option.reset()
	return option
}

func recycleCacheOperationOptions(option interface{}) {
	operationOptionsPool.Put(option)
}

// WithSkipCodec skips marshal & unmarshal.
// Recommended to use it when data fetched from cache will not be modified, to gain higher performance.
// Applicable for InMemoryCache's Get/GetMany/Set/SetMany/Load/LoadMany only.
func WithSkipCodec() OperationOption {
	return func(option *cacheOperationOptions) {
		option.skipCodec = true
	}
}

// WithNonExistKeyStrategy sets nonExistKeyStrategy for GetMany.
// If not set, will fallback to default `FillNil` strategy (another strategy supported is `RemoveKey`)
func WithNonExistKeyStrategy(strategy NonExistKeyStrategy) OperationOption {
	return func(option *cacheOperationOptions) {
		option.nonExistKeyStrategy = strategy
	}
}

// WithNoReply sets `NoReply` flag for SetMany/DeleteMany.
// Only take effect for MemcachedCache.
// By default, `NoReply` flag is true to improve performance.
func WithNoReply(noReply bool) OperationOption {
	return func(option *cacheOperationOptions) {
		option.noReply = noReply
	}
}

// WithSoftExpiration sets soft expiration for to-cache data.
// It only works if value is smaller than hard expiration.
// The soft expiration will take effect for Set/Load(Many)
// For Load(Many), ff meets soft expiration, will return the data, and start a goroutine to update it asynchronously.
func WithSoftExpiration(softExpiration time.Duration) OperationOption {
	return func(option *cacheOperationOptions) {
		option.softExpiration = softExpiration
	}
}

// WithSoftExpirationMultiLayer sets different soft expiration for different levels,
// and has the HIGHER priority for SOFT timeout than `WithSoftExpiration`.
// Applicable for Set/Load(Many) in MultiLayerCache.
// The length of time duration slice should be the same as the layer number of the multiple cache.
func WithSoftExpirationMultiLayer(softExpirationMultiLayer []time.Duration) OperationOption {
	return func(option *cacheOperationOptions) {
		option.softExpirationMultiLayer = softExpirationMultiLayer
	}
}

// WithOnErrExpiration sets on-error expiration for current cache data.
// If data loader return an error with full length result, the result will be cached with on-error expiration.
// The on-error expiration will take effect for Load(Many).
func WithOnErrExpiration(onErrExpiration time.Duration) OperationOption {
	return func(option *cacheOperationOptions) {
		option.onErrExpiration = onErrExpiration
	}
}

// WithExpirationMap sets different expiration for different keys,
// and has the HIGHEST priority for HARD timeout.
// Applicable for Set/Load(Many).
func WithExpirationMap(expirationMap map[string]time.Duration) OperationOption {
	return func(option *cacheOperationOptions) {
		option.expirationMap = expirationMap
	}
}

// WithHardExpirationMultiLayer sets different hard expiration for different layers,
// and has the MEDIUM priority for HARD timeout between `WithExpirationMap` (HIGH) and `expire` variable (LOW).
// Applicable for Set/Load(Many) in MultiLayerCache only.
//
// If a layer's expiration is set to DefaultExpiration (time.Duration(0)), it will use the `DefaultExpirationSecs` as pre-configured for this layer.
// The length of time duration slice should be the same as the layer number of the multiple cache.
func WithHardExpirationMultiLayer(hardExpirationMultiLayer []time.Duration) OperationOption {
	return func(option *cacheOperationOptions) {
		option.hardExpirationMultiLayer = hardExpirationMultiLayer
	}
}

// WithInitNonExistKey determines when Increment/Decrement a non-existing key, whether to implicitly create one or return ErrCacheMiss
func WithInitNonExistKey(initNonExistKey bool) OperationOption {
	return func(option *cacheOperationOptions) {
		option.initNonExistKey = initNonExistKey
	}
}

// WithSkipEncoding skips encoding before sending the data to cache and skips decoding after getting data from cache,
// so that only raw data is stored to the cache without other metadataHeader (which be used to support unified cache advance features).
// Note: Unified Cache's Encoded cache data may not be compatible with other cache libraries.
//
// WithSkipEncoding can be applied when multiple applications are reading/writing same cache data on same cache server,
// but not all applications are using this unified cache library.
//
// Note: WithSkipEncoding can NOT be used for Load() operation, Migration Cache and Multi-layer Cache.
// Applicable for Get(Many)/Set(Many)/Add/Replace (excluding multi-layer cache).
func WithSkipEncoding() OperationOption {
	return func(option *cacheOperationOptions) {
		option.skipEncodeDecode = true
	}
}

// WithWaitRistretto waits for all items in the set buffer to be processed.
// Applies to Set/SetMany/Load/LoadMany operation to in-memory ristretto cache only.
func WithWaitRistretto() OperationOption {
	return func(option *cacheOperationOptions) {
		option.waitRistretto = true
	}
}
