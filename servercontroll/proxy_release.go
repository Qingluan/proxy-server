package servercontroll

import (
	"sync"

	"gitee.com/dark.H/ProxyZ/connections/base"
	"gitee.com/dark.H/ProxyZ/connections/prokcp"
	"gitee.com/dark.H/ProxyZ/connections/proquic"
	"gitee.com/dark.H/ProxyZ/connections/protls"
	"gitee.com/dark.H/gs"
)

var (
	lock         = sync.RWMutex{}
	ErrTypeCount = gs.Dict[int]{
		"tls":  0,
		"kcp":  0,
		"quic": 0,
	}
	lastUse = 0
)

func LockArea(a func()) {
	lock.Lock()
	a()
	lock.Unlock()
	return

}

func GetProxy(proxyType ...string) *base.ProxyTunnel {
	if proxyType == nil {
		c := -1
		LockArea(func() {
			c = Tunnels.Count()
		})

		if c == 0 {
			// st := time.Now()
			// fmt.Println("reading proxy : ", time.Since(st))
			tunnel := NewProxy("quic")
			// fmt.Println("create quic proxy : ", time.Since(st))
			AddProxy(tunnel)
			// fmt.Println("create add proxy : ", time.Since(st))
			return tunnel
		} else {
			LockArea(func() {
				lastUse = lastUse % Tunnels.Count()
				lastUse += 1
				lastUse = lastUse % Tunnels.Count()
			})
			// nts := Tunnels.Sort(func(l, r *base.ProxyTunnel) bool {
			// 	return l.GetClientNum() < r.GetClientNum()
			// })
			// tunnel := nts.Nth(0)
			// st := time.Now()
			// fmt.Println("create other proxy : ", time.Since(st))
			var tunnel *base.ProxyTunnel
			for i := 0; i < 4; i++ {
				var otunnel *base.ProxyTunnel
				LockArea(func() {
					otunnel = Tunnels.Nth(lastUse)
					lastUse += 1
					lastUse = lastUse % Tunnels.Count()
					tunnel = Tunnels.Nth(lastUse)

				})

				if tunnel.GetConfig().ProxyType == otunnel.GetConfig().ProxyType || tunnel.GetClientNum() > otunnel.GetClientNum() {
					continue
				} else {
					return tunnel
				}
			}
			return tunnel

		}
	} else {
		var tu *base.ProxyTunnel
		nts := Tunnels.Sort(func(l, r *base.ProxyTunnel) bool {
			return l.GetClientNum() < r.GetClientNum()
		})
		nts.Every(func(no int, i *base.ProxyTunnel) {
			if i.GetConfig().ProxyType == proxyType[0] {
				tu = i
			}
		})
		if tu == nil {
			tunnel := NewProxy(proxyType[0])
			AddProxy(tunnel)
			return tunnel
		} else {
			return tu
		}
	}
}

func AddProxy(c *base.ProxyTunnel) {
	LockArea(func() {
		Tunnels = append(Tunnels, c)
	})

}

func DelProxy(name string) (found bool) {

	e := gs.List[*base.ProxyTunnel]{}
	for _, tun := range Tunnels {
		if tun == nil {
			continue
		}
		if tun.GetConfig().ID == name {

			if num, ok := ErrTypeCount[tun.GetConfig().ProxyType]; ok {
				num += 1
				LockArea(func() {
					ErrTypeCount[tun.GetConfig().ProxyType] = num
				})
			}
			tun.SetWaitToClose()
			base.ClosePortUFW(tun.GetConfig().ServerPort)
			found = true
			continue
		} else {
			e = e.Add(tun)
		}
	}
	LockArea(func() {
		Tunnels = e
	})

	return
}

func NewProxyByErrCount() (c *base.ProxyTunnel) {
	tlsnum := ErrTypeCount["tls"]
	kcpnum := ErrTypeCount["kcp"]
	quicum := ErrTypeCount["quic"]
	if quicum < tlsnum {
		c = NewProxy("quic")
	} else {
		if kcpnum < tlsnum {

			c = NewProxy("kcp")
		} else {
			c = NewProxy("tls")
		}

	}

	AddProxy(c)
	return
}

func GetProxyByID(name string) (c *base.ProxyTunnel) {
	for _, tun := range Tunnels {
		if tun.GetConfig().ID == name {
			return tun
		} else {

		}
	}
	return
}

func NewProxy(tp string) *base.ProxyTunnel {
	// st := time.Now()
	switch tp {
	case "tls":
		// fmt.Println("tls before config :", time.Since(st))
		config := base.RandomConfig()
		// fmt.Println("tls  config :", time.Since(st))
		protocl := protls.NewTlsServer(config)
		// fmt.Println("tls server :", time.Since(st))
		tunel := base.NewProxyTunnel(protocl)
		// fmt.Println("tls tunnel :", time.Since(st))
		return tunel
	case "kcp":
		config := base.RandomConfig()
		protocl := prokcp.NewKcpServer(config)
		tunel := base.NewProxyTunnel(protocl)
		return tunel
	case "quic":
		// fmt.Println("quic before config :", time.Since(st))
		config := base.RandomConfig()
		// fmt.Println("quic config :", time.Since(st))
		protocl := proquic.NewQuicServer(config)
		// fmt.Println("quic new server :", time.Since(st))
		tunel := base.NewProxyTunnel(protocl)
		// fmt.Println("quic new tunnel :", time.Since(st))
		return tunel
	default:
		// fmt.Println("tls before config :", time.Since(st))

		config := base.RandomConfig()
		// fmt.Println("tls  config :", time.Since(st))
		protocl := protls.NewTlsServer(config)
		// fmt.Println("tls server :", time.Since(st))
		tunel := base.NewProxyTunnel(protocl)
		// fmt.Println("tls tunnel :", time.Since(st))
		return tunel
	}
}
