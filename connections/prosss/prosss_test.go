package prosss

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/dark.H/ProxyZ/connections/base"
	"gitee.com/dark.H/gs"
)

// parseAddr parses "host:port" string
func parseAddr(addr string) (host string, port int) {
	parts := strings.Split(addr, ":")
	host = parts[0]
	port, _ = strconv.Atoi(parts[1])
	return
}

// TestSSServerClient tests the full server-client communication
func TestSSServerClient(t *testing.T) {
	password := "test-password-12345"
	method := "aes-256-gcm"
	serverAddr := "127.0.0.1:18888"

	var wg sync.WaitGroup
	wg.Add(2)

	// Start server
	go func() {
		defer wg.Done()
		runTestServer(t, serverAddr, password, method)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Run client test
	go func() {
		defer wg.Done()
		runTestClient(t, serverAddr, password, method)
	}()

	wg.Wait()
}

func runTestServer(t *testing.T, addr, password, method string) {
	config := &base.ProtocolConfig{
		Server:     "127.0.0.1",
		ServerPort: 18888,
		SSPassword: password,
		SSMethod:   method,
	}

	server, err := NewSSServer(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	handle := func(conn net.Conn) error {
		defer conn.Close()

		// Read data from client
		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("server read failed: %w", err)
		}

		received := string(buf[:n])
		gs.Str("Server received: %s").F(received).Color("g").Println("ss")

		// Echo back to client
		_, err = conn.Write(buf[:n])
		if err != nil {
			return fmt.Errorf("server write failed: %w", err)
		}

		// Signal server to stop after handling one connection
		time.Sleep(100 * time.Millisecond)
		server.TryClose()
		return nil
	}

	// Accept one connection and handle it
	go server.AcceptHandle(5*time.Second, handle)
	time.Sleep(3 * time.Second)
}

func runTestClient(t *testing.T, serverAddr, password, method string) {
	host, port := parseAddr(serverAddr)
	config := &base.ProtocolConfig{
		Server:     host,
		ServerPort: port,
		SSPassword: password,
		SSMethod:   method,
	}

	client, err := NewSSClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.conn.Close()

	// Wrap connection
	conn := &SSClientConn{
		Conn:   client.conn,
		cipher: client.cipher,
	}

	// Test data
	testData := []byte("Hello, this is a test message from client!")

	// Write to server
	_, err = conn.Write(testData)
	if err != nil {
		t.Fatalf("Client write failed: %v", err)
	}
	gs.Str("Client sent: %s").F(string(testData)).Color("y").Println("ss")

	// Read echo response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Client read failed: %v", err)
	}

	received := buf[:n]
	if !bytes.Equal(testData, received) {
		t.Errorf("Data mismatch: expected %q, got %q", testData, received)
	}
	gs.Str("Client received: %s").F(string(received)).Color("y").Println("ss")
}

// TestLargeData tests handling of data larger than buffer
func TestLargeData(t *testing.T) {
	password := "test-password-large"
	method := "chacha20-poly1305"
	serverAddr := "127.0.0.1:18889"

	var wg sync.WaitGroup
	wg.Add(2)

	// Start server
	go func() {
		defer wg.Done()
		runLargeDataServer(t, serverAddr, password, method)
	}()

	time.Sleep(100 * time.Millisecond)

	// Run client
	go func() {
		defer wg.Done()
		runLargeDataClient(t, serverAddr, password, method)
	}()

	wg.Wait()
}

func runLargeDataServer(t *testing.T, addr, password, method string) {
	config := &base.ProtocolConfig{
		Server:     "127.0.0.1",
		ServerPort: 18889,
		SSPassword: password,
		SSMethod:   method,
	}

	server, err := NewSSServer(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	handle := func(conn net.Conn) error {
		defer conn.Close()

		// Read with small buffer to test buffering
		smallBuf := make([]byte, 512)
		var received bytes.Buffer

		// Read one packet and echo back
		n, err := conn.Read(smallBuf)
		if err != nil {
			return err
		}
		received.Write(smallBuf[:n])

		// Continue reading until we have all data (buffer test)
		for received.Len() < 8192 {
			n, err := conn.Read(smallBuf)
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			if n == 0 {
				break
			}
			received.Write(smallBuf[:n])
		}

		gs.Str("Server received %d bytes").F(received.Len()).Color("g").Println("ss")

		// Echo back
		_, err = conn.Write(received.Bytes())
		if err != nil {
			return err
		}

		// Signal server to stop
		time.Sleep(100 * time.Millisecond)
		server.TryClose()
		return nil
	}

	go server.AcceptHandle(10*time.Second, handle)
	time.Sleep(5 * time.Second)
}

func runLargeDataClient(t *testing.T, serverAddr, password, method string) {
	host, port := parseAddr(serverAddr)
	config := &base.ProtocolConfig{
		Server:     host,
		ServerPort: port,
		SSPassword: password,
		SSMethod:   method,
	}

	client, err := NewSSClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.conn.Close()

	conn := &SSClientConn{
		Conn:   client.conn,
		cipher: client.cipher,
	}

	// Create large data (8KB)
	largeData := make([]byte, 8192)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	_, err = conn.Write(largeData)
	if err != nil {
		t.Fatalf("Client write large data failed: %v", err)
	}
	gs.Str("Client sent %d bytes").F(len(largeData)).Color("y").Println("ss")

	// Read with small buffer
	smallBuf := make([]byte, 512)
	var received bytes.Buffer

	for received.Len() < len(largeData) {
		n, err := conn.Read(smallBuf)
		if err != nil {
			t.Fatalf("Client read large data failed: %v", err)
		}
		received.Write(smallBuf[:n])
	}

	if !bytes.Equal(largeData, received.Bytes()) {
		t.Errorf("Large data mismatch")
	}
	gs.Str("Client received %d bytes, verified OK").F(received.Len()).Color("g").Println("ss")
}

// TestCipherEncryption tests cipher encrypt/decrypt
func TestCipherEncryption(t *testing.T) {
	password := "test-cipher-password"
	methods := []string{"aes-256-gcm", "chacha20-poly1305"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			cipher, err := CreateCipher(password, method)
			if err != nil {
				t.Fatalf("Failed to create cipher: %v", err)
			}

			testData := []byte("Hello, World! This is a test.")

			// Encrypt
			ciphertext, err := cipher.Encrypt(testData)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Verify ciphertext is different from plaintext
			if bytes.Equal(testData, ciphertext) {
				t.Error("Ciphertext should differ from plaintext")
			}

			// Decrypt
			plaintext, err := cipher.Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			// Verify round-trip
			if !bytes.Equal(testData, plaintext) {
				t.Errorf("Round-trip failed: expected %q, got %q", testData, plaintext)
			}

			gs.Str("Cipher %s encryption/decryption OK").F(method).Color("g").Println("ss")
		})
	}
}

// TestInvalidPackets tests handling of invalid packets
func TestInvalidPackets(t *testing.T) {
	password := "test-invalid"
	method := "aes-256-gcm"
	serverAddr := "127.0.0.1:18890"

	// Start server
	go func() {
		config := &base.ProtocolConfig{
			Server:     "127.0.0.1",
			ServerPort: 18890,
			SSPassword: password,
			SSMethod:   method,
		}
		server, _ := NewSSServer(config)
		go server.AcceptHandle(5*time.Second, func(conn net.Conn) error {
			defer conn.Close()
			return nil
		})
	}()

	time.Sleep(100 * time.Millisecond)

	// Test 1: Send invalid length prefix
	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send too large packet size
	badLength := []byte{0xFF, 0xFF} // 65535
	conn.Write(badLength)

	buf := make([]byte, 100)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("Expected error for oversized packet")
	}
	gs.Str("Invalid packet test: got expected error %v").F(err).Color("y").Println("ss")
}

// TestConcurrentConnections tests multiple concurrent connections
func TestConcurrentConnections(t *testing.T) {
	password := "test-concurrent"
	method := "aes-256-gcm"
	serverAddr := "127.0.0.1:18891"

	config := &base.ProtocolConfig{
		Server:     "127.0.0.1",
		ServerPort: 18891,
		SSPassword: password,
		SSMethod:   method,
	}

	server, err := NewSSServer(config)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	connCount := 0
	var countLock sync.Mutex

	handle := func(conn net.Conn) error {
		countLock.Lock()
		connCount++
		current := connCount
		countLock.Unlock()

		defer conn.Close()

		// Read message
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return err
		}

		// Send response
		response := fmt.Sprintf("Server response #%d: %s", current, string(buf[:n]))
		_, err = conn.Write([]byte(response))
		return err
	}

	go server.AcceptHandle(10*time.Second, handle)
	time.Sleep(100 * time.Millisecond)

	// Create multiple concurrent clients
	var wg sync.WaitGroup
	numClients := 5

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			host, port := parseAddr(serverAddr)
			clientConfig := &base.ProtocolConfig{
				Server:     host,
				ServerPort: port,
				SSPassword: password,
				SSMethod:   method,
			}

			client, err := NewSSClient(clientConfig)
			if err != nil {
				t.Errorf("Client %d failed: %v", idx, err)
				return
			}
			defer client.conn.Close()

			conn := &SSClientConn{
				Conn:   client.conn,
				cipher: client.cipher,
			}

			message := fmt.Sprintf("Hello from client #%d", idx)
			_, err = conn.Write([]byte(message))
			if err != nil {
				t.Errorf("Client %d write failed: %v", idx, err)
				return
			}

			buf := make([]byte, 2048)
			n, err := conn.Read(buf)
			if err != nil {
				t.Errorf("Client %d read failed: %v", idx, err)
				return
			}

			gs.Str("Client %d received: %s").F(idx, string(buf[:n])).Color("c").Println("ss")
		}(i)
		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()
	gs.Str("Concurrent test completed, handled %d connections").F(connCount).Color("g").Println("ss")
}
