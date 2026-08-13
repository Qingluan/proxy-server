package proquic

import (
	"gitee.com/dark.H/ProxyZ/connections/proquic/bbr"
	"github.com/apernet/quic-go"
)

// SetBBR switches a QUIC connection to the BBR congestion controller, which
// probes bandwidth instead of reacting to loss like the default Cubic. It is
// used on high-packet-loss paths where Cubic collapses throughput.
func SetBBR(conn *quic.Conn) {
	if conn == nil {
		return
	}
	size := conn.InitialPacketSize()
	if byAddr := bbr.GetInitialPacketSize(conn.RemoteAddr()); size <= 0 || byAddr < size {
		size = byAddr
	}
	conn.SetCongestionControl(bbr.NewBbrSender(bbr.DefaultClock{}, size, bbr.ProfileStandard))
}
