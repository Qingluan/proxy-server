package prosss

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
	"gitee.com/dark.H/gs"
)

// SSServer represents a tcp-encrypt server
type SSServer struct {
	config    *base.ProtocolConfig
	cipher    Cipher
	ips       gs.Dict[bool]
	lock      sync.RWMutex
	ZeroToDel bool
}

// NewSSServer creates a new tcp-encrypt server
func NewSSServer(config *base.ProtocolConfig) (*SSServer, error) {
	// Get method from config (use SSMethod or default to aes-256-gcm)
	method := config.SSMethod
	if method == "" {
		method = "aes-256-gcm"
	}

	// Get password from config
	password := config.SSPassword
	if password == "" {
		password = config.Password
	}

	if password == "" {
		return nil, errors.New("tcp-encrypt requires a password")
	}

	// Create cipher
	cipher, err := CreateCipher(password, method)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	config.ProxyType = "ss"
	config.Method = method

	return &SSServer{
		config: config,
		cipher: cipher,
		ips:    make(gs.Dict[bool]),
	}, nil
}

func (s *SSServer) Record(con net.Addr) {
	ip := con.String()
	if s.ips == nil {
		s.ips = make(gs.Dict[bool])
	}
	if _, ok := s.ips[ip]; !ok {
		s.lock.Lock()
		s.ips[ip] = true
		s.lock.Unlock()
	}
}

func (s *SSServer) DelRecord(con net.Conn) {
	if s.ips == nil {
		s.ips = make(gs.Dict[bool])
	}
	ip := con.RemoteAddr().String()
	if _, ok := s.ips[ip]; ok {
		s.lock.Lock()
		delete(s.ips, ip)
		s.lock.Unlock()
	}
}

func (s *SSServer) GetAliveIPS() gs.List[string] {
	ds := gs.List[string]{}
	for k := range s.ips {
		ds = append(ds, k)
	}
	return ds
}

func (s *SSServer) GetConfig() *base.ProtocolConfig {
	return s.config
}

func (s *SSServer) TryClose() {
	s.ZeroToDel = true
}

func (s *SSServer) DelCon(con net.Conn) {
	con.Close()
	s.DelRecord(con)
}

// GetListener creates a TCP listener for the tcp-encrypt server
func (s *SSServer) GetListener() net.Listener {
	address := gs.Str("%s:%d").F(s.config.Server, s.config.ServerPort).Str()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		gs.Str("SS Listen error: %v").F(err).Color("r").Println("ss")
		return nil
	}
	return listener
}

// AcceptHandle accepts and handles incoming tcp-encrypt connections
func (s *SSServer) AcceptHandle(waitTime time.Duration, handle func(con net.Conn) error) error {
	listener := s.GetListener()
	if listener == nil {
		return errors.New("failed to create listener")
	}
	defer listener.Close()

	gs.Str("tcp-encrypt server listening on %s method: %s").F(listener.Addr(), s.config.Method).Color("g").Println("ss")

	ticker := time.NewTicker(waitTime)
	defer ticker.Stop()

	acceptChan := make(chan net.Conn, 128)
	errChan := make(chan error, 1)

	// Accept goroutine
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if !s.ZeroToDel {
					errChan <- err
				}
				return
			}
			s.Record(conn.RemoteAddr())

			// Wrap connection with cipher
			ssConn := &SSConn{
				Conn:   conn,
				cipher: s.cipher,
			}
			acceptChan <- ssConn
		}
	}()

	for {
		select {
		case <-ticker.C:
			if s.ZeroToDel {
				return nil
			}
		case err := <-errChan:
			return err
		case conn := <-acceptChan:
			go handle(conn)
		}
	}
}

// SSConn wraps a net.Conn with tcp-encrypt encryption
type SSConn struct {
	net.Conn
	cipher  Cipher
	buf     []byte // buffer for remaining data from previous packet
	bufSize int    // number of bytes remaining in buf
}

// Read reads and decrypts data from the connection
func (c *SSConn) Read(b []byte) (n int, err error) {
	// If we have buffered data from previous read, return it first
	if c.bufSize > 0 {
		n = copy(b, c.buf[len(c.buf)-c.bufSize:])
		c.bufSize -= n
		if c.bufSize == 0 {
			c.buf = nil
		}
		return n, nil
	}

	// Read length prefix (2 bytes)
	var lengthBuf [2]byte
	_, err = io.ReadFull(c.Conn, lengthBuf[:])
	if err != nil {
		return 0, err
	}
	// Decode length (big endian)
	packetLen := int(lengthBuf[0])<<8 | int(lengthBuf[1])

	// Validate packet size
	if packetLen < c.cipher.NonceSize() {
		return 0, fmt.Errorf("invalid packet size: %d (smaller than nonce size %d)", packetLen, c.cipher.NonceSize())
	}
	if packetLen > maxPacketSize {
		return 0, fmt.Errorf("packet too large: %d (max %d)", packetLen, maxPacketSize)
	}

	// Read the encrypted packet
	encryptedBuf := make([]byte, packetLen)
	_, err = io.ReadFull(c.Conn, encryptedBuf)
	if err != nil {
		return 0, fmt.Errorf("read encrypted packet failed: %w", err)
	}

	// Decrypt
	plaintext, err := c.cipher.Decrypt(encryptedBuf)
	if err != nil {
		return 0, fmt.Errorf("decryption failed: %w", err)
	}

	// Copy to output buffer
	if len(plaintext) <= len(b) {
		copy(b, plaintext)
		return len(plaintext), nil
	}

	// Buffer the overflow data for next read
	c.buf = plaintext
	c.bufSize = len(plaintext)
	n = copy(b, plaintext)
	c.bufSize -= n
	return n, nil
}

// Write encrypts and writes data to the connection
func (c *SSConn) Write(b []byte) (n int, err error) {
	// Encrypt
	ciphertext, err := c.cipher.Encrypt(b)
	if err != nil {
		return 0, fmt.Errorf("encryption failed: %w", err)
	}

	// Check packet size
	if len(ciphertext) > maxPacketSize {
		return 0, fmt.Errorf("encrypted packet too large: %d (max %d)", len(ciphertext), maxPacketSize)
	}

	// Length prefix: 2 bytes big endian
	packetLen := len(ciphertext)
	lengthBuf := []byte{byte(packetLen >> 8), byte(packetLen & 0xff)}

	// Combine length prefix + encrypted data for atomic write
	packet := make([]byte, 0, 2+len(ciphertext))
	packet = append(packet, lengthBuf...)
	packet = append(packet, ciphertext...)

	_, err = c.Conn.Write(packet)
	if err != nil {
		return 0, fmt.Errorf("write packet failed: %w", err)
	}
	return len(b), nil // Return original plaintext length
}
