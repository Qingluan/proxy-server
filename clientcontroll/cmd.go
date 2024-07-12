package clientcontroll

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gitee.com/dark.H/ProxyZ/router"
	"gitee.com/dark.H/gs"
)

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

func ExecBackgroud(str gs.Str) int {
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
	if gs.Str("/tmp").IsExists() {
		gs.Str("/tmp/z-client.log").Println("[Exec Log]")
		_, outbuffer, err := gs.Str("/tmp/z-client.log").OpenFile(gs.O_APPEND_WRITE)
		if err != nil {
			gs.Str(err.Error()).Println("log exec err:")
		}
		cmd.Stdout = outbuffer
		cmd.Stderr = outbuffer

	} else {
		outbuffer := bytes.NewBuffer([]byte{})
		cmd.Stdout = outbuffer
		cmd.Stderr = outbuffer

	}
	cmd.Start()

	time.Sleep(1 * time.Second)
	return cmd.Process.Pid
}

func KillProcess(pid int) {
	Exec(gs.Str("kill -9 %d").F(pid))
}

func CheckProcess(key string) bool {
	res := Exec(gs.Str("ps|grep %s | grep -v '(grep|egrep)'").F(key)).Trim()
	if res == "" {
		return false
	} else {
		return true
	}
}

func GetProcessPID(keys ...string) int {
	key := ""
	PRE := "ps"
	for _, v := range keys {
		if strings.HasPrefix(v, "-") {
			v = "\\" + v
		}
		key += "grep \"" + v + "\"|"
	}
	if key == "" {
		return -1
	}
	if !router.IsRouter() {
		PRE = "ps aux"
	}

	res := Exec(gs.Str(PRE + "|%s  grep -v 'grep'").F(key).Println("Get PID")).Trim()
	if res == "" {
		return -1
	} else {
		pid := -1
		var err error
		found := false
		gs.Str(res).EveryLine(func(lineno int, line gs.Str) {
			if found {
				return
			}
			fs := line.SplitSpace()
			if fs.Len() > 2 {
				pid, err = strconv.Atoi(fs[1].String())
				if err != nil {
					return
				}
				found = true
			}
		})
		return pid
	}
}
