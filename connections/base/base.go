package base

import (
	"io"
	"math/rand"
	"net"
	"runtime"
	"strconv"
	"time"

	"gitee.com/dark.H/gs"
)

const bufSize = 4096

func ErrToFile(label string, err error) {
	c := gs.Str("[%s]:" + err.Error() + "\n").F(label)
	// c.Color("r").Print()
	c.ToFile("/tmp/z.log")
}

// const bufSize = 8192

// Memory optimized io.Copy function specified for this library
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}

	// fallback to standard io.CopyBuffer
	buf := make([]byte, bufSize)
	return io.CopyBuffer(dst, src, buf)
}

func chanFromConn(conn net.Conn, bufsize int) chan []byte {
	c := make(chan []byte)
	go func() {
		b := make([]byte, bufsize)
		for {
			conn.SetReadDeadline(time.Now().Add(1 * time.Minute))
			n, err := conn.Read(b)
			if n > 0 {
				res := make([]byte, n)
				// Copy the buffer so it doesn't get changed while read by the recipient.
				copy(res, b[:n])
				c <- res
			}
			if err != nil {
				c <- nil
				break
			}
		}
	}()
	return c
}

func ChanPipe(p1, p2 net.Conn, bufsizes ...int) (err error) {
	bufsize := 322766
	if len(bufsizes) > 0 {
		bufsize = bufsizes[0]
	}
	chan1 := chanFromConn(p1, bufsize)
	chan2 := chanFromConn(p2, bufsize)

	for {
		select {
		case b1 := <-chan1:
			if b1 == nil {
				return
			} else {
				n := 0
				// var err error
				for n < len(b1) {
					ni, err := p2.Write(b1[n:])
					if err != nil {
						return err

					}
					n += ni
				}

			}
		case b2 := <-chan2:
			if b2 == nil {
				return
			} else {
				n := 0
				for n < len(b2) {
					ni, err := p1.Write(b2[n:])
					if err != nil {
						return err

					}
					n += ni
				}
			}
		}
	}
}

func OpenPortUFW(port int) {
	if runtime.GOOS == "linux" {
		gs.Str("open port :%d").F(port).Color("y").Println()
		gs.Str("ufw allow %d").F(port).Println("ufw").Exec()
		// if res != "" {
		// 	gs.Str(res).Println("ufw")
		// }

	}
}

func ClosePortUFW(port int) {
	switch port {
	case 22, 55443, 60053, 60001:
		return
	}
	if runtime.GOOS == "linux" {
		gs.Str("ufw delete allow %d/tcp ;ufw delete allow %d/udp; ufw delete allow %d").F(port, port, port).Exec()
		gs.Str("close port :%d").F(port).Color("y", "B").Println("Close")

	}
}

func CloseALLPortUFW() {
	ss := GetUFW()
	// ps := []int{}
	gs.Str(ss).Split("\n").Every(func(no int, i gs.Str) {
		if i.In("/") {
			if i.In("22") {
				return
			}
			ii, err := strconv.Atoi(i.Split("/")[0].Str())
			if err == nil {
				ClosePortUFW(ii)
			}
		}
	})
}

func GetUFW() string {
	port := gs.Str("")
	gs.Str("ufw status | grep ALLOW").Exec().EveryLine(func(lineno int, line gs.Str) {
		ss := line.SplitSpace()
		if ss.Len() > 0 {
			p := ss[0]
			switch p.Trim() {
			case "22", "55443", "60053", "22/tcp", "55443/tcp", "60053/tcp", "22/udp", "55443/udp", "60053/udp", "60001/udp":
			default:
				port += p.Trim() + "\n"
			}
		}
	})
	return port.Trim().Str()
}

func GiveAPort() (port int) {
	for {
		port = 40000 + rand.Int()%10000
		ln, err := net.Listen("tcp", ":"+gs.S(port).Str())
		if err == nil {
			ln.Close()
			OpenPortUFW(port)
			return port
		}
	}

}
