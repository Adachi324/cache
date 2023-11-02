package cache

import "strings"

// Error defines cache error
type Error struct {
	Message string
}

// Error defines error string content
func (c Error) Error() string {
	return "cache:" + c.Message
}

func cacheErr(msg string) Error {
	return Error{Message: msg}
}

var (
	// ErrCacheMiss means that a Get failed because the item wasn't present.
	ErrCacheMiss = cacheErr("cache_miss")

	// ErrNotStored means that a conditional write operation (i.e. Add) failed because the condition was not satisfied.
	ErrNotStored = cacheErr("not_stored")

	// ErrCacheClosed means that all resources used by the pool has been released, should not perform any operations after close.
	ErrCacheClosed = cacheErr("cache_closed")

	// ErrCacheDisabled means that current cache is disabled. It will be returned only for RedisCache.Do and RedisPipeline.Exec
	// as a general error when cache is disabled through config, for all the other specific cache operations, nil error is
	// returned for direct write operations such as Set, Delete, and non-nil error is returned for operations involving
	// read such as Get and Replace
	ErrCacheDisabled = cacheErr("cache_disabled")

	// errCacheNotExist means that the cache instance does not exists in the manager.
	errCacheNotExist = cacheErr("cache_instance_not_exist")

	// errNotSupported means the operation is not supported.
	//errNotSupported = cacheErr("operation_not_support")

	// errorConfigTypeNotSupported means the config type is unknown and not supported
	errorConfigTypeNotSupported = cacheErr("config_type_not_supported")

	// errNotSingleCache means that a method that is only for single cache received a non-single cache.
	errNotSingleCache = cacheErr("not_single_cache")

	// errDataLoaderNotReturnAllData means that the data loader of load/load many did not return full data list
	errDataLoaderNotReturnAllData = cacheErr("data_loader_not_return_results_for_all_keys")

	// errDataLoaderPanic means that the data loader of load/load many encountered panic issue
	errDataLoaderPanic = cacheErr("data_loader_panic")

	// errPassNonBytesToCodec means that the stored data in cache is not bytes and codec cannot handle
	errPassNonBytesToCodec = cacheErr("pass_non_bytes_to_codec")

	// errNilReceiver means the receiver(s) from Get/GetMany/Load/LoadMany is nil
	errNilReceiver = cacheErr("receiver_is_nil")

	// errHotKeyRegexpCompile means that the hot key regex patterns are invalid
	errHotKeyRegexpCompile = cacheErr("hotkey_regexp_compile_failed")

	errInMemoryConfigCacheTypeNotSupported = cacheErr("inmemory_config_cache_type_not_supported")

	// errContextTimeout means that cache operation is suspended due to context timeout, but not all timeout error will return this error
	errContextTimeout = cacheErr("cache_context_timeout_err")

	// errDlockLoss means that cache value in waiting instances is filled with nil data due to dlock loss when using the AcrossInstanceSignal strategy
	errDlockLoss = cacheErr("cache_value_fill_in_nil_due_to_dlock_loss")
)

func isTimeoutError(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func isRetryableError(err error) bool {
	return err != ErrCacheMiss && err != ErrCacheClosed && !isTimeoutError(err)
}
