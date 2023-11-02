package size

// CostOne is default for ristretto, the size of item is always=1,
// So the Ristretto::Capacity will be the total number of cache_keys.
var CostOne = func(val interface{}) int64 {
	return 1
}

// CostMemoryUsage function can be passed to ristretto's costFunc
// CostMemoryUsage will return the total bytes usage of val.
// It is suitable if we want to limit Ristretto by total memory usage
var CostMemoryUsage = func(val interface{}) int64 {
	return int64(Of(val))
}
