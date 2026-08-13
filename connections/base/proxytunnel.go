package base

import (
	"errors"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/debuglog"
	"gitee.com/dark.H/ProxyZ/connections/prodns"
	"gitee.com/dark.H/ProxyZ/connections/prosmux"
	"gitee.com/dark.H/ProxyZ/connections/prosocks5"
	"gitee.com/dark.H/gs"
)

type Protocol interface {
	GetListener() net.Listener
	GetConfig() *ProtocolConfig
	AcceptHandle(waitTime time.Duration, handle func(con net.Conn) error) (err error)
	TryClose()
	GetAliveIPS() gs.List[string]
	DelCon(con net.Conn)
}

const (
	// DefaultMaxConnections is the default maximum concurrent connections per tunnel
	DefaultMaxConnections = 1000
	// MaxConnectionsPerTunnel is the absolute maximum to prevent resource exhaustion
	MaxConnectionsPerTunnel = 5000
)

type ProxyTunnel struct {
	cons         gs.List[net.Conn]
	alive        int
	lock         sync.RWMutex
	protocl      Protocol
	UseSmux      bool
	On           bool
	ZeroToDel    bool
	ControllFunc func(rawHost string, con net.Conn) (err error)
	metrics      *HealthMetrics
	maxConn      int32
}

func NewProxyTunnel(procol Protocol) *ProxyTunnel {
	p := new(ProxyTunnel)
	p.protocl = procol
	p.UseSmux = true
	p.maxConn = DefaultMaxConnections
	p.metrics = NewHealthMetrics(p.maxConn)

	return p
}
func (pt *ProxyTunnel) Start(after func()) (err error) {
	pt.On = true
	go func() {
		err := pt.Server(after)
		if err != nil {
			gs.Str("Start proxy err:" + err.Error()).Color("r").Println("service")
		}
	}()
	return
}

func (pt *ProxyTunnel) Server(after func()) (err error) {
	serverPort := pt.GetConfig().ServerPort
	defer func() {
		pt.On = false
		ClosePortUFW(serverPort)
		ReleasePort(serverPort)
		after()
	}()

	if pt.protocl == nil {
		return errors.New("no protocol set in ProxyTunnel")
	}
	gs.Str("%s in %d id: %s").F(pt.GetConfig().ProxyType, serverPort, pt.GetConfig().ID).Println("service")
	debuglog.Write("[tunnel] start type=%s port=%d id=%s", pt.GetConfig().ProxyType, serverPort, pt.GetConfig().ID)

	defer func() {
		debuglog.Write("[tunnel] stop id=%s port=%d", pt.GetConfig().ID, serverPort)
	}()

	if pt.GetConfig().ProxyType == "quic" {
		// gs.Str(pt.GetConfig().ID + "|" + pt.GetConfig().ProxyType + "| addr:" + pt.GetConfig().RemoteAddr()).Println("Start Quic Server ")
		pt.protocl.AcceptHandle(1*time.Minute, func(con net.Conn) error {
			pt.HandleConnAsync(con)
			return nil
		})

	} else if pt.UseSmux {
		// gs.Str(pt.GetConfig().ID + "|" + pt.GetConfig().ProxyType + "| addr:" + pt.GetConfig().RemoteAddr()).Println("Start Smux Tunnel")
		smux := prosmux.NewSmuxServer(pt.protocl, func(con net.Conn) (err error) {
			pt.HandleConnAsync(con)
			return
		})
		return smux.Server()

	} else {
		// gs.Str(pt.GetConfig().ID + "|" + pt.GetConfig().ProxyType + "| addr:" + pt.GetConfig().RemoteAddr()).Println("Start Tunnel")
		pt.protocl.AcceptHandle(1*time.Minute, func(con net.Conn) error {
			pt.HandleConnAsync(con)
			return nil
		})

	}

	return
}

func (pt *ProxyTunnel) SetWaitToClose() {
	pt.protocl.TryClose()

}

func (pt *ProxyTunnel) SetProtocol(procol Protocol) {
	pt.protocl = procol

}

func (pt *ProxyTunnel) GetConfig() *ProtocolConfig {
	if pt.protocl == nil {
		return nil
	}
	return pt.protocl.GetConfig()
}

func (pt *ProxyTunnel) DelCon(con net.Conn) {
	pt.protocl.DelCon(con)
}

func (pt *ProxyTunnel) SetControllFunc(l func(rawHost string, con net.Conn) (err error)) {
	pt.ControllFunc = l
}

func (pt *ProxyTunnel) HandleConnAsync(con net.Conn) {
	// Global stream budget first: this is the primary defence against FD and
	// memory exhaustion across all tunnels. Per-tunnel limits cannot stop a
	// single client from exhausting the whole process.
	if !TryAcquireStream() {
		debuglog.Write("[limit] global stream cap reached active=%d", GlobalActiveStreams())
		con.Close()
		pt.DelCon(con)
		return
	}
	defer ReleaseStream()

	// Check connection limit before processing
	if pt.metrics != nil && !pt.metrics.RecordConnection() {
		gs.Str("Connection limit reached, rejecting new connection").Color("y").Println("limit")
		debuglog.Write("[limit] reject conn max=%d", pt.maxConn)
		con.Close()
		pt.DelCon(con)
		return
	}

	// Track start time for latency measurement
	startTime := time.Now()

	// Generous first-request deadline. The client pre-produces idle streams
	// into a fastTunnels pool and only writes the SOCKS5 CONNECT when a
	// request consumes the stream, so a short deadline kills healthy idle
	// streams and triggers client-side "closed pipe" storms. Resource
	// exhaustion is already bounded by the global stream cap and per-tunnel
	// connection limit above, so this deadline only guards streams that never
	// carry a request.
	con.SetReadDeadline(time.Now().Add(10 * time.Minute))
	host, _, _, err := prosocks5.GetServerRequest(con)
	if err != nil {
		// gs.Str(err.Error()).Println("GetServerRequest | err")
		ErrToFile("Server HandleConnection", err)
		if debuglog.Allow("handshake", 100, 100) {
			debuglog.Write("[handshake] err=%v", err)
		}
		if pt.metrics != nil {
			pt.metrics.RecordFailure()
		}
		con.Close()
		pt.DelCon(con)
		if pt.metrics != nil {
			pt.metrics.ReleaseConnection()
		}
		return
	}
	// Clear the handshake deadline after the request is parsed.
	con.SetReadDeadline(time.Time{})

	pt.lock.Lock()
	pt.cons = pt.cons.Add(con)
	pt.alive += 1
	pt.lock.Unlock()
	defer func() {
		pt.lock.Lock()
		pt.alive -= 1
		pt.lock.Unlock()
		// Record latency and release connection
		if pt.metrics != nil {
			pt.metrics.RecordLatency(time.Since(startTime))
			pt.metrics.ReleaseConnection()
		}
	}()
	if gs.Str(host).StartsWith("R://") {
		if pt.ControllFunc != nil {
			if err := pt.ControllFunc(host, con); err != nil {
				ErrToFile("server controll func ", err)
			}
		}
	} else if gs.Str(host).StartsWith("dns://") || gs.Str(host).StartsWith("[dns://") {
		host = string(gs.Str(host).Split("[")[1].Split("]")[0])
		pt.DnsNormal(host, con)
	} else {
		pt.TcpNormal(host, con)
	}
}

func (pt *ProxyTunnel) GetClientNum() int {
	return pt.alive
}

func (pt *ProxyTunnel) GetClientIP() gs.List[string] {
	return pt.protocl.GetAliveIPS()
}

func (pt *ProxyTunnel) DnsNormal(host string, con net.Conn) (err error) {
	defer pt.DelCon(con)

	// c.SingleInflight = true
	// laddr := net.UDPAddr{
	// 	IP:   net.ParseIP("[::1]"),
	// 	Port: 12345,
	// 	Zone: "",
	// }
	// c.Dialer = &net.Dialer{
	// 	Timeout:   1 * time.Second,
	// 	LocalAddr: &laddr,
	// }
	gs.Str(host).Println("query")
	if msg, err := prodns.ReplyDNS(gs.Str(host)); err != nil || msg == "" {
		gs.Str(err.Error()).Println("DNS")
		return err
	} else {

		con.Write(msg.Bytes())
		if m, err := prodns.UnpackDNS(msg); err == nil && m != nil {
			if len(m.Question) > 0 {
				if len(m.Answer) > 0 {
					gs.Str("%s -> %s ").F(gs.Str(m.Question[0].Name).Color("y"), gs.Str(m.Answer[0].String()).Color("g")).Println("dns")
				}
			}
		}

	}
	return
}

func (pt *ProxyTunnel) TcpNormal(host string, con net.Conn) (err error) {
	defer pt.DelCon(con)

	// Reply with the SOCKS5 confirmation IMMEDIATELY, before dialing the
	// target. The client waits on this confirm for up to 2s; dialing first
	// made every request stall by that amount whenever the target dial was
	// slow or the session congested. On dial failure we simply close: the
	// client already consumed the confirm, so no failure reply can be sent
	// safely at that point.
	con.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, werr := con.Write(prosocks5.Socks5Confirm); werr != nil {
		ErrToFile("back con is break", werr)
		debuglog.Write("[dial] confirm-write fail host=%s err=%v", host, werr)
		con.Close()
		return werr
	}
	con.SetWriteDeadline(time.Time{})

	dialStart := time.Now()
	// Bounded dial: a black-holed target must not pin a stream goroutine forever.
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	remoteConn, err := dialer.Dial("tcp", host)
	dialElapsed := time.Since(dialStart)
	if err != nil {
		if ne, ok := err.(*net.OpError); ok && (ne.Err == syscall.EMFILE || ne.Err == syscall.ENFILE) {
			// log too many open file error
			// EMFILE is process reaches open file limits, ENFILE is system limit
			ErrToFile("dial error too many file!!:", err)
		} else {
			ErrToFile("tcp normal", err)
		}
		gs.Str(host + "|" + err.Error()).Println("host|failed")
		debuglog.Write("[dial] fail host=%s err=%v elapsed=%s", host, err, dialElapsed)
		con.Close()
		return err
	}
	// A dial slower than 1s is worth flagging: it maps to the client-side
	// tunnel_setup stalls.
	if dialElapsed > time.Second {
		debuglog.Write("[dial] SLOW host=%s elapsed=%s", host, dialElapsed)
	}
	debuglog.Write("[req] host=%s dial=%s ok", host, dialElapsed)
	gs.Str(host).Println("host|build")
	pt.Pipe(remoteConn, con)
	return
}

const (
	idleTimeout    = 30 * time.Minute
	pipeBufferSize = 128 * 1024
)

// pipeBufPool recycles the 128KB copy buffers. Without pooling, every stream
// pinned 2 x 128KB for its whole lifetime (256MB at 1000 concurrent streams).
var pipeBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, pipeBufferSize)
		return &b
	},
}

func (pt *ProxyTunnel) Pipe(p1, p2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Idle-based copy: the deadline is refreshed on every read, so long-lived
	// transfers (videos, large downloads) are never truncated by a total
	// timeout; only a truly idle connection is closed.
	streamCopy := func(dst, src net.Conn) {
		defer dst.Close()
		defer src.Close()

		bufp := pipeBufPool.Get().(*[]byte)
		buf := *bufp
		defer pipeBufPool.Put(bufp)

		for {
			src.SetReadDeadline(time.Now().Add(idleTimeout))
			nr, err := src.Read(buf)
			if nr > 0 {
				if nw, ew := dst.Write(buf[:nr]); ew != nil || nw != nr {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}

	go func() {
		defer wg.Done()
		streamCopy(p1, p2)
	}()
	go func() {
		defer wg.Done()
		streamCopy(p2, p1)
	}()

	wg.Wait()
}

func (pt *ProxyTunnel) PipeReadWriteCloser(p1, p2 io.ReadWriteCloser) {
	var wg sync.WaitGroup
	wg.Add(2)

	streamCopy := func(dst, src io.ReadWriteCloser) {
		defer dst.Close()
		defer src.Close()

		bufp := pipeBufPool.Get().(*[]byte)
		buf := *bufp
		defer pipeBufPool.Put(bufp)

		for {
			if c, ok := src.(interface{ SetReadDeadline(time.Time) error }); ok {
				c.SetReadDeadline(time.Now().Add(idleTimeout))
			}
			nr, err := src.Read(buf)
			if nr > 0 {
				if nw, ew := dst.Write(buf[:nr]); ew != nil || nw != nr {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}

	go func() {
		defer wg.Done()
		streamCopy(p1, p2)
	}()
	go func() {
		defer wg.Done()
		streamCopy(p2, p1)
	}()

	wg.Wait()
}

// GetHealthMetrics returns the health metrics for this tunnel
func (pt *ProxyTunnel) GetHealthMetrics() *HealthMetrics {
	return pt.metrics
}

// GetScore returns the health score for this tunnel (0.0 to 1.0, higher is better)
func (pt *ProxyTunnel) GetScore() float64 {
	if pt.metrics == nil {
		return 0.5
	}
	return pt.metrics.CalculateScore()
}

// SetMaxConnections sets the maximum concurrent connections for this tunnel
func (pt *ProxyTunnel) SetMaxConnections(max int32) {
	pt.maxConn = max
	if pt.metrics != nil {
		pt.metrics.SetMaxConnections(max)
	}
}

// GetMaxConnections returns the maximum concurrent connections for this tunnel
func (pt *ProxyTunnel) GetMaxConnections() int32 {
	return pt.maxConn
}

// IsHealthy returns whether the tunnel is healthy and can accept connections
func (pt *ProxyTunnel) IsHealthy() bool {
	if pt.metrics == nil {
		return true
	}
	return pt.metrics.IsHealthy()
}

// AcceptsNewConnections returns whether the tunnel can accept new connections
func (pt *ProxyTunnel) AcceptsNewConnections() bool {
	if pt.metrics == nil {
		return true
	}
	return pt.metrics.AcceptsNewConnections()
}
