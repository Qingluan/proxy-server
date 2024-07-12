package clientcontroll

import (
	"net"

	"gitee.com/dark.H/ProxyZ/connections/prosocks5"
	"gitee.com/dark.H/gs"
)

func TestDomain(testHost string, localPort int) (out string) {
	conn, err := net.Dial("tcp", gs.Str("127.0.0.1:%d").F(localPort).Str())
	if err != nil {
		return err.Error()
	}
	_, err = conn.Write(prosocks5.SOCKS5Init)
	if err != nil {
		return "write init err" + err.Error()
	}
	_b := make([]byte, 10)
	conn.Read(_b)
	// fmt.Println(_b)
	b := prosocks5.HostToRaw(testHost, 443)
	if _, err = conn.Write(b); err != nil {
		conn.Close()
		return err.Error()
	}
	// io.Copy(os.Stdout, conn)
	buf := make([]byte, 8192)
	n, err := conn.Read(buf)
	if err != nil {
		return err.Error()
	}
	return string(buf[:n])
}
