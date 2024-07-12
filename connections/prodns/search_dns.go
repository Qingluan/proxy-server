package prodns

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"gitee.com/dark.H/gs"
)

var (
	PAC_HOSTS      = make(map[string]bool)
	DYNAMIC_DIRECT = make(map[string]bool)
	lock           = sync.RWMutex{}
	paclock        = sync.RWMutex{}
)

func LoadLocalRule(path string) {
	if e := gs.Str(path); e.IsExists() {
		e.MustAsFile().EveryLine(func(lineno int, line gs.Str) {
			if line.Trim().StartsWith("#") {
				return
			}
			if line.Trim() == "" {
				return
			}
			if line.Trim().In(" ") {
				return
			}
			if line.In("*") {
				fuzzyHost = fuzzyHost.Add(line.Trim().Str())
				line.Trim().Color("m").Println("bypass")
			} else {
				local2host[line.Trim().Str()] = "local"
			}

		})
	}
}

func LoadLocalPac(path string) map[string]bool {
	if e := gs.Str(path); e.IsExists() && e.EndsWith(".pac") {
		if local2host == nil {
			local2host = make(gs.Dict[string])
		}

		e.MustAsFile().EveryLine(func(lineno int, line gs.Str) {
			if line.Trim().StartsWith("#") {
				return
			}
			if line.Trim() == "" {
				return
			}
			if !line.Trim().In(`"`) {
				return
			}
			// if line.In("*") {
			h := line.Split(`"`)[1].Trim().Str()
			paclock.Lock()
			if _, ok := PAC_HOSTS[h]; !ok {
				PAC_HOSTS[h] = true
			}
			paclock.Unlock()

		})
	} else {
		gs.Str("load failed from :" + path).Color("y").Println("pac file")
	}
	return PAC_HOSTS
}

func AutoLoadPac() {
	gs.Str("Use Pac loader").Color("m").Println()
	go func() {
		tick := time.NewTicker(10 * time.Second)
		for {
			select {
			case <-tick.C:
				s := []string{
					"/etc/z-client.pac",
					"/root/.config/gfwlist.pac",
					"/etc/gfwlist.pac",
					"~/.config/gfwlist.pac",
					"~/.local/z-client.pac",
					"~/.local/gfwlist.pac",
					"~/.cache/gfwlist.pac",
				}
				for _, p := range s {
					path := gs.Str(p).ExpandUser()
					if path.IsExists() {
						// gs.Str("pac " + path.Str()).Color("g").Println("pac")
						LoadLocalPac(path.Str())
						break
					}
				}

			default:
				time.Sleep(2 * time.Second)
			}
		}
	}()

}

func SearchIP(ip string) (doamin string) {
	lock.Lock()
	defer lock.Unlock()
	if domai, ok := ip2host[ip]; ok {
		doamin = domai
	}
	return
}

func IsPac(ip string) (ok bool) {
	// for dns parse
	ip = strings.TrimSuffix(ip, ".")

	if len(PAC_HOSTS) == 0 {
		return true
	}
	fs := strings.Split(ip, ".")
	dd := ""
	for i := len(fs) - 1; i >= 0; i-- {
		dd = fs[i] + "." + dd
		dd = strings.TrimSuffix(dd, ".")
		paclock.Lock()
		_, ok = PAC_HOSTS[dd]
		paclock.Unlock()
		if ok {
			if dd != ip {
				paclock.Lock()
				PAC_HOSTS[ip] = true
				paclock.Unlock()
			}
			break
		}
	}
	return
}

func IsLocal(ip string) (ok bool) {
	_, ok = local2host[ip]
	if ok {
		return true
	}

	if !ok {
		for _, i := range fuzzyHost {
			if ok {
				break
			}
			if gs.Str(i).In("*") {
				left := gs.Str(i).Replace("*", "")
				if left.Len() > 2 {
					ok = gs.Str(ip).In(left)
				}
			} else {
				if i == ip {
					ok = true
				}
			}
		}
	}
	return
}

func Exec(str gs.Str) gs.Str {
	var args []string
	// sep := "\n"
	if runtime.GOOS == "windows" {
		// sep = "\r\n"
		args = []string{"C:\\Windows\\System32\\Cmd.exe", "/C"}
	} else {
		args = []string{"sh", "-c"}
	}
	PATH := os.Getenv("PATH")
	if PATH == "" {
		PATH = "/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin"
	}
	args = append(args, str.String())
	cmd := exec.Command(args[0], args[1:]...)
	outbuffer := bytes.NewBuffer([]byte{})
	cmd.Stdout = outbuffer
	cmd.Stderr = outbuffer
	cmd.Run()
	return gs.Str(outbuffer.String())
}

func AddLocalIP(ip string) {
	fs := strings.Split(ip, ".")
	ff := strings.Join(fs[:3], ".") + ".0/24"
	lock.Lock()
	if _, ok := DYNAMIC_DIRECT[ff]; !ok {
		gs.Str(ip).Color("g").Println(gs.Str("iptables pass").Color("g"))
		// Exec(gs.Str(`iptables -t nat -I REDSOCKS 2 -d ` + ff + ` -j DIRECT`))
		Exec(gs.Str(`ipset add DIRECT ` + ff))
		DYNAMIC_DIRECT[ff] = true

	}
	defer lock.Unlock()
}

func Clear() {
	lock.Lock()
	names := gs.List[string]{}
	for n := range domainsToAddresses {
		names = names.Add(n)
	}
	names.Every(func(no int, i string) {
		delete(domainsToAddresses, i)
	})
	gs.Str("Clear dns cache").Color("g").Println()

	domainsToAddresses = make(map[string]*DNSRecord)
	lock.Unlock()
	gs.Str("~").ExpandUser().PathJoin(".config").Mkdir()
	s := gs.Str("~").ExpandUser().PathJoin(".config", "local.conf")
	s.Dirname().Mkdir()
	LoadLocalRule(s.Str())
}
