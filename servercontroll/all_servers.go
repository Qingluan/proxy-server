package servercontroll

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
	"gitee.com/dark.H/ProxyZ/update"
	"gitee.com/dark.H/gs"
)

func basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Restricted Area\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		authParts := strings.SplitN(authHeader, " ", 2)
		if len(authParts) != 2 || authParts[0] != "Basic" {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Restricted Area\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(authParts[1])
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Restricted Area\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		credentials := strings.SplitN(string(decoded), ":", 2)
		if len(credentials) != 2 {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Restricted Area\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		validUser := "admin@example.com"
		validPass := fmt.Sprint(time.Now().Year()) + "@2pwd"

		if subtle.ConstantTimeCompare([]byte(credentials[0]), []byte(validUser)) != 1 ||
			subtle.ConstantTimeCompare([]byte(credentials[1]), []byte(validPass)) != 1 {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Restricted Area\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func setupHandler(www string) http.Handler {
	fmt.Println("Scan all listen ports...")
	mux := http.NewServeMux()

	base.CloseALLPortUFW()
	base.InitUsedPORT()

	go func() {
		for {
			time.Sleep(30 * time.Minute)
			if time.Now().Hour() == 0 {
				gs.Str("Start Refresh All Routes").Println()
				ids := gs.List[string]{}
				LockArea(func() {
					Tunnels.Every(func(no int, i *base.ProxyTunnel) {
						ids = append(ids, i.GetConfig().ID)
					})
				})

				ids.Every(func(no int, i string) {
					DelProxy(i)
				})
			}
		}
	}()
	if len(www) > 0 {
		mux.HandleFunc("/z-files", basicAuth(func(w http.ResponseWriter, r *http.Request) {
			fs := gs.List[any]{}
			gs.Str(www).Ls().Every(func(no int, i gs.Str) {
				isDir := i.IsDir()
				name := i.Basename()
				size := i.FileSize()
				fs = fs.Add(gs.Dict[any]{
					"name":  name,
					"isDir": isDir,
					"size":  size,
				})
			})
			Reply(w, fs, true)

		}))
		mux.Handle("/z-files-d/", http.StripPrefix("/z-files-d/", http.FileServer(http.Dir(www))))
		mux.HandleFunc("/z-files-u", uploadFileFunc(www))
	}
	mux.HandleFunc("/z-info", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		d, err := Recv(r.Body)
		if err != nil {
			w.WriteHeader(400)
			Reply(w, err, false)
		}
		if d == nil {
			Reply(w, "alive", true)
		}
	}))

	mux.HandleFunc("/z-set", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		d, err := Recv(r.Body)
		if err != nil {
			w.WriteHeader(400)
			Reply(w, err, false)
			return
		}

		if d == nil {
			Reply(w, "no val", false)
			return
		}
		if name, ok := d["name"]; ok {
			if val, ok := d["val"]; ok {
				gs.Str(val.(string)).Color("g", "B").Println(name.(string))

				Reply(w, "Good ", true)
				return

			}

		}
	}))

	mux.HandleFunc("/proxy-info", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		ids := []string{}
		proxy := gs.Dict[int]{
			"tls":  0,
			"quic": 0,
			"kcp":  0,
			"ss":   0,
		}
		aliveProxy := gs.Dict[int]{
			"tls":  0,
			"quic": 0,
			"kcp":  0,
			"ss":   0,
		}
		// New health metrics
		healthScores := gs.Dict[any]{}
		errorRates := gs.Dict[any]{}
		loadFactors := gs.Dict[any]{}

		clients := gs.List[string]{}
		st := time.Now()
		Tunnels.Every(func(no int, i *base.ProxyTunnel) {
			proxyType := i.GetConfig().ProxyType
			proxy[proxyType] += 1
			aliveProxy[proxyType] += i.GetClientNum()

			// Collect health metrics
			metrics := i.GetHealthMetrics()
			if metrics != nil {
				total, active, failed, _, avgLatency := metrics.GetStats()
				errorRate := metrics.GetErrorRate()
				loadFactor := metrics.GetLoadFactor()
				score := i.GetScore()

				tunnelInfo := gs.Dict[any]{
					"id":           i.GetConfig().ID,
					"type":         proxyType,
					"score":        score,
					"total":        total,
					"active":       active,
					"failed":       failed,
					"error_rate":   errorRate,
					"load_factor":  loadFactor,
					"avg_latency":  avgLatency.Milliseconds(),
					"max_conn":     metrics.GetMaxConnections(),
					"is_healthy":   i.IsHealthy(),
					"accepts_new":  i.AcceptsNewConnections(),
				}
				healthScores[i.GetConfig().ID] = tunnelInfo

				// Aggregate by protocol type
				if errorRates[proxyType] == nil {
					errorRates[proxyType] = 0.0
					loadFactors[proxyType] = 0.0
				}
				errorRates[proxyType] = errorRates[proxyType].(float64) + errorRate
				loadFactors[proxyType] = loadFactors[proxyType].(float64) + loadFactor
			}

			i.GetClientIP().Every(func(no int, ip string) {
				if !clients.In(ip) {
					clients = clients.Add(ip)
				}
			})
			ids = append(ids, i.GetConfig().ID)
		})

		// Calculate averages for protocol types
		for ptype := range proxy {
			if proxy[ptype] > 0 {
				count := float64(proxy[ptype])
				if errorRates[ptype] != nil {
					errorRates[ptype] = errorRates[ptype].(float64) / count
				}
				if loadFactors[ptype] != nil {
					loadFactors[ptype] = loadFactors[ptype].(float64) / count
				}
			}
		}

		used := time.Now().Sub(st)
		Reply(w, gs.Dict[any]{
			"ids":           ids,
			"alive":         aliveProxy,
			"client":        clients,
			"proxy":         proxy,
			"health_scores": healthScores,
			"error_rates":   errorRates,
			"load_factors":  loadFactors,
			"used":          used,
		}, true)
	}))

	mux.HandleFunc("/z-dns", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		d, err := Recv(r.Body)
		if err != nil {
			Reply(w, err, false)
			return
		}
		// gs.Str("DNS ???").Println("Query DNS")

		if hostsStr, ok := d["hosts"]; ok {
			res := gs.Dict[any]{}
			for _, host := range gs.Str(hostsStr.(string)).Split(",") {
				gs.Str(host).Println("Query DNS")
				if ips, err := net.LookupIP(host.Str()); err == nil {
					for _, _ip := range ips {
						if _ip.To4() != nil {
							res[_ip.String()] = host
						}

					}
				}
			}
			Reply(w, res, true)

		} else {
			Reply(w, "no dns", false)
		}
	}))

	mux.HandleFunc("/z-ufw-close-all", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		base.CloseALLPortUFW()
		Reply(w, base.GetUFW(), true)

	}))

	mux.HandleFunc("/z-ufw", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		d, err := Recv(r.Body)
		if err != nil {
			Reply(w, err, false)
			return
		}
		if d == nil {
			Reply(w, base.GetUFW(), true)
			return
		}
		if port, ok := d["port"]; ok && port != nil {
			switch port.(type) {
			case int:
				if op, ok := d["op"]; ok && op != nil {
					if op == "" {
						base.OpenPortUFW(port.(int))
					}
				} else {
					base.ClosePortUFW(port.(int))
				}

			case string:
				t := gs.Str(port.(string))
				if t.In("\n") {
					t.Split("\n").Every(func(no int, i gs.Str) {
						pi, err := strconv.Atoi(i.Trim().Str())
						if err != nil {
						} else {
							base.ClosePortUFW(pi)
						}
					})
				} else if t.In(",") {
					t.Split(",").Every(func(no int, i gs.Str) {
						pi, err := strconv.Atoi(i.Trim().Str())
						if err != nil {
						} else {
							base.ClosePortUFW(pi)
						}
					})
				} else {

				}
				i, err := strconv.Atoi(port.(string))
				if err != nil {
				} else {
					base.ClosePortUFW(i)
				}
			}
		}
		Reply(w, base.GetUFW(), true)
	}))

	mux.HandleFunc("/proxy-get", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		d, err := Recv(r.Body)
		if err != nil {
			Reply(w, err, false)
			return
		}
		if d == nil {
			// st := time.Now()
			tu := GetProxy()
			// fmt.Println("use new proxy :", time.Now().Sub(st))
			if !tu.On {
				// fmt.Println("no proxy start , to start  :", time.Now().Sub(st))
				afterID := tu.GetConfig().ID
				// gs.Str("start id" + afterID).Println()
				err := tu.Start(func() {
					DelProxy(afterID)
				})
				if err != nil {
					Reply(w, err, false)
					return
				}
				// fmt.Println("start ok   :", time.Now().Sub(st))
			}
			str := tu.GetConfig()
			// fmt.Println("Init ok   :", time.Now().Sub(st))
			// gs.Str(str.ProxyType + " in port %d").F(str.ServerPort).Color("g").Println("get")
			Reply(w, str, true)
			// fmt.Println("Fin   :", time.Now().Sub(st))
		} else {
			if proxyType, ok := d["type"]; ok {
				switch proxyType.(type) {
				case string:
					tu := GetProxy(proxyType.(string))
					if !tu.On {
						afterID := tu.GetConfig().ID
						err := tu.Start(func() {
							DelProxy(afterID)
						})
						if err != nil {
							Reply(w, err, false)
							return
						}
					}
					str := tu.GetConfig()
					gs.Str(str.ProxyType + " in port %d").F(str.ServerPort).Color("g").Println("get")
					Reply(w, str, true)
				default:
					tu := GetProxy()
					if !tu.On {
						afterID := tu.GetConfig().ID
						err := tu.Start(func() {
							DelProxy(afterID)
						})
						if err != nil {
							Reply(w, err, false)
							return
						}
					}
					str := tu.GetConfig()
					gs.Str(str.ProxyType + " in port %d").F(str.ServerPort).Color("g").Println("get")
					Reply(w, str, true)
				}
			} else {
				tu := GetProxy()
				if !tu.On {
					afterID := tu.GetConfig().ID
					err := tu.Start(func() {
						DelProxy(afterID)
					})
					if err != nil {
						Reply(w, err, false)
						return
					}
				}
				str := tu.GetConfig()
				gs.Str(str.ProxyType + " in port %d").F(str.ServerPort).Color("g").Println("get")
				Reply(w, str, true)
			}
		}
	}))

	mux.HandleFunc("/z-log", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		if gs.Str("/tmp/z.log").IsExists() {
			fp, err := os.Open("/tmp/z.log")
			if err != nil {
				w.Write([]byte(err.Error()))
			} else {
				defer fp.Close()
				io.Copy(w, fp)
			}
		} else {
			w.Write(gs.Str("/tmp/z.log not exists !!!").Bytes())
		}
	}))

	mux.HandleFunc("/c-log", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		if gs.Str("/tmp/z.log").IsExists() {
			defer gs.Str("").ToFile("/tmp/z.log", gs.O_NEW_WRITE)
			fp, err := os.Open("/tmp/z.log")
			if err != nil {
				w.Write([]byte(err.Error()))
			} else {
				defer fp.Close()
				io.Copy(w, fp)
			}
		} else {
			w.Write(gs.Str("/tmp/z.log not exists !!!").Bytes())
		}
	}))

	mux.HandleFunc("/__close-all", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		ids := gs.List[string]{}
		LockArea(func() {
			Tunnels.Every(func(no int, i *base.ProxyTunnel) {
				ids = append(ids, i.GetConfig().ID)
			})
		})

		ids.Every(func(no int, i string) {
			DelProxy(i)
		})
		Reply(w, gs.Dict[gs.List[string]]{
			"ids": ids,
		}, true)
	}))

	mux.HandleFunc("/z11-update", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		ids := gs.List[string]{}
		Tunnels.Every(func(no int, i *base.ProxyTunnel) {
			ids = append(ids, i.GetConfig().ID)
		})

		ids.Every(func(no int, i string) {
			DelProxy(i)
		})
		go update.Update(func(info string, ok bool) {
			Reply(w, info, ok)
		})
		Reply(w, "updaing... wait 3 s", true)

		// os.Exit(0)
		// }
	}))

	mux.HandleFunc("/proxy-err", basicAuth(func(w http.ResponseWriter, r *http.Request) {

		d, err := Recv(r.Body)

		if err != nil {
			w.WriteHeader(400)
			Reply(w, err, false)
		}
		if id, ok := d["Host"]; ok && id != nil {
			switch id.(type) {
			case []any:
				hosts := id.([]any)
				wait := sync.WaitGroup{}
				errn := 0
				for _, h := range hosts {
					wait.Add(1)
					go func(hh string, w *sync.WaitGroup) {
						defer w.Done()
						if !CanReachHOST(hh) {
							errn += 1
						}
					}(h.(string), &wait)
				}
				wait.Wait()
				if errn == 0 {
					Reply(w, "status ok:"+gs.S(id), false)
					return
				}
			case string:
				if CanReachHOST(id.(string)) {
					Reply(w, "status ok:"+id.(string), false)
					return
				}
			}

		}
		if id, ok := d["ID"]; ok && id != nil {
			idstr := id.(string)
			gs.Str(idstr).Color("r").Println("proxy-err")
			DelProxy(idstr)
		}
		tu := NewProxyByErrCount()
		afterID := tu.GetConfig().ID
		err = tu.Start(func() {
			DelProxy(afterID)
		})
		if err != nil {
			Reply(w, err, false)
			return
		}
		c := tu.GetConfig()
		Reply(w, c, true)

	}))

	mux.HandleFunc("/proxy-new", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		tu := NewProxy("tls")

		str := tu.GetConfig()
		Reply(w, str, true)
	}))

	mux.HandleFunc("/proxy-del", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		d, err := Recv(r.Body)
		if err != nil {
			w.WriteHeader(400)
			Reply(w, err, false)
		}
		configName := d["msg"].(string)

		str := DelProxy(configName)
		Reply(w, str, true)
	}))
	return mux
}
