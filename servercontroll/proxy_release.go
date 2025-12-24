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
			// No tunnels exist, create a new quic tunnel (default)
			tunnel := NewProxy("quic")
			AddProxy(tunnel)
			return tunnel
		}

		// Use health-based scoring to select the best tunnel
		return selectBestTunnel()
	} else {
		// Specific proxy type requested
		return selectByType(proxyType[0])
	}
}

// selectBestTunnel selects the best tunnel using health score
func selectBestTunnel() *base.ProxyTunnel {
	var bestTunnel *base.ProxyTunnel
	var bestScore float64 = -1
	var bestType string = ""

	LockArea(func() {
		for _, tunnel := range Tunnels {
			if tunnel == nil || !tunnel.On {
				continue
			}

			// Skip tunnels that can't accept new connections
			if !tunnel.AcceptsNewConnections() {
				continue
			}

			score := tunnel.GetScore()
			tunnelType := tunnel.GetConfig().ProxyType

			// Prefer different protocol types for load balancing
			typeBonus := 0.0
			if tunnelType != bestType {
				typeBonus = 0.1
			}

			adjustedScore := score + typeBonus

			if adjustedScore > bestScore {
				bestScore = adjustedScore
				bestTunnel = tunnel
				bestType = tunnelType
			}
		}
	})

	// If no healthy tunnel found, create a new one
	if bestTunnel == nil {
		bestTunnel = NewProxy("quic")
		AddProxy(bestTunnel)
		gs.Str("No healthy tunnel found, created new quic tunnel").Color("y").Println("proxy")
	}

	return bestTunnel
}

// selectByType selects the best tunnel of a specific type
func selectByType(proxyType string) *base.ProxyTunnel {
	var bestTunnel *base.ProxyTunnel
	var bestScore float64 = -1

	LockArea(func() {
		for _, tunnel := range Tunnels {
			if tunnel == nil || !tunnel.On {
				continue
			}

			if tunnel.GetConfig().ProxyType == proxyType {
				score := tunnel.GetScore()
				if tunnel.AcceptsNewConnections() && score > bestScore {
					bestScore = score
					bestTunnel = tunnel
				}
			}
		}
	})

	// If no tunnel of this type exists, create one
	if bestTunnel == nil {
		bestTunnel = NewProxy(proxyType)
		AddProxy(bestTunnel)
		gs.Str("No tunnel of type %s found, created new one").F(proxyType).Color("y").Println("proxy")
	}

	return bestTunnel
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
	// Calculate average error rate for each protocol type
	protocolStats := gs.Dict[struct {
		totalConns   int64
		failedConns int64
		count       int
	}]{
		"tls":  {},
		"kcp":  {},
		"quic": {},
	}

	LockArea(func() {
		for _, tunnel := range Tunnels {
			if tunnel == nil {
				continue
			}
			proxyType := tunnel.GetConfig().ProxyType
			metrics := tunnel.GetHealthMetrics()
			if metrics != nil {
				total, _, failed, _, _ := metrics.GetStats()
				stats := protocolStats[proxyType]
				stats.totalConns += total
				stats.failedConns += failed
				stats.count += 1
				protocolStats[proxyType] = stats
			}
		}
	})

	// Select protocol with lowest error rate
	bestType := "quic"
	lowestErrorRate := 1.0

	for ptype, stats := range protocolStats {
		if stats.count == 0 {
			bestType = ptype
			break
		}
		errorRate := float64(stats.failedConns) / float64(stats.totalConns)
		if errorRate < lowestErrorRate {
			lowestErrorRate = errorRate
			bestType = ptype
		}
	}

	c = NewProxy(bestType)
	AddProxy(c)
	gs.Str("Created new %s tunnel (error-based selection)").F(bestType).Color("g").Println("proxy")
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
