package base

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/debuglog"
	"gitee.com/dark.H/ProxyZ/connections/prodns"
)

const (
	// dnsCacheTTL bounds how long a positive resolution is trusted (G13).
	dnsCacheTTL = 60 * time.Second
	// dnsCacheMaxEntries bounds cache memory; beyond it the oldest-inserted
	// entries are evicted (FIFO).
	dnsCacheMaxEntries = 1024
)

type dnsEntry struct {
	ips []net.IP
	exp time.Time
	// seq is the insertion order stamp used for FIFO eviction. A TTL
	// refresh re-stamps the entry, so recently re-resolved hosts survive.
	seq uint64
}

// inflight carries one in-progress resolution so concurrent misses for the
// same host share a single underlying resolve call (single-flight).
type inflight struct {
	done chan struct{}
	ips  []net.IP
	err  error
}

// DNSCache is a small TTL cache in front of the system resolver. The server
// dials upstream targets by hostname on every stream and answers proxied DNS
// queries per request, which meant one system resolution per request for the
// same handful of hosts; this cache removes that repetition.
//
// Positive results are cached for ttl; errors and empty results are never
// cached. Insertions beyond max evict the oldest entry. Concurrent misses on
// the same host block on a single shared resolution.
type DNSCache struct {
	mu       sync.Mutex
	entries  map[string]*dnsEntry
	inflight map[string]*inflight
	nextSeq  uint64

	// ttl and max are fields so tests can shorten/shrink them.
	ttl time.Duration
	max int

	// resolve is injectable for tests; production uses net.LookupIP
	// (returns IPv4 and IPv6 — callers filter as they always have).
	resolve func(host string) ([]net.IP, error)

	// now is a clock seam for tests.
	now func() time.Time

	hits   atomic.Int64
	misses atomic.Int64
}

// NewDNSCache returns a cache with the fixed production parameters
// (TTL 60s, 1024 entries, net.LookupIP).
func NewDNSCache() *DNSCache {
	return &DNSCache{
		entries:  make(map[string]*dnsEntry),
		inflight: make(map[string]*inflight),
		ttl:      dnsCacheTTL,
		max:      dnsCacheMaxEntries,
		resolve:  net.LookupIP,
		now:      time.Now,
	}
}

// Lookup returns the cached IPs for host, resolving on a miss. Concurrent
// misses for the same host share one resolution. The returned slice is
// shared with the cache and must not be modified by callers.
func (c *DNSCache) Lookup(host string) ([]net.IP, error) {
	c.mu.Lock()
	if e, ok := c.entries[host]; ok && c.now().Before(e.exp) {
		c.hits.Add(1)
		ips := e.ips
		c.mu.Unlock()
		return ips, nil
	}
	c.misses.Add(1)
	if fl, ok := c.inflight[host]; ok {
		c.mu.Unlock()
		<-fl.done
		return fl.ips, fl.err
	}
	fl := &inflight{done: make(chan struct{})}
	c.inflight[host] = fl
	c.mu.Unlock()

	fl.ips, fl.err = c.resolve(host)

	c.mu.Lock()
	delete(c.inflight, host)
	if fl.err == nil && len(fl.ips) > 0 {
		c.nextSeq++
		c.entries[host] = &dnsEntry{
			ips: fl.ips,
			exp: c.now().Add(c.ttl),
			seq: c.nextSeq,
		}
		c.evictLocked()
	}
	c.mu.Unlock()
	close(fl.done)
	return fl.ips, fl.err
}

// evictLocked drops the oldest-inserted entries until the cache fits.
// The linear scan only runs once the cache is full (rare: 1024+ distinct
// hosts between evictions), and is negligible next to a DNS round trip.
func (c *DNSCache) evictLocked() {
	for len(c.entries) > c.max {
		oldestKey := ""
		oldestSeq := uint64(0)
		first := true
		for k, e := range c.entries {
			if first || e.seq < oldestSeq {
				oldestKey, oldestSeq, first = k, e.seq, false
			}
		}
		delete(c.entries, oldestKey)
	}
}

// Len reports the number of entries currently cached.
func (c *DNSCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Hits reports how many lookups were served from cache.
func (c *DNSCache) Hits() int64 { return c.hits.Load() }

// Misses reports how many lookups had to (re)resolve.
func (c *DNSCache) Misses() int64 { return c.misses.Load() }

// SharedDNSCache is the process-wide cache used for upstream dials
// (TcpNormal) and proxied DNS answers (prodns.ReplyDNS).
var SharedDNSCache = NewDNSCache()

func init() {
	// prodns cannot import base (base imports prodns), so the shared cache
	// is injected into prodns from here: ReplyDNS then resolves through the
	// same cache as the dial path. Wiring at init makes it unconditional for
	// every binary that links this package.
	prodns.SetLookup(SharedDNSCache.Lookup)
}

// dialCached resolves hostport through the DNS cache and dials the resolved
// IP(s), preserving the original port. The tunnel relay (Pipe) is a pure
// byte passthrough — the server never terminates TLS or sends SNI (TLS is
// end-to-end client<->target), so dialing by IP instead of hostname is
// safe. Literal IPs and anything the cache cannot resolve fall back to a
// plain hostname dial, preserving the pre-cache error behavior.
func dialCached(cache *DNSCache, dialer *net.Dialer, hostport string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || net.ParseIP(host) != nil {
		// Not host:port, or already a literal IP: nothing to cache.
		return dialer.Dial("tcp", hostport)
	}
	ips, rerr := cache.Lookup(host)
	if rerr != nil || len(ips) == 0 {
		debuglog.Write("[dns] resolve fail host=%s err=%v fallback=direct", host, rerr)
		return dialer.Dial("tcp", hostport)
	}
	var lastErr error
	for _, ip := range ips {
		conn, derr := dialer.Dial("tcp", net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	// Every resolved IP failed: fall back to the hostname dial so the error
	// surfaced (and logged) matches the original behavior.
	debuglog.Write("[dns] all resolved IPs failed host=%s lastErr=%v fallback=direct", host, lastErr)
	return dialer.Dial("tcp", hostport)
}
