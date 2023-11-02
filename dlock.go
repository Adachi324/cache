package cache

import (
	"bytes"
	"context"
	"math/rand"
	"time"
)

const dlockPrefix = "dlock."

const (
	// defaultDlockUnitExpirationMillis will be used as the default unit expiration for distributed lock
	defaultDlockUnitExpirationMillis = 5000
	// defaultDlockRetryIntervalMillis will be used as the default interval for retrying to get values
	defaultDlockRetryIntervalMillis = 100
)

// We init rand seed for the global variable of rand package, so it may affect the random seed of the service if both use the global variable of rand package.
// But it should be fine because it just initiates a seed, which can be triggered by either unified cache or integrated services.
// If Users want to remain seeds unchanged, they should NOT use the global variable of rand package
func init() {
	rand.Seed(time.Now().Unix())
}

func addDlockPrefix(key string) string {
	return dlockPrefix + key
}

// randDlockValue is the randome unique 32-bit value for the distributed lock
// nolint:gosec
func randDlockValue() []byte {
	bytesData := make([]byte, 4)
	_, _ = rand.Read(bytesData)
	return bytesData
}

// reclassifyToHandleKeys re-classifies the toHandleKeys into toHandleKeys and waitingAcrossInstanceKeys.
// Reclassification is implemented by the distributed lock [For Lock-Holders and Lock-Listeners].
func reclassifyToHandleKeys(ctx context.Context, inner *cacheWrapperInner, toHandleKeys []string, value []byte, expire time.Duration) ([]string, []string, error) {
	var waitingAcrossInstanceKeys []string
	isAcquire, err := acquireDlock(ctx, inner, toHandleKeys, value, expire)
	if err != nil {
		return toHandleKeys, waitingAcrossInstanceKeys, err
	}
	// iterate reversely to avoid the impact when we shrink the slice itself
	for idx := len(toHandleKeys) - 1; idx >= 0; idx-- {
		// If the key fails to get the dlock, the key is (being) loaded by the other instance,
		// thus we need to add the key into `waitingAcrossInstanceKeys` and remove it from `toHandleKeys`.
		if !isAcquire[idx] {
			waitingAcrossInstanceKeys = append(waitingAcrossInstanceKeys, toHandleKeys[idx])
			// delete the corresponding key in toHandleKeys
			toHandleKeys[idx] = toHandleKeys[len(toHandleKeys)-1]
			toHandleKeys = toHandleKeys[:len(toHandleKeys)-1]
		}
	}
	return toHandleKeys, waitingAcrossInstanceKeys, nil
}

// acquireDlock tries to acquire the dlock [For Lock-Holders and Lock-Listeners].
func acquireDlock(ctx context.Context, inner *cacheWrapperInner, keys []string, value []byte, expire time.Duration) ([]bool, error) {
	isAcquire := make([]bool, len(keys))
	for idx, key := range keys {
		err := inner.cache.add(ctx, addDlockPrefix(key), value, expire)
		if err != nil && err != ErrNotStored {
			// the acquired keys will be released later, no need to release them here
			// we can return error directly
			return nil, err
		}
		isAcquire[idx] = err == nil
	}
	return isAcquire, nil
}

// releaseDlock releases dlock after value validation [For Lock-Holders].
func releaseDlock(ctx context.Context, inner *cacheWrapperInner, loadResultMap map[string]loadResult, value []byte) {
	if value == nil || len(loadResultMap) == 0 {
		return
	}
	for key := range loadResultMap {
		// get error does not matter when we release keys, since it will be expired within at most DlockUnitExpiration
		curDlockVal, _ := getDlock(ctx, inner, key)

		if bytes.Equal(curDlockVal, value) {
			_ = inner.cache.delete(ctx, addDlockPrefix(key))
		}
	}
}

// extendDlock extends dlock after value validation [For Lock-Holders].
// If validation has gone wrong, it will take at most the duration of DlockUnitExpiration for dlock to expire.
func extendDlock(ctx context.Context, inner *cacheWrapperInner, keys []string, value []byte, expire time.Duration) {
	for _, key := range keys {
		curDlockVal, err := getDlock(ctx, inner, key)
		if err != nil {
			continue
		}
		if bytes.Equal(curDlockVal, value) {
			err := inner.cache.expire(ctx, addDlockPrefix(key), expire)
			if err != nil {
				//TODO 添加日志

			}
		} else {
			// this log may appear many times, since lock extension will continue even if it has failed before
			//TODO 添加日志
		}
	}
}

// getLostDlockKeys gets keys whose dlock is no longer held by the previous owner [For Lock-Listeners].
func getLostDlockKeys(ctx context.Context, inner *cacheWrapperInner, keys []string, oldDlockMap map[string][]byte) (lostDlockKeys []string) {
	for _, key := range keys {
		curDlockVal, err := getDlock(ctx, inner, key)
		// if dlock does not even exist previously, or possession status of dlock is different, or the err is not retryable,
		// we need to note its loss in lostDlockKeys
		isLost := (oldDlockMap[key] == nil) ||
			(err == nil && !bytes.Equal(curDlockVal, oldDlockMap[key])) ||
			(err != nil && !isRetryableError(err))
		if isLost {
			lostDlockKeys = append(lostDlockKeys, key)
		}
	}
	return lostDlockKeys
}

// getDlock gets value from dlock's keys [For Lock-Holder and Lock-Listeners].
func getDlock(ctx context.Context, inner *cacheWrapperInner, key string) (byteData []byte, err error) {
	data, err := inner.cache.get(ctx, addDlockPrefix(key))
	if err != nil {
		return nil, err
	}
	byteData, _ = data.([]byte)
	return byteData, nil
}

// getDlockMap gets values map from dlock's keys [For Lock-Listeners].
func getDlockMap(ctx context.Context, inner *cacheWrapperInner, keys []string) map[string][]byte {
	dlockMap := make(map[string][]byte, len(keys))
	for _, key := range keys {
		val, _ := getDlock(ctx, inner, key)
		dlockMap[key] = val
	}
	return dlockMap
}
