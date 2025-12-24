package base

import (
	"sync"
	"sync/atomic"
	"time"
)

// HealthMetrics tracks the health and performance metrics of a proxy tunnel
type HealthMetrics struct {
	totalConnections  atomic.Int64   // Total connections served
	activeConnections atomic.Int32   // Currently active connections
	failedConnections atomic.Int64   // Failed connections
	bytesTransferred  atomic.Int64   // Total bytes transferred
	lastHealthCheck   atomic.Value   // time.Time of last health check
	totalLatency      atomic.Int64   // Total latency in nanoseconds (for avg calculation)
	latencySamples    atomic.Int64   // Number of latency samples

	mu       sync.RWMutex
	maxConn  int32         // Maximum allowed connections
	isPaused atomic.Bool   // Whether new connections are paused
}

// NewHealthMetrics creates a new health metrics instance
func NewHealthMetrics(maxConnections int32) *HealthMetrics {
	hm := &HealthMetrics{
		maxConn: maxConnections,
	}
	hm.lastHealthCheck.Store(time.Now())
	return hm
}

// RecordConnection records a new connection
func (hm *HealthMetrics) RecordConnection() bool {
	if hm == nil {
		return true
	}

	// Check if we've hit the connection limit
	if hm.maxConn > 0 {
		current := hm.activeConnections.Load()
		if current >= hm.maxConn {
			hm.isPaused.Store(true)
			return false
		}
	}

	hm.totalConnections.Add(1)
	hm.activeConnections.Add(1)
	hm.isPaused.Store(false)
	return true
}

// ReleaseConnection records a closed connection
func (hm *HealthMetrics) ReleaseConnection() {
	if hm == nil {
		return
	}
	hm.activeConnections.Add(-1)
}

// RecordFailure records a failed connection
func (hm *HealthMetrics) RecordFailure() {
	if hm == nil {
		return
	}
	hm.failedConnections.Add(1)
}

// RecordBytes records bytes transferred
func (hm *HealthMetrics) RecordBytes(bytes int64) {
	if hm == nil {
		return
	}
	hm.bytesTransferred.Add(bytes)
}

// RecordLatency records a latency sample
func (hm *HealthMetrics) RecordLatency(latency time.Duration) {
	if hm == nil {
		return
	}
	hm.totalLatency.Add(int64(latency))
	hm.latencySamples.Add(1)
}

// GetStats returns current statistics
func (hm *HealthMetrics) GetStats() (total, active, failed, bytes int64, avgLatency time.Duration) {
	if hm == nil {
		return 0, 0, 0, 0, 0
	}
	total = hm.totalConnections.Load()
	active = int64(hm.activeConnections.Load())
	failed = hm.failedConnections.Load()
	bytes = hm.bytesTransferred.Load()

	samples := hm.latencySamples.Load()
	if samples > 0 {
		avgLatency = time.Duration(hm.totalLatency.Load() / samples)
	}
	return
}

// GetErrorRate returns the error rate (0.0 to 1.0)
func (hm *HealthMetrics) GetErrorRate() float64 {
	if hm == nil {
		return 0
	}
	total := hm.totalConnections.Load()
	if total == 0 {
		return 0
	}
	failed := hm.failedConnections.Load()
	return float64(failed) / float64(total)
}

// GetActiveCount returns the current active connection count
func (hm *HealthMetrics) GetActiveCount() int32 {
	if hm == nil {
		return 0
	}
	return hm.activeConnections.Load()
}

// IsHealthy returns whether the tunnel is healthy
func (hm *HealthMetrics) IsHealthy() bool {
	if hm == nil {
		return true
	}
	// Consider unhealthy if error rate > 50% or connections are paused
	if hm.isPaused.Load() {
		return false
	}
	errorRate := hm.GetErrorRate()
	return errorRate < 0.5
}

// GetLoadFactor returns the load factor (0.0 to 1.0)
func (hm *HealthMetrics) GetLoadFactor() float64 {
	if hm == nil {
		return 0
	}
	if hm.maxConn <= 0 {
		return 0
	}
	active := hm.activeConnections.Load()
	return float64(active) / float64(hm.maxConn)
}

// SetMaxConnections updates the maximum connections
func (hm *HealthMetrics) SetMaxConnections(max int32) {
	if hm == nil {
		return
	}
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.maxConn = max
}

// GetMaxConnections returns the maximum allowed connections
func (hm *HealthMetrics) GetMaxConnections() int32 {
	if hm == nil {
		return 0
	}
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.maxConn
}

// AcceptsNewConnections returns whether the tunnel accepts new connections
func (hm *HealthMetrics) AcceptsNewConnections() bool {
	if hm == nil {
		return true
	}
	if hm.maxConn <= 0 {
		return true
	}
	return hm.activeConnections.Load() < hm.maxConn
}

// CalculateScore calculates a health score for tunnel selection (0.0 to 1.0)
// Higher score is better
func (hm *HealthMetrics) CalculateScore() float64 {
	if hm == nil {
		return 0.5
	}

	// Factor 1: Error rate (lower is better) - weight 0.4
	errorRate := hm.GetErrorRate()
	errorScore := (1 - errorRate) * 0.4

	// Factor 2: Load factor (lower is better) - weight 0.3
	loadFactor := hm.GetLoadFactor()
	loadScore := (1 - loadFactor) * 0.3

	// Factor 3: Connection health (has capacity) - weight 0.3
	capacityScore := 0.3
	if hm.AcceptsNewConnections() && hm.IsHealthy() {
		capacityScore = 0.3
	} else {
		capacityScore = 0.0
	}

	return errorScore + loadScore + capacityScore
}
