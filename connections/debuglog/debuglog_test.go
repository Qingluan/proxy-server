package debuglog

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func resetWriter() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil {
		w.f.Close()
		w.f = nil
	}
	w.writes = 0
}

func withTempLog(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "debug.log")
	old := LogPath
	LogPath = path
	resetWriter()
	t.Cleanup(func() {
		LogPath = old
		SetEnabled(false)
		resetWriter()
	})
	return path
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log %s: %v", path, err)
	}
	return string(data)
}

func TestGateDisabledSuppressesHotPath(t *testing.T) {
	path := withTempLog(t)
	SetEnabled(false)

	Write("[req] host=%s dial=%s ok", "example.com", "1ms")
	Write("[dial] SLOW host=%s elapsed=%s", "example.com", "2s")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("gate disabled but log output exists (stat err=%v): %q", err, readLog(t, path))
	}
}

func TestGateDisabledKeepsLifecycleAndErrors(t *testing.T) {
	path := withTempLog(t)
	SetEnabled(false)

	Write("[smux] session end err=%v alive=%s streams=%d", nil, "1m0s", 1)
	Write("[smux] session create err=%v", nil)
	Write("[dial] fail host=%s err=%v elapsed=%s", "example.com", "boom", "1s")
	Write("[dial] confirm-write fail host=%s err=%v", "example.com", "broken pipe")
	Write("[handshake] err=%v", "short read")
	Write("[tunnel] start type=%s port=%d id=%s", "tls", 20001, "abc")

	got := readLog(t, path)
	for _, want := range []string{
		"[smux] session end err=<nil> alive=1m0s streams=1",
		"[smux] session create err=<nil>",
		"[dial] fail host=example.com err=boom elapsed=1s",
		"[dial] confirm-write fail host=example.com err=broken pipe",
		"[handshake] err=short read",
		"[tunnel] start type=tls port=20001 id=abc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lifecycle/error line missing when gate disabled: %q\ngot:\n%s", want, got)
		}
	}
}

func TestGateEnabledWritesHotPath(t *testing.T) {
	path := withTempLog(t)
	SetEnabled(true)

	Write("[req] host=%s dial=%s ok", "example.com", "1ms")

	got := readLog(t, path)
	if !strings.Contains(got, "[req] host=example.com dial=1ms ok") {
		t.Fatalf("gate enabled but [req] line missing, got:\n%s", got)
	}
}

func TestSetEnabledRoundTrip(t *testing.T) {
	path := withTempLog(t)

	SetEnabled(true)
	if !Enabled() {
		t.Fatal("Enabled() false after SetEnabled(true)")
	}
	Write("[req] host=%s dial=%s ok", "a.com", "1ms")

	SetEnabled(false)
	if Enabled() {
		t.Fatal("Enabled() true after SetEnabled(false)")
	}
	Write("[req] host=%s dial=%s ok", "b.com", "1ms")

	got := readLog(t, path)
	if strings.Contains(got, "b.com") {
		t.Fatalf("line written after gate re-disabled:\n%s", got)
	}
	if !strings.Contains(got, "a.com") {
		t.Fatalf("line missing from enabled phase:\n%s", got)
	}
}

func TestWriteAppends(t *testing.T) {
	path := withTempLog(t)
	SetEnabled(false)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Write("line %d", n)
		}(i)
	}
	wg.Wait()

	got := readLog(t, path)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) != 50 {
		t.Fatalf("expected 50 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "line ") {
		t.Fatalf("unexpected content: %q", lines[0])
	}
}

func TestConcurrentWritesRaceFree(t *testing.T) {
	path := withTempLog(t)
	SetEnabled(false)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Write("[req] host=%s dial=%s ok", "c.com", "1ms")
			Write("[smux] session end err=%v alive=%s streams=%d", nil, "1s", 0)
		}()
	}
	wg.Wait()

	SetEnabled(true)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Write("[req] host=%s dial=%s ok", "c.com", "1ms")
		}()
	}
	wg.Wait()

	got := readLog(t, path)
	if n := strings.Count(got, "[smux] session end"); n != 50 {
		t.Fatalf("session-end count = %d, want 50", n)
	}
	if n := strings.Count(got, "[req]"); n != 50 {
		t.Fatalf("gated [req] count = %d, want 50 (enabled phase only)", n)
	}
}
