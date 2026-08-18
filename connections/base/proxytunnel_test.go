package base

import (
	"net"
	"testing"
	"time"

	"gitee.com/dark.H/gs"
)

// fakeTunnelProto is the minimal Protocol implementation HandleConnAsync
// needs: GetConfig for tunnel identity, DelCon for cleanup. No listener is
// ever created.
type fakeTunnelProto struct {
	cfg *ProtocolConfig
}

func (f *fakeTunnelProto) GetListener() net.Listener  { return nil }
func (f *fakeTunnelProto) GetConfig() *ProtocolConfig { return f.cfg }
func (f *fakeTunnelProto) AcceptHandle(time.Duration, func(net.Conn) error) error {
	return nil
}
func (f *fakeTunnelProto) TryClose()                    {}
func (f *fakeTunnelProto) GetAliveIPS() gs.List[string] { return nil }
func (f *fakeTunnelProto) DelCon(con net.Conn)          { con.Close() }

// socks5RedirectRequest builds a SOCKS5 request with the redirect address
// type (9). GetServerRequest returns that host raw — no port append, no
// JoinHostPort bracketing (a "R://" host contains colons and would otherwise
// be mangled into "[R://x]:port") — so it reaches HandleConnAsync's
// ControllFunc branch intact. That branch needs no network.
func socks5RedirectRequest(host string) []byte {
	b := []byte{0x05, 0x01, 0x00, 0x09, byte(len(host))}
	return append(b, host...)
}

func waitClientNum(t *testing.T, pt *ProxyTunnel, want int, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for {
		if got := pt.GetClientNum(); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("GetClientNum = %d, want %d (timeout)", pt.GetClientNum(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestHandleConnAsyncAliveCounter guards the cons-list-to-atomic-counter
// replacement: the tunnel used to append every accepted conn to a list no
// one ever removed; only the count (GetClientNum) was consumed. The counter
// must read 1 while a stream is being handled and drop back to 0 once
// HandleConnAsync returns.
func TestHandleConnAsyncAliveCounter(t *testing.T) {
	pt := NewProxyTunnel(&fakeTunnelProto{cfg: &ProtocolConfig{ID: "t-alive"}})
	inControl := make(chan struct{})
	release := make(chan struct{})
	pt.SetControllFunc(func(rawHost string, con net.Conn) error {
		close(inControl)
		<-release
		return nil
	})

	client, server := net.Pipe()
	defer client.Close()
	go pt.HandleConnAsync(server)
	go func() {
		client.Write(socks5RedirectRequest("R://ping"))
	}()

	select {
	case <-inControl:
	case <-time.After(3 * time.Second):
		t.Fatal("ControllFunc never invoked; SOCKS5 request not parsed")
	}
	if got := pt.GetClientNum(); got != 1 {
		t.Fatalf("GetClientNum during handling = %d, want 1", got)
	}
	close(release)
	waitClientNum(t, pt, 0, 3*time.Second)
}

// TestAliveCounterStableAcrossCycles runs N sequential handle cycles and
// asserts the count returns to 0 after each one — the old cons list's length
// only ever grew across the same sequence.
func TestAliveCounterStableAcrossCycles(t *testing.T) {
	pt := NewProxyTunnel(&fakeTunnelProto{cfg: &ProtocolConfig{ID: "t-cycles"}})
	done := make(chan struct{}, 64)
	pt.SetControllFunc(func(rawHost string, con net.Conn) error {
		con.Close()
		done <- struct{}{}
		return nil
	})
	const cycles = 50
	for i := 0; i < cycles; i++ {
		client, server := net.Pipe()
		go pt.HandleConnAsync(server)
		go func() {
			client.Write(socks5RedirectRequest("R://x"))
			client.Close()
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("cycle %d: ControllFunc not reached", i)
		}
		waitClientNum(t, pt, 0, 3*time.Second)
	}
	if got := pt.GetClientNum(); got != 0 {
		t.Fatalf("GetClientNum after %d cycles = %d, want 0", cycles, got)
	}
}
