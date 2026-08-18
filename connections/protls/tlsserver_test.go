package protls

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
)

func freeEphemeralPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ephemeral listener: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// TestAcceptLoopSurvivesIdle guards against the removed idle-suicide logic:
// the accept loop used to exit one tick (1 minute) after the last accepted
// connection, leaving a zombie listener that kept the port bound after
// ProxyTunnel.Server had already released it in accounting.
//
// The tick is gone entirely, so we cannot wait a real minute; instead we
// verify the invariants the fix guarantees:
//  1. connections are still accepted after idle gaps,
//  2. TryClose (explicit deletion via DelProxy -> SetWaitToClose) makes
//     AcceptHandle return, and
//  3. when it returns the port is actually freed (defer listener.Close()).
func TestAcceptLoopSurvivesIdle(t *testing.T) {
	port := freeEphemeralPort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	srv := NewTlsServer(&base.ProtocolConfig{Server: "127.0.0.1", ServerPort: port})

	accepted := make(chan net.Conn, 8)
	returned := make(chan error, 1)
	go func() {
		returned <- srv.AcceptHandle(time.Minute, func(con net.Conn) error {
			accepted <- con
			return nil
		})
	}()

	// dialAndExpect asserts a fresh connection is picked up by the accept
	// loop. Note: net.DialTimeout alone would succeed even against a zombie
	// listener (TCP handshake completes into the backlog) — waiting for the
	// accepted channel is what proves the loop is alive.
	dialAndExpect := func(step string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		var c net.Conn
		for {
			var err error
			c, err = net.DialTimeout("tcp", addr, time.Second)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: dial: %v", step, err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		defer c.Close()
		select {
		case got := <-accepted:
			got.Close()
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: connection not accepted (accept loop dead?)", step)
		}
	}

	dialAndExpect("first conn")
	time.Sleep(2 * time.Second)
	dialAndExpect("conn after 2s idle gap")
	time.Sleep(2 * time.Second)
	dialAndExpect("conn after another 2s idle gap")

	srv.TryClose()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("AcceptHandle did not return after TryClose")
	}

	rel, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port %d still bound after AcceptHandle returned: %v", port, err)
	}
	rel.Close()
}

// remoteAddrConn pins RemoteAddr to a fixed address; Record/DelRecord only
// ever look at RemoteAddr, never at the underlying conn.
type remoteAddrConn struct {
	net.Conn
	remote net.Addr
}

func (c *remoteAddrConn) RemoteAddr() net.Addr { return c.remote }

func tcpAddr(ip string, port int) net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}

// TestRecordDelRecordChurn guards the ips-map unbounded growth: Record keys
// the map by remote addr (port included, so every connection is a fresh key)
// and DelRecord's inverted !ok condition made every delete a no-op. After the
// fix, a full open/close cycle must return the map to its baseline size.
func TestRecordDelRecordChurn(t *testing.T) {
	srv := NewTlsServer(&base.ProtocolConfig{Server: "127.0.0.1", ServerPort: 0})
	if got := len(srv.GetAliveIPS()); got != 0 {
		t.Fatalf("baseline alive ips = %d, want 0", got)
	}
	const cycles = 1000
	for i := 0; i < cycles; i++ {
		a := tcpAddr("10.0.0.1", 20000+i)
		srv.Record(a)
		srv.DelRecord(&remoteAddrConn{remote: a})
	}
	if got := len(srv.GetAliveIPS()); got != 0 {
		t.Fatalf("alive ips after %d open/close cycles = %d, want 0 (map never shrank)", cycles, got)
	}
}

// TestRecordGetAliveIPSParallel drives concurrent Record/DelRecord churn
// against concurrent GetAliveIPS iteration. Before the fix, Record's
// presence check ran before taking the lock and GetAliveIPS iterated the map
// unlocked — concurrent map iteration + write is a fatal (unrecoverable)
// runtime crash that the race detector flags.
func TestRecordGetAliveIPSParallel(t *testing.T) {
	srv := NewTlsServer(&base.ProtocolConfig{Server: "127.0.0.1", ServerPort: 0})
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				a := tcpAddr(fmt.Sprintf("10.0.%d.%d", w, i%250), 30000+i)
				srv.Record(a)
				srv.DelRecord(&remoteAddrConn{remote: a})
			}
		}(w)
	}
	for r := 0; r < 2; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				_ = srv.GetAliveIPS()
			}
		}()
	}
	wg.Wait()
	if got := len(srv.GetAliveIPS()); got != 0 {
		t.Fatalf("alive ips after parallel churn = %d, want 0", got)
	}
}
