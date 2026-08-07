package debuglog

import (
	"os"
	"strings"
	"sync"
	"testing"
)

func TestWriteAppends(t *testing.T) {
	const testPath = "/tmp/proxy-server-debuglog-test.log"
	os.Remove(testPath)

	old := LogPath
	LogPath = testPath
	defer func() { LogPath = old; os.Remove(testPath) }()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Write("line %d", n)
		}(i)
	}
	wg.Wait()

	b, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 50 {
		t.Fatalf("expected 50 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "line ") {
		t.Fatalf("unexpected content: %q", lines[0])
	}
}
