package prosmux

import (
	"bytes"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xtaci/smux"
)

// TestNewStreamSurvivesBulkBufferFill guards against the session-bucket
// starvation regression (plan G13): with the old 4MB/2MB ratio, ~4MB of
// unread bulk data parks the server recvLoop in its bucket wait, after
// which new-stream SYNs are never processed — even a tiny round-trip on a
// fresh stream stalls. The fixed 16MB/1MB ratio keeps 4MB of headroom over
// the 12MB this test pushes, so the probe below must stay fast.
//
// Starvation mechanics that make this deterministic on protocol v1:
// writeV1 has no per-stream window throttle, so the session receive bucket
// is the ONLY flow control; tokens return only when the application reads
// the parked streams, which this test never does. If the buffer ratio ever
// regresses, the probe fails (bounded by the 5s watchdog below), not the
// bulk writers.
func TestNewStreamSurvivesBulkBufferFill(t *testing.T) {
	const (
		bulkStreams = 6
		bulkBytes   = 2 << 20 // 2MB per stream; 6*2MB = 12MB, must stay < MaxReceiveBuffer
		probeSize   = 64
	)

	pcfg := &SmuxConfig{}
	pcfg.SetAsDefault()
	scfg := pcfg.GenerateConfig()
	if scfg.MaxReceiveBuffer < bulkStreams*bulkBytes {
		t.Fatalf("test premise broken: session bucket %d cannot absorb %d bytes of unread bulk data; "+
			"buffer ratio regressed?", scfg.MaxReceiveBuffer, bulkStreams*bulkBytes)
	}

	c1, c2 := net.Pipe()
	t.Cleanup(func() {
		c1.Close()
		c2.Close()
	})

	srv, err := smux.Server(c1, scfg)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := smux.Client(c2, scfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cli.Close()
		srv.Close()
	})

	// Server: park the first bulkStreams streams (never read → their tokens
	// never return to the bucket), echo on every stream after them.
	// Parked wrappers MUST stay referenced: AcceptStream registers a
	// runtime finalizer that Close()s the stream (sending FIN to the
	// client) as soon as the wrapper is garbage collected.
	parked := make(chan *smux.Stream, bulkStreams)
	go func() {
		accepted := 0
		for {
			st, err := srv.AcceptStream()
			if err != nil {
				return
			}
			accepted++
			if accepted <= bulkStreams {
				parked <- st
				continue
			}
			go func(s *smux.Stream) {
				buf := make([]byte, probeSize)
				if _, err := io.ReadFull(s, buf); err != nil {
					return
				}
				s.Write(buf)
				s.Close()
			}(st)
		}
	}()

	// Bulk writers. No t.* calls here: under a regressed ratio these
	// goroutines stay blocked until session teardown, which may happen
	// after the test function has returned.
	var written atomic.Int64
	firstErr := make(chan error, 2*bulkStreams)
	for i := 0; i < bulkStreams; i++ {
		go func() {
			chunk := make([]byte, 32*1024)
			st, err := cli.OpenStream()
			if err != nil {
				firstErr <- err
				return
			}
			defer st.Close()
			sent := 0
			for sent < bulkBytes {
				n, err := st.Write(chunk)
				written.Add(int64(n))
				sent += n
				if err != nil {
					firstErr <- err
					return
				}
			}
		}()
	}

	// Wait for all bulk data to land in the server's stream buffers.
	total := int64(bulkStreams * bulkBytes)
	gateDeadline := time.Now().Add(10 * time.Second)
	for written.Load() < total && time.Now().Before(gateDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := written.Load(); got < total {
		t.Logf("bulk writers stalled at %d/%d bytes — session bucket starved", got, total)
		select {
		case err := <-firstErr:
			t.Logf("first writer error: %v", err)
		default:
		}
	}

	// Watchdog: a regressed ratio stalls OpenStream inside the fork's
	// 30s openCloseTimeout; force the session closed to fail fast.
	watchdog := time.AfterFunc(5*time.Second, func() { cli.Close() })
	defer watchdog.Stop()

	probeStart := time.Now()
	probe, err := cli.OpenStream()
	if err != nil {
		t.Fatalf("new stream blocked for %v under bulk pressure: %v (starvation regression)",
			time.Since(probeStart), err)
	}
	probe.SetDeadline(time.Now().Add(2 * time.Second))

	payload := make([]byte, probeSize)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	if _, err := probe.Write(payload); err != nil {
		t.Fatalf("probe write: %v (starvation regression)", err)
	}
	echo := make([]byte, probeSize)
	if _, err := io.ReadFull(probe, echo); err != nil {
		t.Fatalf("probe round-trip incomplete after %v: %v (starvation regression)",
			time.Since(probeStart), err)
	}
	if !bytes.Equal(payload, echo) {
		t.Fatal("probe payload mismatch")
	}
	if elapsed := time.Since(probeStart); elapsed > 500*time.Millisecond {
		t.Fatalf("new-stream round-trip took %v (>500ms) with %d bytes of unread bulk data (starvation regression)",
			elapsed, written.Load())
	}
	t.Logf("new-stream round-trip with %d bytes unread bulk data: %v", written.Load(), time.Since(probeStart))
}
