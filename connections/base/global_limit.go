package base

import "sync/atomic"

var (
	// globalStreamLimit caps concurrent streams across ALL tunnels. It is the
	// primary defence against FD/memory exhaustion: per-tunnel limits alone
	// cannot stop one client from exhausting the process. 0 = unlimited.
	globalStreamLimit atomic.Int64
	activeStreams     atomic.Int64
)

// SetGlobalStreamLimit sets the maximum concurrent streams across all tunnels.
func SetGlobalStreamLimit(n int64) {
	globalStreamLimit.Store(n)
}

// GlobalActiveStreams returns the number of streams currently being processed.
func GlobalActiveStreams() int64 {
	return activeStreams.Load()
}

// TryAcquireStream reserves a slot in the global stream budget.
func TryAcquireStream() bool {
	if globalStreamLimit.Load() <= 0 {
		return true
	}
	cur := activeStreams.Add(1)
	if cur > globalStreamLimit.Load() {
		activeStreams.Add(-1)
		return false
	}
	return true
}

// ReleaseStream releases a slot reserved by TryAcquireStream.
func ReleaseStream() {
	activeStreams.Add(-1)
}
