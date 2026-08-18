package proquic

import (
	"testing"
	"time"
)

// TestNewServerQuicConfig pins the server transport parameters that pair with
// the client half (proxy-z): a 15s keepalive must divide into the 90s idle
// timeout with at least a 3x margin, otherwise the two idle timers race and
// tear down live connections (recurring EOF resets).
func TestNewServerQuicConfig(t *testing.T) {
	c := newServerQuicConfig()
	if c == nil {
		t.Fatal("newServerQuicConfig returned nil")
	}
	if c.MaxIdleTimeout != 90*time.Second {
		t.Errorf("MaxIdleTimeout = %v, want 90s", c.MaxIdleTimeout)
	}
	if c.KeepAlivePeriod != 15*time.Second {
		t.Errorf("KeepAlivePeriod = %v, want 15s", c.KeepAlivePeriod)
	}
	if c.MaxIdleTimeout < 3*c.KeepAlivePeriod {
		t.Errorf("idle/keepalive margin %v/%v violates the >=3x invariant", c.MaxIdleTimeout, c.KeepAlivePeriod)
	}
	if c.MaxStreamReceiveWindow < 4<<20 {
		t.Errorf("MaxStreamReceiveWindow = %d, want >= 4MB", c.MaxStreamReceiveWindow)
	}
	if c.MaxConnectionReceiveWindow < 16<<20 {
		t.Errorf("MaxConnectionReceiveWindow = %d, want >= 16MB", c.MaxConnectionReceiveWindow)
	}
}
