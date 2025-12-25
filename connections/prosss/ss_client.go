package prosss

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
)

// SSClient represents a tcp-encrypt client connection
type SSClient struct {
	conn   net.Conn
	cipher Cipher
}

// NewSSClient creates a new tcp-encrypt client connection
func NewSSClient(config *base.ProtocolConfig) (*SSClient, error) {
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

	// Establish TCP connection
	dialer := &net.Dialer{
		Timeout: 12 * time.Second,
	}
	conn, err := dialer.Dial("tcp", config.RemoteAddr())
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &SSClient{
		conn:   conn,
		cipher: cipher,
	}, nil
}

// ConnectSS connects to a tcp-encrypt server
func ConnectSS(config *base.ProtocolConfig) (net.Conn, error) {
	client, err := NewSSClient(config)
	if err != nil {
		return nil, err
	}

	return &SSClientConn{
		Conn:   client.conn,
		cipher: client.cipher,
	}, nil
}

const maxPacketSize = 64 * 1024 // 64KB max packet size

// SSClientConn wraps a net.Conn with tcp-encrypt encryption for client
type SSClientConn struct {
	net.Conn
	cipher  Cipher
	buf     []byte // buffer for remaining data from previous packet
	bufSize int    // number of bytes remaining in buf
}

// Read reads and decrypts data from the connection
func (c *SSClientConn) Read(b []byte) (n int, err error) {
	// If we have buffered data from previous read, return it first
	if c.bufSize > 0 {
		start := len(c.buf) - c.bufSize
		n = copy(b, c.buf[start:])
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
func (c *SSClientConn) Write(b []byte) (n int, err error) {
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

// GetProxyType returns the proxy type (for compatibility with client interface)
func (c *SSClient) GetProxyType() string {
	return "ss"
}
