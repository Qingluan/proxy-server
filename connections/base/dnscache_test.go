package base

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestCache(resolve func(host string) ([]net.IP, error)) *DNSCache {
	c := NewDNSCache()
	c.resolve = resolve
	return c
}

func TestDNSCacheTTL(t *testing.T) {
	var calls atomic.Int64
	c := newTestCache(func(host string) ([]net.IP, error) {
		calls.Add(1)
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})
	c.ttl = 40 * time.Millisecond

	for i := 0; i < 3; i++ {
		if _, err := c.Lookup("ttl.test"); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("within TTL: expected 1 resolve, got %d", got)
	}

	time.Sleep(60 * time.Millisecond)
	if _, err := c.Lookup("ttl.test"); err != nil {
		t.Fatalf("post-TTL lookup: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("after TTL expiry: expected 2 resolves, got %d", got)
	}
}

func TestDNSCacheHitCounter(t *testing.T) {
	c := newTestCache(func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})
	for i := 0; i < 5; i++ {
		if _, err := c.Lookup("hit.test"); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	if c.Hits() != 4 || c.Misses() != 1 {
		t.Fatalf("counters: hits=%d misses=%d, want 4/1", c.Hits(), c.Misses())
	}
}

func TestDNSCacheSingleFlight(t *testing.T) {
	var calls atomic.Int64
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	c := newTestCache(func(host string) ([]net.IP, error) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return []net.IP{net.ParseIP("10.0.0.1")}, nil
	})

	const n = 50
	ips := make([][]net.IP, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ips[i], errs[i] = c.Lookup("sf.test")
		}(i)
	}
	<-started
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("single-flight violated: %d underlying resolves for %d lookups", got, n)
	}
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("lookup %d errored: %v", i, errs[i])
		}
		if len(ips[i]) != 1 || !ips[i][0].Equal(net.ParseIP("10.0.0.1")) {
			t.Fatalf("lookup %d got %v", i, ips[i])
		}
	}
}

func TestDNSCacheEviction(t *testing.T) {
	c := newTestCache(func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})
	total := dnsCacheMaxEntries + 100
	for i := 0; i < total; i++ {
		if _, err := c.Lookup(fmt.Sprintf("host-%d.test", i)); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if got := c.Len(); got > dnsCacheMaxEntries {
		t.Fatalf("cache holds %d entries, cap is %d", got, dnsCacheMaxEntries)
	}

	c.mu.Lock()
	_, oldest := c.entries["host-0.test"]
	_, newest := c.entries[fmt.Sprintf("host-%d.test", total-1)]
	c.mu.Unlock()
	if oldest {
		t.Fatalf("FIFO eviction failed: oldest entry still cached")
	}
	if !newest {
		t.Fatalf("newest entry missing after eviction")
	}
}

func TestDialCachedFallsBackOnResolveError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		con, _ := ln.Accept()
		accepted <- con
	}()

	c := newTestCache(func(host string) ([]net.IP, error) {
		return nil, fmt.Errorf("resolve boom")
	})
	// "localhost" so the fallback hostname dial resolves for real and
	// reaches the loopback listener (the fake name would not).
	conn, err := dialCached(c, &net.Dialer{Timeout: 2 * time.Second}, "localhost:"+fmt.Sprint(ln.Addr().(*net.TCPAddr).Port))
	if err != nil {
		t.Fatalf("fallback dial failed: %v", err)
	}
	defer conn.Close()
	select {
	case ac := <-accepted:
		ac.Close()
	case <-time.After(2 * time.Second):
		t.Fatalf("fallback dial reached no listener")
	}
}

func TestDialCachedLiteralIPBypassesCache(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		con, _ := ln.Accept()
		accepted <- con
	}()

	c := newTestCache(func(host string) ([]net.IP, error) {
		t.Errorf("resolver must not be called for literal IP, got %q", host)
		return nil, nil
	})
	conn, err := dialCached(c, &net.Dialer{Timeout: 2 * time.Second}, ln.Addr().String())
	if err != nil {
		t.Fatalf("literal dial failed: %v", err)
	}
	defer conn.Close()
	if c.Hits() != 0 || c.Misses() != 0 {
		t.Fatalf("literal IP touched cache: hits=%d misses=%d", c.Hits(), c.Misses())
	}
	select {
	case ac := <-accepted:
		ac.Close()
	case <-time.After(2 * time.Second):
		t.Fatalf("dial reached no listener")
	}
}
