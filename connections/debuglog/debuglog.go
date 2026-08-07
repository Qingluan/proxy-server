// Package debuglog appends timestamped lines to a dedicated log file for
// server-side debugging. It is intentionally standalone (stdlib only) so both
// connections/base and connections/prosmux can import it without an import
// cycle.
package debuglog

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// LogPath is the file every line is appended to.
var LogPath = "/tmp/proxy-server.err.log"

var mu sync.Mutex

// Write appends a timestamped line. It is thread-safe, does not block
// meaningfully, and silently ignores write errors, so it is safe to call from
// any hot path without affecting normal service.
func Write(format string, args ...any) {
	line := fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))

	mu.Lock()
	defer mu.Unlock()

	f, err := os.OpenFile(LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	f.WriteString(line)
	f.Close()
}
