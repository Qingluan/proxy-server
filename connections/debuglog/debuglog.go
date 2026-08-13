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

// reopenEvery reopens the log file after this many writes so external log
// rotation is picked up without paying an open/close syscall per line.
const reopenEvery = 10000

type writer struct {
	mu     sync.Mutex
	f      *os.File
	writes uint64
}

var w writer

func (w *writer) write(line string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil || w.writes%reopenEvery == 0 {
		if w.f != nil {
			w.f.Close()
			w.f = nil
		}
		f, err := os.OpenFile(LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		w.f = f
	}
	w.writes++
	w.f.WriteString(line)
}

// Write appends a timestamped line. It is thread-safe, does not block
// meaningfully, and silently ignores write errors, so it is safe to call from
// any hot path without affecting normal service.
func Write(format string, args ...any) {
	w.write(fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...)))
}

// sampler rate-limits high-volume log lines under a "first firstN always, then
// every modNth" policy, keyed per category so unrelated lines don't share a
// budget.
type sampler struct {
	mu       sync.Mutex
	counters map[string]uint64
}

var smp = sampler{counters: make(map[string]uint64)}

// Allow reports whether a line for the given category should be logged.
// The first firstN calls return true, then every modNth call returns true.
func Allow(category string, firstN, modN uint64) bool {
	smp.mu.Lock()
	defer smp.mu.Unlock()
	n := smp.counters[category] + 1
	smp.counters[category] = n
	return n <= firstN || n%modN == 0
}
