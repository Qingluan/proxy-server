// Package debuglog appends timestamped lines to a dedicated log file for
// server-side debugging. It is intentionally standalone (stdlib only) so both
// connections/base and connections/prosmux can import it without an import
// cycle.
package debuglog

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

// enabled gates hot-path debug lines. It is initialized once from the
// PUZZLE_DEBUG env var (any non-empty value enables) and can be flipped at
// runtime via SetEnabled (tests, load-test evidence collection).
var enabled atomic.Bool

func init() {
	enabled.Store(os.Getenv("PUZZLE_DEBUG") != "")
}

// SetEnabled turns the hot-path debug gate on or off.
func SetEnabled(v bool) { enabled.Store(v) }

// Enabled reports whether the hot-path debug gate is currently on.
func Enabled() bool { return enabled.Load() }

// gatedPrefixes lists the hot-path debug categories that are suppressed when
// the gate is off. These are the per-request volume lines. Everything NOT
// listed here (session lifecycle, tunnel lifecycle, errors, overload limits)
// is always logged: load-test metrics parse those lines, so they must never
// be silenced.
var gatedPrefixes = []string{"[req]", "[dial] SLOW"}

func gated(format string) bool {
	for _, p := range gatedPrefixes {
		if strings.HasPrefix(format, p) {
			return true
		}
	}
	return false
}

// Write appends a timestamped line. It is thread-safe, does not block
// meaningfully, and silently ignores write errors, so it is safe to call from
// any hot path without affecting normal service. Hot-path debug categories
// (see gatedPrefixes) become no-ops unless the PUZZLE_DEBUG gate is enabled.
func Write(format string, args ...any) {
	if gated(format) && !enabled.Load() {
		return
	}
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
