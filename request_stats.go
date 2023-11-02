package cache

import (
	"context"
	"time"
)

type observationConfig struct {
	slowOperationLogConfig slowOperationLogConfig
}

type slowOperationLogConfig struct {
	disable                   bool
	elapseThresholdForSlowLog time.Duration // the minimum time threshold for logging slow operations, default as 100ms.
}

// RequestStats defines stats to be used to record metrics
type RequestStats struct {
	CacheType      string // "redis", or "memcached"
	CacheName      string // the name defined in config
	CacheOperation string // e.g. "Get", "GetMany", ...
	hostName       string // host name can be the format of ip:port, used by tracing in Redis only

	req             interface{} // reference of request sent to Cache, for outputting debug message into tracing span
	resp            interface{} // reference of response received from Cache, for outputting debug message into tracing span
	RequestSize     int         // size of request (in bytes) for this cache operation
	ResponseSize    int         // size of response (in bytes) for this cache operation
	TotalKeyCount   int         // total key count for this cache operation
	SuccessKeyCount int         // success key count for this cache operation, can be used to calculate cache hit ratio

	startTime time.Time
	endTime   time.Time
	Elapsed   time.Duration // the duration of this single cache internal operation

	Err error // error occurred during cache operation (used by tracing only)

	skipOperationLogs bool // to determine if needed to skip logs on the operation
}

func (rs *RequestStats) needToReportOperationLogs() bool {
	return !rs.skipOperationLogs
}

// requestStatsDecorator injects metrics into f. Target function should populate stats through closure.
func requestStatsDecorator(ctx context.Context, stats *RequestStats, f func() error) {
	startTime := time.Now()

	stats.Err = f()

	stats.Elapsed = time.Since(startTime)

	//TODO 可以在这里后续添加监控等信息

}

// StatsCollector defines interface of stats collector
type StatsCollector interface {
	CollectStats(ctx context.Context, stats RequestStats)
}

var defaultPrometheusCollector = &defaultCollector{}

const unknown = "unknown"

type defaultCollector struct{}
