package prokcp

import (
	"crypto/sha1"
	"errors"
	"net"
	"sync"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
	"gitee.com/dark.H/gs"
	"github.com/xtaci/kcp-go"
	"github.com/xtaci/smux"
	"golang.org/x/crypto/pbkdf2"
	// "github.com/cs8425/smux"
)

const (
	idType  = 0 // address type index
	idIP0   = 1 // ip address start index
	idDmLen = 1 // domain address length index
	idDm0   = 2 // domain address start index

	typeIPv4     = 1 // type is ipv4 address
	typeDm       = 3 // type is domain address
	typeIPv6     = 4 // type is ipv6 address
	typeRedirect = 9

	lenIPv4        = net.IPv4len + 2 // ipv4 + 2port
	lenIPv6        = net.IPv6len + 2 // ipv6 + 2port
	lenDmBase      = 2               // 1addrLen + 2port, plus addrLen
	AddrMask  byte = 0xf
	// lenHmacSha1 = 10
)

var (
	debug                 bool
	sanitizeIps           bool
	udp                   bool
	managerAddr           string
	smuxConfig            = smux.DefaultConfig()
	Socks5ConnectedRemote = []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x08, 0x43}
)

type Channel struct {
	stream net.Conn
	host   string
}

func newChannel(stream net.Conn, host string) Channel {
	return Channel{
		stream: stream,
		host:   host,
	}
}

// KcpServer used for server
type KcpServer struct {
	config *base.ProtocolConfig
	// RedirectMode  bool
	// TunnelChan     chan Channel
	// TcpListenPorts map[string]int
	AcceptConn int
	ZeroToDel  bool
	ips        gs.Dict[bool]
	listener   net.Listener
	lock       sync.RWMutex
	// RedirectBook  *utils.Config
}

// acceptTuned accepts from the KCP listener and tunes the session on the way in.
func acceptTuned(l net.Listener) (net.Conn, error) {
	con, err := l.Accept()
	if err != nil {
		return nil, err
	}
	if sess, ok := con.(*kcp.UDPSession); ok {
		tuneKCPSession(sess)
	}
	return con, nil
}

// tuneKCPSession mirrors the client's fast4 KCP params: immediate mode,
// 10ms interval, fast resend, congestion control off, no delayed ACK,
// 512/512 windows. kcp-go defaults (normal mode, 40ms, delayed ACK) are
// the server/client asymmetry that added latency on every round trip.
func tuneKCPSession(sess *kcp.UDPSession) {
	sess.SetNoDelay(1, 10, 2, 1)
	sess.SetACKNoDelay(true)
	sess.SetWindowSize(512, 512)
}

func (ksever *KcpServer) Accept() (con net.Conn, err error) {
	listener := ksever.GetListener()
	if listener == nil {
		return nil, errors.New("get listener err! in kcp")
	}
	con, err = acceptTuned(listener)
	if err != nil {
		return
	}
	// KeepAlive := 10
	// ScavengeTTL := 600
	// AutoExpire := 7
	// SmuxBuf := 4194304 * 2
	// StreamBuf := 2097152 * 2
	ksever.AcceptConn += 1
	return
}

func (kserver *KcpServer) DelCon(con net.Conn) {
	con.Close()
	kserver.DelRecord(con)
	kserver.AcceptConn -= 1
}

func (ksever *KcpServer) GetListener() net.Listener {
	_key := ksever.config.Password
	_salt := ksever.config.SALT

	key := pbkdf2.Key([]byte(_key), []byte(_salt), 4096, 32, sha1.New)
	block, _ := kcp.NewAESBlockCrypt(key)
	// var listener net.Listener
	serverAddr := gs.Str("%s:%d").F(ksever.config.Server, ksever.config.ServerPort)

	DataShard := 10
	ParityShard := 3
	addr := serverAddr.Str()
	gs.Str(addr).Println("listen kcp")
	if listener, err := kcp.ListenWithOptions(addr, block, DataShard, ParityShard); err == nil {
		// 4MB UDP socket buffers to match the client's fast4 mode; on Linux
		// net.core.rmem_max must be >= 4MB or the kernel silently clamps.
		_ = listener.SetReadBuffer(4 * 1024 * 1024)
		_ = listener.SetWriteBuffer(4 * 1024 * 1024)
		return listener
	} else {
		return nil
	}

}

func (kserver *KcpServer) GetConfig() *base.ProtocolConfig {
	return kserver.config
}

func NewKcpServer(config *base.ProtocolConfig) *KcpServer {
	k := new(KcpServer)
	config.ProxyType = "kcp"
	config.Type = "fast"
	k.config = config

	return k
}

func (kcpServer *KcpServer) AcceptHandle(waitTime time.Duration, handle func(con net.Conn) error) (err error) {
	listener := kcpServer.GetListener()
	if listener == nil {
		return errors.New("listener is closed")
	}
	kcpServer.lock.Lock()
	closedEarly := kcpServer.ZeroToDel
	kcpServer.listener = listener
	kcpServer.lock.Unlock()
	if closedEarly {
		listener.Close()
		return nil
	}
	defer listener.Close()
	for {
		con, err := acceptTuned(listener)
		if err != nil {
			return err
		}
		kcpServer.Record(con.RemoteAddr())
		go handle(con)
	}
}

func (kcpServer *KcpServer) TryClose() {
	kcpServer.lock.Lock()
	kcpServer.ZeroToDel = true
	listener := kcpServer.listener
	kcpServer.lock.Unlock()
	if listener != nil {
		listener.Close()
	}
}

func (kcpserver *KcpServer) Record(con net.Addr) {
	ip := con.String()
	kcpserver.lock.Lock()
	if kcpserver.ips == nil {
		kcpserver.ips = make(gs.Dict[bool])
	}
	kcpserver.ips[ip] = true
	kcpserver.lock.Unlock()
}

func (kcpserver *KcpServer) DelRecord(con net.Conn) {
	ip := con.RemoteAddr().String()
	kcpserver.lock.Lock()
	if kcpserver.ips == nil {
		kcpserver.ips = make(gs.Dict[bool])
	}
	if _, ok := kcpserver.ips[ip]; ok {
		delete(kcpserver.ips, ip)
	}
	kcpserver.lock.Unlock()

}

func (kcpserver *KcpServer) GetAliveIPS() gs.List[string] {
	ds := gs.List[string]{}
	kcpserver.lock.RLock()
	for k := range kcpserver.ips {
		ds = append(ds, k)
	}
	kcpserver.lock.RUnlock()
	return ds
}
