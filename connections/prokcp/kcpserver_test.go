package prokcp

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
	"github.com/xtaci/kcp-go"
)

// TestAcceptTunedRoundTrip drives the real accept path (acceptTuned ->
// tuneKCPSession) over a loopback KCP listener and ping-pongs one byte.
func TestAcceptTunedRoundTrip(t *testing.T) {
	l, err := kcp.ListenWithOptions("127.0.0.1:0", nil, 10, 3)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	srvDone := make(chan error, 1)
	go func() {
		con, err := acceptTuned(l)
		if err != nil {
			srvDone <- err
			return
		}
		if _, ok := con.(*kcp.UDPSession); !ok {
			con.Close()
			srvDone <- fmt.Errorf("accepted conn is %T, tune branch skipped", con)
			return
		}
		con.SetDeadline(time.Now().Add(5 * time.Second))
		buf := make([]byte, 1)
		if _, err := io.ReadFull(con, buf); err != nil {
			con.Close()
			srvDone <- fmt.Errorf("server read: %w", err)
			return
		}
		if buf[0] != 0xAB {
			con.Close()
			srvDone <- fmt.Errorf("server got %#x, want 0xab", buf[0])
			return
		}
		_, err = con.Write([]byte{0xCD})
		con.Close()
		srvDone <- err
	}()

	cli, err := kcp.DialWithOptions(l.Addr().String(), nil, 10, 3)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()
	cli.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := cli.Write([]byte{0xAB}); err != nil {
		t.Fatalf("client write: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(cli, buf); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if buf[0] != 0xCD {
		t.Fatalf("client got %#x, want 0xcd", buf[0])
	}
	if err := <-srvDone; err != nil {
		t.Fatalf("server side: %v", err)
	}
}

// TestGetListenerBindsAfterBufferConfig guards the listen path against the
// SetReadBuffer/SetWriteBuffer additions breaking listener creation.
func TestGetListenerBindsAfterBufferConfig(t *testing.T) {
	c, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe port: %v", err)
	}
	port := c.Addr().(*net.TCPAddr).Port
	c.Close()

	srv := NewKcpServer(&base.ProtocolConfig{
		Server:     "127.0.0.1",
		ServerPort: port,
		Password:   "test-pass",
		SALT:       "test-salt",
	})
	l := srv.GetListener()
	if l == nil {
		t.Fatal("GetListener returned nil")
	}
	l.Close()
}
