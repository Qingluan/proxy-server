package clientcontroll

import (
	"bytes"
	"encoding/json"
	"io"
	"net"

	"gitee.com/dark.H/ProxyZ/connections/prosocks5"
	"gitee.com/dark.H/gs"
)

type SocksControl struct {
	Op      string `json:"op"`
	Data    string `json:"data"`
	control *ClientControl
}

func FromData(data []byte, c *ClientControl) (s *SocksControl, err error) {
	// 解析数据
	s = &SocksControl{}
	err = json.Unmarshal(data, s)
	if err != nil {
		gs.Str(err.Error()).Color("r").Println("From Data")
	}
	s.control = c
	return
}

func (s *SocksControl) ToData() (data []byte) {
	data, _ = json.Marshal(s)
	return
}

func (s *SocksControl) Reply(writer io.WriteCloser) {
	defer writer.Close()
	r := &SocksControl{
		Op: "reply",
	}
	switch s.Op {
	case "mode":
		switch s.Data {
		case "global":
			s.control.Mode = MODE_GLOBAL
			r.Data = "global mode"
		case "smart":
			s.control.Mode = MODE_SMART
			r.Data = "smart mode"
		}
	}

	writer.Write(r.ToData())
}

func (s *SocksControl) SendCommand(socks5listenAddr string) {
	conn, err := net.Dial("tcp", socks5listenAddr)
	if err != nil {
		gs.Str(err.Error()).Color("r").Println()
		return
	}
	_, err = conn.Write(prosocks5.SOCKS5Init)
	if err != nil {
		gs.Str("write init err" + err.Error()).Color("r").Println()
		return
	}
	_b := make([]byte, 10)
	conn.Read(_b)
	// fmt.Println(_b)
	b := prosocks5.HostToRaw("config.me", 0)
	if _, err = conn.Write(b); err != nil {
		conn.Close()
		gs.Str(err.Error()).Color("r").Println()
		return
	}
	// io.Copy(os.Stdout, conn)
	buf := make([]byte, 200)
	n, err := conn.Read(buf)
	if err != nil {
		gs.Str(err.Error()).Color("r").Println()
	}
	if bytes.Equal(buf[:n], prosocks5.Socks5Confirm) {
		buf := s.ToData()
		_, err = conn.Write(buf)
		if err != nil {
			gs.Str(err.Error()).Color("r").Println()
		}
		n, err = conn.Read(buf)
		if err != nil {
			gs.Str(err.Error()).Color("r").Println()
		}
		gs.Str(string(buf[:n])).Color("g").Println()
	}
	// return string(buf[:n])
}

func NewSocksControl(op string, data string, c *ClientControl) (s *SocksControl) {

	s = &SocksControl{
		Op:      op,
		Data:    data,
		control: c,
	}
	return
}
