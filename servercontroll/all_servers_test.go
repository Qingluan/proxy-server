package servercontroll

import (
	"net"
	"testing"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
	"gitee.com/dark.H/gs"
)

type reaperTestProto struct {
	cfg      *base.ProtocolConfig
	tryClose bool
}

func (f *reaperTestProto) GetListener() net.Listener       { return nil }
func (f *reaperTestProto) GetConfig() *base.ProtocolConfig { return f.cfg }
func (f *reaperTestProto) AcceptHandle(time.Duration, func(net.Conn) error) error {
	return nil
}
func (f *reaperTestProto) TryClose()                    { f.tryClose = true }
func (f *reaperTestProto) GetAliveIPS() gs.List[string] { return nil }
func (f *reaperTestProto) DelCon(con net.Conn)          { con.Close() }

func newReaperTunnel(id string) (*base.ProxyTunnel, *reaperTestProto) {
	proto := &reaperTestProto{cfg: &base.ProtocolConfig{ID: id, ProxyType: "quic"}}
	return base.NewProxyTunnel(proto), proto
}

func tunnelIDs() map[string]bool {
	ids := map[string]bool{}
	LockArea(func() {
		Tunnels.Every(func(no int, i *base.ProxyTunnel) {
			ids[i.GetConfig().ID] = true
		})
	})
	return ids
}

// holdStream drives HandleConnAsync through a real SOCKS5 redirect request
// and parks it inside ControllFunc, so GetClientNum reads 1. The returned
// release func must be called to unwind the stream.
func holdStream(t *testing.T, pt *base.ProxyTunnel) (release func()) {
	t.Helper()
	inControl := make(chan struct{})
	rel := make(chan struct{})
	pt.SetControllFunc(func(rawHost string, con net.Conn) error {
		close(inControl)
		<-rel
		con.Close()
		return nil
	})

	client, server := net.Pipe()
	go pt.HandleConnAsync(server)
	go func() {
		host := "R://reaper"
		b := []byte{0x05, 0x01, 0x00, 0x09, byte(len(host))}
		client.Write(append(b, host...))
	}()

	select {
	case <-inControl:
	case <-time.After(3 * time.Second):
		t.Fatal("ControllFunc never invoked; SOCKS5 request not parsed")
	}
	return func() {
		close(rel)
		client.Close()
		deadline := time.Now().Add(3 * time.Second)
		for pt.GetClientNum() != 0 {
			if time.Now().After(deadline) {
				t.Fatalf("GetClientNum stuck at %d after release", pt.GetClientNum())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// TestMidnightReaperDeletesOnlyIdleTunnels: one reaper pass must delete only
// the tunnel with zero live connections AND stale lastActive. A tunnel with
// an in-flight stream survives even with a stale timestamp; a fresh tunnel
// survives even with zero connections.
func TestMidnightReaperDeletesOnlyIdleTunnels(t *testing.T) {
	Tunnels = gs.List[*base.ProxyTunnel]{}

	activeStale, activeStaleProto := newReaperTunnel("t-active-stale")
	AddProxy(activeStale)
	release := holdStream(t, activeStale)
	defer release()
	activeStale.SetLastActive(time.Now().Add(-2 * time.Hour))

	fresh, _ := newReaperTunnel("t-fresh")
	AddProxy(fresh)

	idle, idleProto := newReaperTunnel("t-idle")
	AddProxy(idle)
	idle.SetLastActive(time.Now().Add(-2 * time.Hour))

	for _, id := range CollectIdleTunnelIDs(time.Now()) {
		DelProxy(id)
	}

	ids := tunnelIDs()
	if len(ids) != 2 || !ids["t-active-stale"] || !ids["t-fresh"] {
		t.Fatalf("after reaper pass, tunnels = %v, want only [t-active-stale t-fresh]", ids)
	}
	if ids["t-idle"] {
		t.Fatal("idle tunnel survived the reaper pass")
	}
	if !idleProto.tryClose {
		t.Fatal("idle tunnel was removed from Tunnels without SetWaitToClose teardown")
	}
	if activeStaleProto.tryClose {
		t.Fatal("active tunnel was torn down by the reaper")
	}
}

// TestCollectIdleTunnelIDSBoundary pins the strict-inequality threshold:
// 59min idle -> kept, exactly 1h -> kept, 61min idle -> collected.
func TestCollectIdleTunnelIDSBoundary(t *testing.T) {
	Tunnels = gs.List[*base.ProxyTunnel]{}
	now := time.Now()

	mk := func(id string, age time.Duration) {
		pt, _ := newReaperTunnel(id)
		AddProxy(pt)
		pt.SetLastActive(now.Add(-age))
	}
	mk("t-59min", 59*time.Minute)
	mk("t-exact-1h", time.Hour)
	mk("t-61min", 61*time.Minute)

	got := CollectIdleTunnelIDs(now)
	if len(got) != 1 || got[0] != "t-61min" {
		t.Fatalf("CollectIdleTunnelIDs = %v, want [t-61min]", got)
	}
}
