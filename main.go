package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"time"

	"gitee.com/dark.H/ProxyZ/servercontroll"
	"gitee.com/dark.H/edns/pkg/edns"
	"gitee.com/dark.H/gs"
)

var (
	tlsserver = ""
	// quicserver = ""
	www           = ""
	godaemon      = false
	logFile       = ""
	guardProcess  = false
	ifnotstartdns = false
	watch         = ""
)

func Daemon(args []string, LOG_FILE string) {
	createLogFile := func(fileName string) (fd *os.File, err error) {
		dir := path.Dir(fileName)
		if _, err = os.Stat(dir); err != nil && os.IsNotExist(err) {
			if err = os.MkdirAll(dir, 0755); err != nil {
				log.Println(err)
				return
			}
		}
		if fd, err = os.Create(fileName); err != nil {
			log.Println(err)
			return
		}
		return
	}
	if LOG_FILE != "" {
		logFd, err := createLogFile(LOG_FILE)
		if err != nil {
			log.Println(err)
			return
		}
		defer logFd.Close()

		cmdName := args[0]
		newProc, err := os.StartProcess(cmdName, args, &os.ProcAttr{
			Files: []*os.File{logFd, logFd, logFd},
		})
		if err != nil {
			log.Fatal("daemon error:", err)
			return
		}
		log.Printf("Start-Deamon: run in daemon success, pid: %v\nlog : %s", newProc.Pid, LOG_FILE)
	} else {
		cmdName := args[0]
		newProc, err := os.StartProcess(cmdName, args, &os.ProcAttr{
			Files: []*os.File{nil, nil, nil},
		})
		if err != nil {
			log.Fatal("daemon error:", err)
			return
		}
		log.Printf("Start-Deamon: run in daemon success, pid: %v\n", newProc.Pid)
	}
	return
	// }
}

func StartDNS(port int) {
	cfg := edns.DefaultConfig()
	cfg.Server.ListenAddr = fmt.Sprintf("0.0.0.0:%d", port)
	cfg.Server.TransportProtocol = "udp"
	cfg.Server.EnableLogging = true
	cfg.Server.UpstreamDNS = []string{"8.8.8.8:53", "1.1.1.1:53"}
	// cfg.Common.EncryptionKey = "my-secret-key-123"

	// 创建服务器
	server, err := edns.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	log.Println("Starting DNS proxy server...")
	// 启动服务器（阻塞）
	if err := server.StartService(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	// flag.StringVar(&quicserver, "quic-api", "0.0.0.0:55444", "http3 server addr")
	flag.StringVar(&tlsserver, "tls-api", "0.0.0.0:55443", "http3 server addr")
	flag.StringVar(&www, "www", "/tmp/www", "http3 server www dir path")
	flag.BoolVar(&godaemon, "d", false, "run as a daemon !")
	flag.StringVar(&watch, "watch", "", "set watch puzzcle")
	flag.StringVar(&logFile, "log", "/tmp/z.log", "set daemon log file path")
	flag.BoolVar(&guardProcess, "g", false, "set gurad process to commit")
	flag.BoolVar(&ifnotstartdns, "nodns", false, "set if close dns server")
	flag.Parse()
	if !gs.Str(www).IsExists() {
		gs.Str(www).Mkdir()
	}

	if watch != "" {
		Watch(watch)
		os.Exit(0)
	}
	if godaemon {
		args := []string{}
		for _, a := range os.Args {
			if a == "-d" {
				continue
			}
			args = append(args, a)
		}
		Daemon(args, logFile)
		time.Sleep(2 * time.Second)
		// fmt.Printf("%s [PID] %d running...\n", os.Args[0], cmd.Process.Pid)
		os.Exit(0)
	}

	if guardProcess {
		// name := filepath.Base(os.Args[0])
		args := []string{os.Args[0], "-watch", os.Args[0]}
		// for _, a := range os.Args {
		// 	if a == "-g" {
		// 		continue
		// 	}
		// 	args = append(args, a)
		// }
		Daemon(args, "/tmp/log-g.log")
	}
	// gs.Str(quicserver).Println("Server Run")
	// go servercontroll.HTTP3Server(quicserver, www, true)
	time.Sleep(7 * time.Second)
	if !ifnotstartdns {
		go StartDNS(55354)
	}
	servercontroll.HTTP3Server(tlsserver, www, false)

}

func Watch(watch string) {
	for {
		time.Sleep(2 * time.Second)
		cmd := exec.Command("bash", "-c", "ps aux | grep "+watch+" | grep -v grep")
		cmd.Env = append(cmd.Env, os.Environ()...)
		buf := bytes.NewBuffer([]byte{})
		cmd.Stdout = buf
		cmd.Stderr = buf
		cmd.Run()
		// basename := filepath.Base(watch)
		res := gs.Str(buf.String()).Replace(watch+" -watch "+watch, "")
		gs.Str(res).Color("g").Println("test result")
		if res.String() == "" {
			Daemon([]string{watch}, "/tmp/z.log")
			time.Sleep(10 * time.Second)
		} else if !res.In(watch) {
			Daemon([]string{watch}, "/tmp/z.log")
			time.Sleep(10 * time.Second)
		}

	}
}
