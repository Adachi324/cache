package cache

import (
	"context"
	"go-eCache/internal/xcontext"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"go-eCache/internal/utils"
)

// cacheType's value should map with cache.Type's
type cacheType int

const (
	redis cacheType = iota + 1
	inMemory
)

// cacheWrapperInner is a immutable struct
// do NOT change it after loaded from cacheWrapper, but create a new one if need any modification and set back
type cacheWrapperInner struct {
	name                string
	cacheType           cacheType
	cache               innerCache
	cacheHostName       string
	codecHandler        codecHandler
	encodingHandler     encodingHandler
	manufacturerHandler manufacturerHandler
	defaultExpiration   time.Duration
	maxExpiration       time.Duration
	isDisabled          bool
	isClosed            uint32
}

// cacheWrapper defines wrapper for different cache types (redis, memcached, and in-memory)
// it contains common logic like marshal/unmarshal, encoding/decoding, report metrics and tracing etc.
type cacheWrapper struct {
	inner unsafe.Pointer // of type *cacheWrapperInner

	updateMutex sync.Mutex // to ensure cacheWrapper update is atomic
}

// unsetExpiration is used when `setMany` purely relies on the option.expirationMap for each entry's expiration
const unsetExpiration = time.Duration(0)

func (s cacheType) String() string {
	switch s {
	case redis:
		return "redis"
	case inMemory:
		return "inmemory"
	}
	return "unknown"
}

func newCacheWrapper(name string, config Config) (*cacheWrapper, error) {

	inner, err := newCacheWrapperInner(name, config)
	if err != nil {
		return nil, err
	}

	return &cacheWrapper{
		inner: unsafe.Pointer(inner),
	}, nil
}

func newCacheWrapperInner(name string, config Config) (*cacheWrapperInner, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	var err error

	newInner := &cacheWrapperInner{
		name:     name,
		isClosed: 0, // initialize isClosed status
	}

	switch config.Type {
	case InMemory:
		inMemoryConfig := config.InMemory
		newInner.cache, err = newInMemoryCache(inMemoryConfig)
		if err != nil {
			return nil, err
		}
		fillCacheWrapperInnerFieldsWithInMemConfig(inMemoryConfig, newInner)
	default:
		return nil, errorConfigTypeNotSupported
	}

	return newInner, nil
}

func fillCacheWrapperInnerFieldsWithInMemConfig(config InMemoryCacheConfig, newInner *cacheWrapperInner) {
	newInner.cacheType = inMemory
	newInner.defaultExpiration = config.defaultExpiration()
	newInner.maxExpiration = time.Duration(config.MaxExpirationSecs) * time.Second
	newInner.codecHandler = newCodecHandler(config.CodecConfig)
	newInner.manufacturerHandler = newManufacturerHandler(config.ManufacturerConfig)
	var err error
	if err != nil {
		// will only return an error if provided hotkey config is invalid which should not be possible.
		panic(err)
	}
}

func (c *cacheWrapper) get(ctx context.Context, key string, receiver interface{}, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return ErrCacheMiss
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	fixedKey := inner.getFixedKey(ctx, key)

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdGet,
		TotalKeyCount:  1,
		RequestSize:    len(fixedKey),
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		stats.req = key

		var value interface{}
		value, err = inner.cache.get(ctx, fixedKey)
		if err != nil {
			return err
		}

		stats.ResponseSize = inner.getEncodedDataSize(value)
		stats.SuccessKeyCount = 1

		var data interface{}
		data, _, err = inner.decode(value, option.skipEncodeDecode)
		if err != nil {
			return err
		}

		err = inner.setCacheDataToReceiver(data, receiver, option)
		stats.resp = receiver

		return err
	})

	return err
}

func (c *cacheWrapper) getMany(ctx context.Context, receiverMap map[string]interface{}, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()
	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if len(receiverMap) == 0 {
		return nil
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	if c.isDisabled(ctx) {
		for k := range receiverMap {
			handleMissingKey(option.nonExistKeyStrategy, receiverMap, k)
		}
		return nil
	}

	keys := make([]string, 0, len(receiverMap))
	for key := range receiverMap {
		keys = append(keys, key)
	}
	fixedKeys, requestSize := inner.getFixedKeys(ctx, keys)

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdGetMany,
		TotalKeyCount:  len(fixedKeys),
		RequestSize:    requestSize,
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		err = c.getManyInner(ctx, keys, fixedKeys, receiverMap, stats, option)
		return err
	})

	return err
}

func (c *cacheWrapper) getManyInner(ctx context.Context, originalKeys []string, fixedKeys []string, receiverMap map[string]interface{}, stats *RequestStats, option *cacheOperationOptions) (err error) {
	stats.req = fixedKeys

	inner := c.loadCacheWrapperInner()

	var values []interface{}
	values, err = inner.cache.getMany(ctx, fixedKeys...)
	if err != nil {
		return
	}

	responseSize := 0
	successKeyCount := 0
	for _, value := range values {
		responseSize += inner.getEncodedDataSize(value)
		if value != nil {
			successKeyCount++
		}
	}

	stats.SuccessKeyCount = successKeyCount
	stats.ResponseSize = responseSize

	for idx := range values {
		originalKey := originalKeys[idx]
		if values[idx] == nil {
			handleMissingKey(option.nonExistKeyStrategy, receiverMap, originalKey)
		} else {
			var data interface{}
			data, _, err = inner.decode(values[idx], option.skipEncodeDecode)
			if err != nil {
				return
			}

			if err = inner.setCacheDataToReceiver(data, receiverMap[originalKey], option); err != nil {
				return
			}
		}
	}
	stats.resp = receiverMap

	return err
}

func handleMissingKey(nonExistKeyStrategy NonExistKeyStrategy, receiverMap map[string]interface{}, originalKey string) {
	if nonExistKeyStrategy == FillNil {
		receiverMap[originalKey] = nil
	} else {
		delete(receiverMap, originalKey)
	}
}

func (c *cacheWrapper) load(ctx context.Context, loader DataLoader, key string, receiver interface{}, expire time.Duration, opts ...OperationOption) error {
	receiverMap := map[string]interface{}{key: receiver}

	err := c.loadMany(ctx, loader, receiverMap, expire, opts...)
	if err != nil {
		return err
	}
	if receiverMap[key] == nil {
		return ErrCacheMiss
	}

	return nil
}

func (c *cacheWrapper) loadMany(ctx context.Context, loader DataLoader, receiverMap map[string]interface{}, expire time.Duration, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}
	if len(receiverMap) == 0 {
		return nil
	}

	keys := make([]string, 0, len(receiverMap))
	for key := range receiverMap {
		keys = append(keys, key)
	}
	missingKeys, toUpdateKeys, successKeyResultMap, getManyErr := getManyForLoad(ctx, inner, keys, receiverMap, inner.codecHandler, *option)
	if getManyErr != nil {
		return getManyErr
	}

	if len(successKeyResultMap) > 0 {
		setResultErr := setLoadResultsToReceiverMap(successKeyResultMap, receiverMap, inner.codecHandler, *option)
		//for key, val := range successKeyResultMap {
		//	fmt.Printf("successKey:%v,successVal:%v,sucessSoft:%v,sucessHard:%v\n", key, string(val.dataBytes), val.header.SoftTimeoutTs, val.header.HardTimeoutTs)
		//}
		if setResultErr != nil {
			return setResultErr
		}
	}

	if len(toUpdateKeys) > 0 {
		//for _, key := range toUpdateKeys {
		//	fmt.Printf("updateKey:%v\n", key)
		//}
		c.loadHandleToUpdateKeys(ctx, toUpdateKeys, receiverMap, loader, expire, *option)
	}

	if len(missingKeys) > 0 {
		//for _, key := range missingKeys {
		//	fmt.Printf("missingKey:%v\n", key)
		//}
		err = c.loadHandleMissingKeys(ctx, missingKeys, receiverMap, loader, expire, *option)
	}

	return err
}

func (c *cacheWrapper) loadHandleToUpdateKeys(ctx context.Context, toUpdateKeys []string, receiverMap map[string]interface{}, loader DataLoader, expire time.Duration, option cacheOperationOptions) {
	inner := c.loadCacheWrapperInner()

	// copy current reference of manufacturerHandler
	// so if users choose to update config for manufacturer policy,
	// old requests still can proceed with old group lock
	curManufacturerHandler := inner.manufacturerHandler

	// waitingInProcessSignalCallsMap will be loaded back to cache by another go-routine/instance, so we can ignore them in this go-routine.
	toHandleKeys, _ := curManufacturerHandler.add(ctx, toUpdateKeys)

	if len(toHandleKeys) > 0 {
		go func() {
			detachCtx := xcontext.Detach(ctx)
			if deadline, ok := ctx.Deadline(); ok {
				var cancel context.CancelFunc
				detachCtx, cancel = context.WithDeadline(detachCtx, deadline)
				defer func() {
					if cancel != nil {
						cancel()
					}
				}()
			}
			loadResultMap := loadHandleKeys(detachCtx, inner, toHandleKeys, receiverMap, loader, expire, curManufacturerHandler, inner.codecHandler, option)
			curManufacturerHandler.complete(ctx, genToCompleteResultMap(loadResultMap))
		}()
	}
}

func (c *cacheWrapper) loadHandleMissingKeys(ctx context.Context, missingKeys []string, receiverMap map[string]interface{}, loader DataLoader, expire time.Duration, option cacheOperationOptions) error {
	inner := c.loadCacheWrapperInner()

	// copy current reference of manufacturerHandler
	// so if users choose to update config for manufacturer policy,
	// old requests still can proceed with old group lock
	curManufacturerHandler := inner.manufacturerHandler

	toHandleKeys, waitingInProcessSignalCallsMap := curManufacturerHandler.add(ctx, missingKeys)

	if len(toHandleKeys) > 0 {
		loadResultMap := loadHandleKeys(ctx, inner, toHandleKeys, receiverMap, loader, expire, curManufacturerHandler, inner.codecHandler, option)
		curManufacturerHandler.complete(ctx, genToCompleteResultMap(loadResultMap))
		err := setLoadResultsToReceiverMap(loadResultMap, receiverMap, inner.codecHandler, option)
		if err != nil {
			return err
		}
	}
	for key, call := range waitingInProcessSignalCallsMap {
		val, _ := curManufacturerHandler.wait(call)
		result, ok := val.(loadResult)
		if !ok {
			continue
		}
		err := setLoadResultToReceiver(key, result, receiverMap, inner.codecHandler, option)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *cacheWrapperInner) encode(val interface{}, option *cacheOperationOptions) (interface{}, error) {
	if option.skipEncodeDecode {
		return val, nil
	}
	now := time.Now()
	var softTimeoutTs, hardTimeoutTs int64
	if option.softTimeoutTs > 0 {
		softTimeoutTs = option.softTimeoutTs
	} else if option.softExpiration > 0 {
		softTimeoutTs = now.Add(option.softExpiration).Unix()
	}

	if option.hardTimeoutTs > 0 {
		hardTimeoutTs = option.hardTimeoutTs
	} else if option.hardExpiration > 0 {
		hardTimeoutTs = now.Add(option.hardExpiration).Unix()
	} else if option.hardExpiration == NoExpiration {
		hardTimeoutTs = hardTimeoutForeverIndicator
	}

	if c.cacheType == inMemory {
		return inMemoryEncode(val, withSoftTimeoutTs(softTimeoutTs), withHardTimeoutTs(hardTimeoutTs))
	}

	b, ok := val.([]byte)
	if !ok {
		return nil, cacheErr("data_from_input_is_not_bytes")
	}

	return c.encodingHandler.encode(b, withSoftTimeoutTs(softTimeoutTs), withHardTimeoutTs(hardTimeoutTs))
}

func (c *cacheWrapper) set(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return nil
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	fixedKey := inner.getFixedKey(ctx, key)

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdSet,
		TotalKeyCount:  1,
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		stats.req = map[string]interface{}{key: value}

		var data interface{}
		data, err = inner.convertValueToCacheData(value, option)
		if err != nil {
			return err
		}

		expire = inner.translateExpire(ctx, expire)
		option.hardExpiration = expire

		var encodedData interface{}
		encodedData, err = inner.encode(data, option)
		if err != nil {
			return err
		}

		err = inner.cache.set(ctx, fixedKey, encodedData, expire, withWaitRistretto(option.waitRistretto))
		stats.RequestSize = len(fixedKey) + inner.getEncodedDataSize(encodedData)

		return err
	})

	return err
}

func (c *cacheWrapper) setMany(ctx context.Context, valueMap map[string]interface{}, expire time.Duration, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return nil
	}

	if len(valueMap) == 0 {
		return nil
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdSetMany,
		TotalKeyCount:  len(valueMap),
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		expire = inner.translateExpire(ctx, expire)
		err = c.setManyInner(ctx, valueMap, expire, stats, option)

		return err
	})

	return err
}

func (c *cacheWrapper) setManyInner(ctx context.Context, valueMap map[string]interface{}, expire time.Duration, stats *RequestStats, option *cacheOperationOptions) (err error) {
	stats.req = valueMap

	inner := c.loadCacheWrapperInner()

	requestSize := 0
	newValueMap := make(map[string]interface{}, len(valueMap))
	keys := make([]string, 0, len(valueMap))
	for k := range valueMap {
		keys = append(keys, k)
	}

	fixedKeys, _ := inner.getFixedKeys(ctx, keys)

	expirationMap := make(map[string]time.Duration)
	for idx, fixedKey := range fixedKeys {
		originalKey := keys[idx]
		value := valueMap[originalKey]

		var data interface{}
		data, err = inner.convertValueToCacheData(value, option)
		if err != nil {
			return
		}

		var expiration time.Duration
		if exp, ok := option.expirationMap[originalKey]; ok {
			expiration = inner.translateExpire(ctx, exp)
			expirationMap[fixedKey] = expiration
		} else {
			expiration = expire
		}

		option.hardExpiration = expiration

		var encodedData interface{}
		encodedData, err = inner.encode(data, option)
		if err != nil {
			return
		}

		newValueMap[fixedKey] = encodedData

		requestSize += len(fixedKey) + inner.getEncodedDataSize(encodedData)
	}

	stats.RequestSize = requestSize

	return inner.cache.setMany(ctx, newValueMap, expire, withNoReply(option.noReply), withExpirationMap(expirationMap), withWaitRistretto(option.waitRistretto))
}

// nolint:predeclared
func (c *cacheWrapper) delete(ctx context.Context, key string, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return nil
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	fixedKey := inner.getFixedKey(ctx, key)

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdDelete,
		TotalKeyCount:  1,
		RequestSize:    len(fixedKey),
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		stats.req = key
		err = inner.cache.delete(ctx, fixedKey)

		return err
	})

	return err
}

func (c *cacheWrapper) deleteMany(ctx context.Context, keys []string, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return nil
	}

	if len(keys) == 0 {
		return nil
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	return c.deleteManyInner(ctx, keys, inner, option)
}

// deleteManyInner deletes the cache items identified by the sharded keys.
func (c *cacheWrapper) deleteManyInner(ctx context.Context, shardedKeys []string, inner *cacheWrapperInner, option *cacheOperationOptions) error {
	var err error
	fixedKeys := make([]string, len(shardedKeys))
	requestSize := 0

	for _, shardedKey := range shardedKeys {
		requestSize += len(shardedKey)
	}

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdDeleteMany,
		TotalKeyCount:  len(fixedKeys),
		RequestSize:    requestSize,
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		stats.req = fixedKeys
		err = inner.cache.deleteMany(ctx, fixedKeys, withNoReply(option.noReply))

		return err
	})

	return err
}

func (c *cacheWrapper) add(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()
	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return nil
	}
	return c.addOrReplaceInner(ctx, key, value, expire, cmdAdd, opts)
}

func (c *cacheWrapper) replace(ctx context.Context, key string, value interface{}, expire time.Duration, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return ErrNotStored
	}
	return c.addOrReplaceInner(ctx, key, value, expire, cmdReplace, opts)
}

func (c *cacheWrapper) addOrReplaceInner(ctx context.Context, key string, value interface{}, expire time.Duration, command string, opts []OperationOption) error {
	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	inner := c.loadCacheWrapperInner()

	var err error

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: command,
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		stats.req = map[string]interface{}{key: value}

		var data interface{}
		data, err = inner.convertValueToCacheData(value, option)
		if err != nil {
			return err
		}

		expire = inner.translateExpire(ctx, expire)
		option.hardExpiration = expire

		var encodedData interface{}
		encodedData, err = inner.encode(data, option)
		if err != nil {
			return err
		}

		fixedKey := inner.getFixedKey(ctx, key)

		stats.RequestSize = len(fixedKey) + inner.getEncodedDataSize(encodedData)

		switch command {
		case cmdReplace:
			err = inner.cache.replace(ctx, fixedKey, encodedData, expire)
		case cmdAdd:
			err = inner.cache.add(ctx, fixedKey, encodedData, expire)
		default:
			// Logic error
			panic(cacheErr("unknown_operation"))
		}

		return err
	})

	return err
}

func (c *cacheWrapper) increment(ctx context.Context, key string, delta uint64, opts ...OperationOption) (num int64, err error) {
	return c.incrDecrInner(ctx, key, delta, cmdIncrement, opts...)
}

func (c *cacheWrapper) decrement(ctx context.Context, key string, delta uint64, opts ...OperationOption) (num int64, err error) {
	return c.incrDecrInner(ctx, key, delta, cmdDecrement, opts...)
}

func (c *cacheWrapper) incrDecrInner(ctx context.Context, key string, delta uint64, command string, opts ...OperationOption) (num int64, err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return 0, ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return 0, ErrCacheMiss
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	fixedKey := inner.getFixedKey(ctx, key)

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: command,
		RequestSize:    len(fixedKey),
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		stats.req = map[string]interface{}{key: delta}

		switch command {
		case cmdIncrement:
			num, err = inner.cache.increment(ctx, fixedKey, delta, withInitNonExistKey(option.initNonExistKey))
		case cmdDecrement:
			num, err = inner.cache.decrement(ctx, fixedKey, delta, withInitNonExistKey(option.initNonExistKey))
		default:
			// Logic error
			panic(cacheErr("unknown_operation"))
		}

		return err
	})

	return num, err
}

func (c *cacheWrapper) expire(ctx context.Context, key string, expire time.Duration, opts ...OperationOption) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return ErrCacheMiss
	}

	option := newCacheOperationOptions()
	defer recycleCacheOperationOptions(option)
	for _, opt := range opts {
		opt(option)
	}

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdExpire,
		RequestSize:    len(key),
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		expire = inner.translateExpire(ctx, expire)
		stats.req = map[string]interface{}{key: expire}

		err = c.expireInner(ctx, key, expire, inner, option)
		return err
	})
	return err
}

func (c *cacheWrapper) expireInner(ctx context.Context, fixedKey string, expire time.Duration, inner *cacheWrapperInner, option *cacheOperationOptions) (err error) {
	if option.skipEncodeDecode {
		err = inner.cache.expire(ctx, fixedKey, expire)
	} else {
		var result interface{}
		result, err = inner.cache.get(ctx, fixedKey)
		if err != nil {
			return err
		}

		var decodedData interface{}
		var header metaHeader
		decodedData, header, err = inner.decode(result, false)
		if err != nil {
			return err
		}

		option.hardExpiration = expire
		option.softTimeoutTs = header.SoftTimeoutTs

		var encodedData interface{}
		encodedData, err = inner.encode(decodedData, option)
		if err != nil {
			return err
		}

		err = inner.cache.set(ctx, fixedKey, encodedData, expire)
	}

	return err
}

func (c *cacheWrapper) flush(ctx context.Context) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return nil
	}

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdFlush,
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		err = inner.cache.flush(ctx)

		return err
	})

	return err
}

func (c *cacheWrapper) ping(ctx context.Context) (err error) {
	inner := c.loadCacheWrapperInner()

	if inner.isCacheClosed() {
		return ErrCacheClosed
	}
	if c.isDisabled(ctx) {
		return nil
	}

	stats := &RequestStats{
		CacheName:      inner.name,
		CacheType:      inner.cacheType.String(),
		CacheOperation: cmdPing,
		hostName:       inner.cacheHostName,
	}

	requestStatsDecorator(ctx, stats, func() error {
		err = inner.cache.ping(ctx)

		return err
	})

	return err
}

// rawClient is used to get the raw cache client,
// only applicable for cacheWrapper which is backed by RedisCache or MemcachedCache
func (c *cacheWrapper) rawClient() interface{} {
	inner := c.loadCacheWrapperInner()
	return inner.cache.rawClient()
}

// nolint:predeclared
func (c *cacheWrapper) close() error {
	inner := c.loadCacheWrapperInner()
	// compare current cache status, only if cache is not closed, then close the cache
	if atomic.CompareAndSwapUint32(&inner.isClosed, 0, 1) {
		return inner.cache.close()
	}
	// If someone already close the cache, will return nil, since the status already changed to 1
	return nil
}

// updateConfig updates the cacheWrapper with the new config
func (c *cacheWrapper) updateConfig(config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}

	c.updateMutex.Lock()
	defer c.updateMutex.Unlock()

	curInner := c.loadCacheWrapperInner()

	newInner := genCacheWrapperInnerCopy(curInner)
	var err error

	switch config.Type {
	case InMemory:
		inMemConfig := config.InMemory
		err = updateInMemCacheWrapperInner(newInner, inMemConfig)
		if err != nil {
			return err
		}
	default:
		return errorConfigTypeNotSupported
	}

	atomic.StorePointer(&c.inner, unsafe.Pointer(newInner))
	return nil
}

func updateInMemCacheWrapperInner(inner *cacheWrapperInner, config InMemoryCacheConfig) error {
	if err := inner.cache.(*inMemoryCacheInner).updateConfig(config); err != nil {
		return err
	}

	fillCacheWrapperInnerFieldsWithInMemConfig(config, inner)

	return nil
}

func (c *cacheWrapperInner) getFixedKey(ctx context.Context, key string) string {
	fixedKeys, _ := c.getFixedKeys(ctx, []string{key})
	return fixedKeys[0]
}

// getFixedKeys returns the fixed-to-originalKeys map, slice of fixed keys, total length of all keys
func (c *cacheWrapperInner) getFixedKeys(ctx context.Context, keys []string) (fixedKeys []string, totalKeysLength int) {
	newKeys := make([]string, len(keys))
	totalNewKeysLength := 0

	for i := range keys {
		//TODO  可以对key进行额外处理
		newKey := keys[i]
		newKeys[i] = newKey
		totalNewKeysLength += len(newKey)
	}

	return newKeys, totalNewKeysLength
}

func genCacheWrapperInnerCopy(oldInner *cacheWrapperInner) *cacheWrapperInner {
	newInner := &cacheWrapperInner{}

	newInner.name = oldInner.name
	newInner.cacheType = oldInner.cacheType
	newInner.cache = oldInner.cache
	newInner.cacheHostName = oldInner.cacheHostName
	newInner.defaultExpiration = oldInner.defaultExpiration
	newInner.maxExpiration = oldInner.maxExpiration
	newInner.codecHandler = oldInner.codecHandler
	newInner.encodingHandler = oldInner.encodingHandler
	newInner.manufacturerHandler = oldInner.manufacturerHandler

	return newInner
}

// translateExpire generates real expire based on the one passed-in by client
func (c *cacheWrapperInner) translateExpire(ctx context.Context, expire time.Duration) time.Duration {
	switch expire {
	case DefaultExpiration:
		expire = c.defaultExpiration
	case NoExpiration:
		expire = NoExpiration
	}

	if c.maxExpiration != 0 && expire > c.maxExpiration {
		expire = c.maxExpiration
	}

	return expire
}

// setCacheDataToReceiver sets cached data to receiver
func (c *cacheWrapperInner) setCacheDataToReceiver(data interface{}, receiver interface{}, option *cacheOperationOptions) error {
	if receiver == nil {
		return errNilReceiver
	}

	if c.cacheType == inMemory && option.skipCodec {
		return utils.SetValue(data, receiver)
	}

	b, ok := data.([]byte)
	if !ok {
		return errPassNonBytesToCodec
	}

	return c.codecHandler.unmarshal(b, receiver, option.codecType, option.customCodec)
}

// convertValueToCacheData converts value to to-cache data
func (c *cacheWrapperInner) convertValueToCacheData(value interface{}, option *cacheOperationOptions) (data interface{}, err error) {
	if c.cacheType == inMemory && option.skipCodec {
		return utils.GetValue(value), nil
	}

	bytesData, err := c.codecHandler.marshal(value, option.codecType, option.customCodec)
	if err != nil {
		return nil, err
	}

	return bytesData, nil
}

// nolint:unparam
func (c *cacheWrapperInner) decode(val interface{}, skip bool) (interface{}, metaHeader, error) {
	if skip {
		return val, metaHeader{}, nil
	}

	if c.cacheType == inMemory {
		return inMemoryDecode(val)
	}

	b, ok := val.([]byte)
	if !ok {
		return nil, metaHeader{}, cacheErr("data_from_cache_is_not_bytes")
	}

	return c.encodingHandler.decode(b)
}

func (c *cacheWrapperInner) getEncodedDataSize(val interface{}) int {
	if v, ok := val.([]byte); ok {
		return len(v)
	}
	return 0
}

func (c *cacheWrapperInner) isDisabledCtxOrConfig(ctx context.Context) bool {
	if c.isDisabled {
		return true
	}

	return false
}

func (c *cacheWrapper) isDisabled(ctx context.Context) bool {
	inner := c.loadCacheWrapperInner()

	return inner.isDisabledCtxOrConfig(ctx)
}

func (c *cacheWrapperInner) isCacheClosed() bool {
	// read the status, check whether current status is close or not
	return atomic.LoadUint32(&c.isClosed) == 1
}

func (c *cacheWrapper) loadCacheWrapperInner() *cacheWrapperInner {
	inner := (*cacheWrapperInner)(atomic.LoadPointer(&c.inner))
	return inner
}
