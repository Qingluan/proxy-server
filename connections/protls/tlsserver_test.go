package protls

import (
	"fmt"
	"net"
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
